package llm

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// BreakingChatModel 在 ChatModel 之外再包一层熔断器。
//
// 部署位置（关键）：必须放在 LimitedChatModel 之外。原因——
// open 状态下要立刻 fail-fast，不能先去抢 limiter 槽位；
// 否则限流通过的请求被熔断挡回，等于把 limiter 当成黑洞。
// 推荐链路：RealChatModel → LimitedChatModel → BreakingChatModel。
//
// 状态机：
//
//	closed   失败计数 < threshold
//	         ─── 第 threshold 次 ErrTransient/DeadlineExceeded ──> open
//	open     openedAt + openDuration 内全部 fail-fast
//	         ─── 时间到 ──> halfOpen
//	halfOpen 同一时间只允许 1 个 probe 请求穿透
//	         probe 成功 ──> closed（计数清零）
//	         probe 失败 ──> open（openedAt 重置）
//	         其它并发请求 ──> fail-fast（不参与 probe）
//
// 计入失败：errors.Is(err, ErrTransient) 或 errors.Is(err, context.DeadlineExceeded)。
// 不计入失败：ErrPermanent（4xx 配置/参数错）、ErrSchemaInvalid（模型输出问题）、
// context.Canceled（客户端断）、ErrBreakerOpen（自身 fail-fast，避免自激）。
type BreakingChatModel struct {
	inner        ChatModel
	threshold    int
	openDuration time.Duration

	// now 可注入，便于测试用虚拟时钟跨越 openDuration 而不真 sleep。
	now func() time.Time

	mu                  sync.Mutex
	state               breakerState
	consecutiveFailures int
	openedAt            time.Time
	halfOpenInFlight    bool
}

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

func (s breakerState) String() string {
	switch s {
	case breakerClosed:
		return "closed"
	case breakerOpen:
		return "open"
	case breakerHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// NewBreakingChatModel 构造熔断装饰器。
//   - inner 为 nil 时直接返回 nil（呼应 NewLimitedChatModel 的"无 inner 不装饰"约定）。
//   - threshold <= 0 或 openDuration <= 0 时跳过熔断，inner 透传——
//     这样上层即便配置漏填也不会全断。validate() 已在 real 模式硬校验，
//     这里只是 defensive。
func NewBreakingChatModel(inner ChatModel, threshold int, openDuration time.Duration) ChatModel {
	if inner == nil {
		return nil
	}
	if threshold <= 0 || openDuration <= 0 {
		return inner
	}
	return &BreakingChatModel{
		inner:        inner,
		threshold:    threshold,
		openDuration: openDuration,
		now:          time.Now,
		state:        breakerClosed,
	}
}

// Name 透传。Metrics 维度仍使用底层模型名。
func (m *BreakingChatModel) Name() string {
	return m.inner.Name()
}

// State 返回当前状态字符串，供 /readyz 报告。读锁保护。
func (m *BreakingChatModel) State() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 进入 open 后到 openedAt+openDuration 之间外部看到的应该仍是 open。
	// halfOpen 只在尝试 acquire 时短暂出现，外部不必关心。
	return m.state.String()
}

func (m *BreakingChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	allowed, isProbe := m.tryAcquire(ctx)
	if !allowed {
		return nil, ErrBreakerOpen
	}

	resp, err := m.inner.Generate(ctx, messages, opts)
	m.recordResult(ctx, err, isProbe)
	return resp, err
}

// Stream 同 Generate，但 chunk 流式错误较难精确捕获——
// 现实路径里 agent loop 走 Generate，Stream 主要给 SSE。先做最小可用：
// 启动失败按一次失败计入，启动成功后流内错误暂不计入。
func (m *BreakingChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	allowed, isProbe := m.tryAcquire(ctx)
	if !allowed {
		return nil, ErrBreakerOpen
	}

	ch, err := m.inner.Stream(ctx, messages, opts)
	if err != nil {
		m.recordResult(ctx, err, isProbe)
		return nil, err
	}
	// 启动成功：以 probe 视角，假设流能正常结束。
	// 真实失败需要等读到 Chunk{Err: ...} 时由调用方判断；这里先记录成功。
	m.recordResult(ctx, nil, isProbe)
	return ch, nil
}

// tryAcquire 返回 (是否允许穿透, 是否 probe 探针)。
//   - closed：永远允许，非 probe
//   - open 且未到 openDuration：拒绝
//   - open 且已到 openDuration：原子地转 halfOpen，允许 probe；后续并发拒绝
//   - halfOpen 且没有 in-flight probe：允许 probe（fallback 路径，极少触发）
//   - halfOpen 且已有 probe：拒绝
func (m *BreakingChatModel) tryAcquire(ctx context.Context) (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case breakerClosed:
		return true, false
	case breakerOpen:
		if m.now().Sub(m.openedAt) < m.openDuration {
			return false, false
		}
		// open → halfOpen，发出第一个 probe
		m.transitionLocked(ctx, breakerHalfOpen, "open_duration_elapsed")
		m.halfOpenInFlight = true
		return true, true
	case breakerHalfOpen:
		if m.halfOpenInFlight {
			return false, false
		}
		m.halfOpenInFlight = true
		return true, true
	default:
		return false, false
	}
}

