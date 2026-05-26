package graph

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"interview-agent/internal/domain"
)

// Decorator 是 NodeFunc → NodeFunc 的函数变换。
// 经典装饰器模式，便于横切关注点（重试、超时、日志、指标、Schema 校验）解耦。
type Decorator func(NodeFunc) NodeFunc

// Compose 把多个 Decorator 叠在一起，第一个 Decorator 是最外层。
//
// 例如 Compose(WithRetry(3), WithTimeout(5s))(fn) 的执行顺序：
//
//	WithRetry 外层 → WithTimeout 内层 → fn
//
// 这意味着 retry 看到的是"已带 timeout 的调用"——
// 每次重试都重新启动一个 5s 超时，符合直觉。
//
// 反过来的顺序（WithTimeout 外层 → WithRetry 内层）意味着所有重试共享
// 一个 5s 总预算，也合理但语义不同。所以 Compose 顺序是有意义的，
// 调用方按需调整。
func Compose(decorators ...Decorator) Decorator {
	return func(fn NodeFunc) NodeFunc {
		for i := len(decorators) - 1; i >= 0; i-- {
			fn = decorators[i](fn)
		}
		return fn
	}
}

// RetryConfig 控制重试行为。
//
// 重试只对"可重试错误"生效——节点用 graph.ErrPermanent 包裹错误
// 表示不重试。这是个明确的契约，避免重试装饰器"猜哪个错该重试"。
type RetryConfig struct {
	MaxAttempts int           // 总尝试次数（含首次），默认 3
	BaseDelay   time.Duration // 首次重试前的延迟，默认 200ms
	MaxDelay    time.Duration // 单次重试延迟上限，默认 5s
	JitterRatio float64       // 抖动比例 0~1，默认 0.25
}

func (c RetryConfig) withDefaults() RetryConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = 200 * time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 5 * time.Second
	}
	if c.JitterRatio < 0 || c.JitterRatio > 1 {
		c.JitterRatio = 0.25
	}
	return c
}

// WithRetry 给节点加指数退避 + jitter 重试。
//
// 退避公式：delay = min(BaseDelay * 2^attempt, MaxDelay) ± Jitter
//
// jitter 的作用：当多个并发请求同时失败时，错开重试时间避免雪崩。
// 这是分布式系统经典模式，AWS / Google SRE Book 都强调过。
func WithRetry(cfg RetryConfig) Decorator {
	cfg = cfg.withDefaults()

	return func(next NodeFunc) NodeFunc {
		return func(ctx context.Context, sess *domain.Session) error {
			var lastErr error
			for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
				if err := ctx.Err(); err != nil {
					return err
				}

				err := next(ctx, sess)
				if err == nil {
					return nil
				}
				if errors.Is(err, ErrPermanent) {
					return err // 不可重试，立即返回
				}
				lastErr = err

				// 最后一次失败不再 sleep
				if attempt == cfg.MaxAttempts-1 {
					break
				}

				delay := backoff(cfg, attempt)
				if err := sleepCtx(ctx, delay); err != nil {
					return err
				}
			}
			return fmt.Errorf("retry exhausted after %d attempts: %w", cfg.MaxAttempts, lastErr)
		}
	}
}

// backoff 计算第 attempt 次重试前的延迟（attempt 从 0 起计）。
// 这个函数提出来便于单测验证延迟序列。
func backoff(cfg RetryConfig, attempt int) time.Duration {
	d := cfg.BaseDelay * (1 << attempt) // 2^attempt
	if d > cfg.MaxDelay {
		d = cfg.MaxDelay
	}
	if cfg.JitterRatio > 0 {
		jitterRange := float64(d) * cfg.JitterRatio
		// rand.Int63n 不能为 0
		if jitterRange > 1 {
			jitter := time.Duration(rand.Int63n(int64(jitterRange)))
			// 50% 概率正负
			if rand.Intn(2) == 0 {
				d += jitter
			} else if d > jitter {
				d -= jitter
			}
		}
	}
	return d
}

// WithTimeout 给节点加上下文超时。
//
// 注意是装饰器层级的超时——每次重试都会启动一个新 timeout。
// 如果要"所有重试共享一个总超时"，Compose 时把 WithTimeout 放在 WithRetry 外层。
func WithTimeout(d time.Duration) Decorator {
	return func(next NodeFunc) NodeFunc {
		return func(ctx context.Context, sess *domain.Session) error {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, sess)
		}
	}
}

// Permanent 把任意 error 包成 ErrPermanent，标记为不可重试。
// 节点用法：return graph.Permanent(fmt.Errorf("malformed JD: %v", err))
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrPermanent, err)
}

// sleepCtx 是带 ctx 取消的 sleep。
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
