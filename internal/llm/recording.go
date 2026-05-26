package llm

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RecordingChatModel 把每次 Generate / Stream 调用的 latency / tokens / 错误分类
// 落到 in-memory slice。给 cmd/demo（端到端验证）做 prompt 稳定性统计用。
//
// 部署位置（关键）：必须放在 BreakingChatModel **之外**。理由——
// 熔断器 fail-fast 时返回 ErrBreakerOpen，我们要能数出"有多少请求被 breaker 挡了"；
// 同理 context.Canceled 也要被记录。
// 推荐链路：RealChatModel → LimitedChatModel → BreakingChatModel → RecordingChatModel。
//
// 并发：records 用 sync.Mutex 保护。一个 Generate 一条记录，记录写入在调用结束后，
// inner 调用本身不持锁。
//
// 默认错误消息截断到 maxErrMsg；过大的 provider error body 会拖大 run.json。
type RecordingChatModel struct {
	inner     ChatModel
	now       func() time.Time
	maxErrMsg int

	mu      sync.Mutex
	records []CallRecord
}

// CallRecord 是单次 LLM 调用的结构化痕迹。
// 字段都用基础类型，方便直接 json.Marshal 写到 run.json。
type CallRecord struct {
	StartedAt        time.Time     `json:"started_at"`
	Duration         time.Duration `json:"duration_ns"`
	Model            string        `json:"model,omitempty"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	// ErrClass 见 classifyChatErr。
	ErrClass string `json:"err_class"`
	// ErrMsg 截断到 maxErrMsg 长度避免 run.json 暴涨。
	ErrMsg string `json:"err_msg,omitempty"`
}

// NewRecordingChatModel 构造录制装饰器。
//   - inner 为 nil 时返回 nil，呼应 NewLimitedChatModel / NewBreakingChatModel
//     的"无 inner 不装饰"约定。
func NewRecordingChatModel(inner ChatModel) *RecordingChatModel {
	if inner == nil {
		return nil
	}
	return &RecordingChatModel{
		inner:     inner,
		now:       time.Now,
		maxErrMsg: 200,
	}
}

// Name 透传。Metrics 维度仍使用底层模型名。
func (m *RecordingChatModel) Name() string {
	return m.inner.Name()
}

// Generate 调底层模型并记录单次 CallRecord。
// 上游错误透传不变。
func (m *RecordingChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	start := m.now()
	resp, err := m.inner.Generate(ctx, messages, opts)
	m.record(start, resp, err)
	return resp, err
}

// Stream 同步部分：启动成功/失败都记一条，启动后 chunk 内错误不再追踪
// （和 BreakingChatModel.Stream 保持一致）。
//   - 启动失败：err != nil，记一条带 ErrClass 的失败记录
//   - 启动成功：记一条 ErrClass="ok" 的记录，duration 仅覆盖到 inner.Stream 返回
func (m *RecordingChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	start := m.now()
	ch, err := m.inner.Stream(ctx, messages, opts)
	m.record(start, nil, err)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// Snapshot 返回当前所有 CallRecord 的副本。
// 拷贝避免调用方持有内部 slice 后续被 append 影响。
func (m *RecordingChatModel) Snapshot() []CallRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CallRecord, len(m.records))
	copy(out, m.records)
	return out
}

// Reset 清空已记录的 CallRecord。
// 用于跨 run 复用同一个 recorder 实例的场景（测试 / 多脚本批跑）。
func (m *RecordingChatModel) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = nil
}

func (m *RecordingChatModel) record(start time.Time, resp *Response, err error) {
	end := m.now()
	rec := CallRecord{
		StartedAt: start,
		Duration:  end.Sub(start),
		ErrClass:  classifyChatErr(err),
	}
	if resp != nil {
		rec.Model = resp.Model
		rec.PromptTokens = resp.PromptTokens
		rec.CompletionTokens = resp.CompletionTokens
	}
	if err != nil {
		rec.ErrMsg = truncate(err.Error(), m.maxErrMsg)
	}
	m.mu.Lock()
	m.records = append(m.records, rec)
	m.mu.Unlock()
}

// classifyChatErr 把 error 映射到 7 个桶之一。
// 与 BreakingChatModel.shouldCount 共用 sentinel 集合，但桶定义更细：
// 这里要区分 ErrPermanent / ErrSchemaInvalid / ErrBreakerOpen / 取消 / 超时，
// 给 demo 报告"哪种错占主导"。
//
// 顺序保留语义优先级：
//   - breaker_open 在 transient 之前——熔断 fail-fast 不应该和真正的 transient 混淆
//   - schema_invalid 优先于 permanent（CallWithSchema 会把 schema 错 wrap 成 ErrSchemaInvalid）
func classifyChatErr(err error) string {
	return ClassifyChatErr(err)
}

// ClassifyChatErr 是 classifyChatErr 的导出版本，供其他包（如 observability
// 包的 RecordingCallback）复用相同的错误分桶逻辑。
func ClassifyChatErr(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, ErrBreakerOpen):
		return "breaker_open"
	case errors.Is(err, ErrSchemaInvalid):
		return "schema_invalid"
	case errors.Is(err, ErrPermanent):
		return "permanent"
	case errors.Is(err, ErrTransient):
		return "transient"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	default:
		return "other"
	}
}

// ensure interface satisfied at compile time
var _ ChatModel = (*RecordingChatModel)(nil)