// recordResult 在请求结束时更新熔断器内部状态。
func (m *BreakingChatModel) recordResult(ctx context.Context, err error, isProbe bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if isProbe {
		m.halfOpenInFlight = false
	}

	count := m.shouldCount(err)

	if isProbe {
		// halfOpen 下的 probe 结果决定回 closed 还是回 open。
		if count {
			m.openedAt = m.now()
			m.transitionLocked(ctx, breakerOpen, "half_open_probe_failed")
			// consecutiveFailures 保持原值（已经在阈值或之上），等下一次 probe。
		} else {
			m.consecutiveFailures = 0
			m.transitionLocked(ctx, breakerClosed, "half_open_probe_succeeded")
		}
		return
	}

	// closed 路径
	if count {
		m.consecutiveFailures++
		if m.consecutiveFailures >= m.threshold && m.state == breakerClosed {
			m.openedAt = m.now()
			m.transitionLocked(ctx, breakerOpen, "threshold_reached")
		}
		return
	}
	// 不计入失败的错误（含成功）：重置计数。
	// 注意：ErrPermanent 也清零——一次永久错不意味着 provider 在持续抽风，
	// 只是这一条请求配置不对；继续给下个请求公平机会。
	m.consecutiveFailures = 0
}

func (m *BreakingChatModel) transitionLocked(ctx context.Context, to breakerState, reason string) {
	if m.state == to {
		return
	}
	from := m.state
	m.state = to
	// slog 走 ctx，trace_id 自动附带；不强依赖 ctx。
	if ctx == nil {
		ctx = context.Background()
	}
	slog.InfoContext(ctx, "llm breaker transition",
		"event", "breaker_transition",
		"from", from.String(),
		"to", to.String(),
		"reason", reason,
		"consecutive_failures", m.consecutiveFailures,
	)
}

// shouldCount 决定一个错误是否算"provider 在抽风"。
// 详细分类见类型注释。
func (m *BreakingChatModel) shouldCount(err error) bool {
	if err == nil {
		return false
	}
	// 自身 fail-fast 不算（防自激）。
	if errors.Is(err, ErrBreakerOpen) {
		return false
	}
	// 客户端主动取消不算。
	if errors.Is(err, context.Canceled) {
		return false
	}
	// 永久配置错不算（一次 401 不该把全进程拉黑）。
	if errors.Is(err, ErrPermanent) {
		return false
	}
	// 模型输出 schema 错不算（业务校验失败，不是 provider 问题）。
	if errors.Is(err, ErrSchemaInvalid) {
		return false
	}
	// 计入：传输/服务端层面的瞬时错。
	if errors.Is(err, ErrTransient) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// 未分类错误：保守地不计入，避免把"业务 bug"放大成熔断。
	// 调用方应该用 ErrTransient/ErrPermanent 显式分类。
	return false
}

// ensure interface satisfied at compile time
var _ ChatModel = (*BreakingChatModel)(nil)
