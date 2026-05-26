// recording_callback.go 提供 graph.Callback 实现，把节点 start / end / error
// 落到 in-memory slice，供 cmd/demo 写 run.json 时使用。
//
// 放在 observability 包而非 graph 包：依赖 graph 是单向的（observability →
// graph），避免 graph 反向依赖 llm 包的 classifyChatErr。
package observability

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// NodeRecord 是单个节点执行的结构化痕迹。
// 字段都用基础类型，方便直接 json.Marshal 写到 run.json。
type NodeRecord struct {
	Node      string        `json:"node"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration_ns"`
	// ErrClass 复用 llm 包的分类逻辑（见 llm.classifyChatErr 的桶定义）；
	// 节点错误大多来自 LLM 调用失败 + graph.ErrSuspended/ErrPermanent，
	// 这里同表方便 demo 报告做统一聚合。
	ErrClass string `json:"err_class"`
	// ErrMsg 截断到 maxErrMsg，避免 run.json 暴涨。
	ErrMsg string `json:"err_msg,omitempty"`
}

// RecordingCallback 跟踪图节点的 start/end/error，得到 NodeRecord 时间线。
//
// 内部用 map[node]time.Time 记 in-flight 节点的 start 时间；agent 子图有循环
// 回边，同一名节点会被多次 start/end 配对，这里假设单节点不会并发自己重入
// （agent loop 串行执行）。OnNodeStart 在同 key 上覆盖了未结束的旧 start 时，
// 打 warn-level slog 提示——这种情况意味着图执行模型出 bug 了，需要排查。
type RecordingCallback struct {
	now       func() time.Time
	maxErrMsg int

	mu       sync.Mutex
	inflight map[string]time.Time
	records  []NodeRecord
}

// NewRecordingCallback 构造 RecordingCallback。
func NewRecordingCallback() *RecordingCallback {
	return &RecordingCallback{
		now:       time.Now,
		maxErrMsg: 200,
		inflight:  map[string]time.Time{},
	}
}

// OnNodeStart 记录节点开始时间。
func (r *RecordingCallback) OnNodeStart(ctx context.Context, node string, _ *domain.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.inflight[node]; ok {
		// 不应该发生：同节点未 end 又 start 表示节点框架出 bug。
		// 记 warn 帮助调试，但仍然覆盖 old，让后续配对走 happy path。
		slog.WarnContext(ctx, "recording callback: node started while previous in-flight",
			"event", "node_overlap",
			"node", node,
			"previous_started_at", old,
		)
	}
	r.inflight[node] = r.now()
}

// OnNodeEnd 配对 start，写一条 ok 记录。
func (r *RecordingCallback) OnNodeEnd(ctx context.Context, node string, _ *domain.Session) {
	r.complete(ctx, node, nil)
}

// OnNodeError 配对 start，写一条带 ErrClass / ErrMsg 的记录。
// graph 框架在节点 fn 返回非 nil 时调 OnNodeError 而非 OnNodeEnd
// （见 internal/graph/graph.go: executeNode），所以正常情况下二者只触发一次。
func (r *RecordingCallback) OnNodeError(ctx context.Context, node string, _ *domain.Session, err error) {
	r.complete(ctx, node, err)
}

// Snapshot 返回当前所有 NodeRecord 的副本。
func (r *RecordingCallback) Snapshot() []NodeRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]NodeRecord, len(r.records))
	copy(out, r.records)
	return out
}

// Reset 清空 records 与 in-flight 状态。
func (r *RecordingCallback) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = nil
	r.inflight = map[string]time.Time{}
}

func (r *RecordingCallback) complete(ctx context.Context, node string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	start, ok := r.inflight[node]
	if !ok {
		// 不应该发生：end/error 没对应的 start。
		// 这意味着 callback 漏了 start，框架层 bug。仍然写一条 0 时长的记录，便于排查。
		slog.WarnContext(ctx, "recording callback: node completed without matching start",
			"event", "node_unmatched_end",
			"node", node,
		)
		start = r.now()
	} else {
		delete(r.inflight, node)
	}
	rec := NodeRecord{
		Node:      node,
		StartedAt: start,
		Duration:  r.now().Sub(start),
		ErrClass:  classifyNodeErr(err),
	}
	if err != nil {
		rec.ErrMsg = truncate(err.Error(), r.maxErrMsg)
	}
	r.records = append(r.records, rec)
}

// classifyNodeErr 把节点 err 映射到 llm.classifyChatErr 的桶——
// 大多数节点错误最终都包了 LLM 错误，能用统一表。
// 额外覆盖 graph.ErrSuspended（业务正常暂停，不算失败）和 graph.ErrPermanent。
func classifyNodeErr(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, graph.ErrSuspended):
		return "suspended"
	case errors.Is(err, graph.ErrPermanent):
		return "permanent"
	}
	// 默认走 llm 的分类——LLM 错误 sentinel 在 llm 包里定义。
	return llm.ClassifyChatErr(err)
}

// truncate 与 llm.truncate 相同语义但放在本包内避免跨包引用包私有函数。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
