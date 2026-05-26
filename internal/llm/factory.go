package llm

import (
	"fmt"

	"interview-agent/internal/config"
)

// BuildChatModel 装配 LLM 客户端链路。
// 多个入口（cmd/server、cmd/demo）共享这套 wiring，避免漂移。
//
// real 模式链路（外→内）：BreakingChatModel → LimitedChatModel → RealChatModel。
// 熔断器放最外层的原因：open 时要直接 fail-fast，不能先去抢 limiter 槽位。
//
// 第二个返回值 breakerState 是熔断器状态查询函数，给 /readyz 用；
// mock 模式为 nil，调用方应跳过 breaker 字段。
func BuildChatModel(cfg *config.Config) (ChatModel, func() string, error) {
	switch cfg.LLM.Mode {
	case "mock":
		return NewMockChatModel(""), nil, nil
	case "real":
		m := NewRealChatModel(cfg.LLM.BaseURL, cfg.LLMAPIKey, cfg.LLM.Model, cfg.LLM.Timeout)
		if cfg.LLM.MaxRetries > 0 {
			m.MaxRetries = cfg.LLM.MaxRetries
		}
		limited := NewLimitedChatModel(m, cfg.LLM.MaxConcurrency)
		breaker := NewBreakingChatModel(limited, cfg.LLM.BreakerFailureThreshold, cfg.LLM.BreakerOpenDuration)
		// 若 NewBreakingChatModel 因配置非法回退到 inner（理论上 validate 已挡，这里 defensive），
		// State() 查询函数也跟着退化为 nil，调用方不上报 breaker 字段。
		if bm, ok := breaker.(*BreakingChatModel); ok {
			return bm, bm.State, nil
		}
		return breaker, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported llm mode %q", cfg.LLM.Mode)
	}
}
