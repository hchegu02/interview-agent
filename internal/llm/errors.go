package llm

import (
	"errors"
	"fmt"
)

// 错误分类是 RealChatModel 重试策略的核心契约。
//
// 设计原则：
//   - ErrTransient   = 短暂错误（429/5xx/网络抖动/超时）→ 应该重试
//   - ErrPermanent   = 永久错误（4xx 业务错、模型不存在、key 失效）→ 立即返回
//
// 不能让"重试装饰器自己猜哪个错该重试"——猜错了要么放大故障（重试 401 把 quota 打爆），
// 要么误判正常（把 502 当 permanent 直接挂掉）。
// 用 sentinel error + errors.Is 校验，让调用方一眼看到契约。
var (
	// ErrTransient 标记可重试错误。RealChatModel 内部用。
	ErrTransient = errors.New("llm: transient error")

	// ErrPermanent 标记不可重试错误。
	// 上层 Graph 看到这个错误应该立即终止节点，不要再 retry。
	ErrPermanent = errors.New("llm: permanent error")

	// ErrSchemaInvalid 标记 LLM 返回的 JSON 不符合 schema。
	// CallWithSchema 用这个错误触发"自纠正"重试。
	ErrSchemaInvalid = errors.New("llm: response schema invalid")

	// ErrEmptyResponse 模型返回空 content。罕见但要 defensive。
	ErrEmptyResponse = errors.New("llm: empty response content")

	// ErrBreakerOpen 表示熔断器处于 open 状态，立刻 fail-fast，没有真去调底层模型。
	//
	// 故意 *不* wrap ErrTransient：上游若按"重试 ErrTransient"的策略递归调，
	// 会在 open 期内反复重试，把限流/退避全部抵消。节点层的 markDegradedReason
	// 路径对任何错误都走规则降级，所以零侵入节点。
	ErrBreakerOpen = errors.New("llm: breaker open")
)

// classifyHTTPStatus 把 HTTP 状态码映射到错误类型。
//   - 200~299  → nil
//   - 429      → Transient（限流，退避后能恢复）
//   - 500~599  → Transient（服务端临时故障）
//   - 408      → Transient（请求超时，可重试）
//   - 其他 4xx → Permanent（401 认证失败、400 参数错误、404 模型不存在）
//
// 把这个抽出来是为了单测好覆盖每个分支。
func classifyHTTPStatus(status int, bodyHint string) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == 429:
		return fmt.Errorf("%w: 429 rate limited (%s)", ErrTransient, truncate(bodyHint, 200))
	case status == 408:
		return fmt.Errorf("%w: 408 request timeout (%s)", ErrTransient, truncate(bodyHint, 200))
	case status >= 500 && status < 600:
		return fmt.Errorf("%w: %d server error (%s)", ErrTransient, status, truncate(bodyHint, 200))
	default:
		return fmt.Errorf("%w: %d client error (%s)", ErrPermanent, status, truncate(bodyHint, 200))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
