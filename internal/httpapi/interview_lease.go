package httpapi

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"interview-agent/internal/domain"
)

func (s *InterviewService) acquireSessionLease(ctx context.Context, sessionID string) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	ok, err := s.retryAcquireLease(ctx, func(c context.Context) (bool, error) {
		return s.coordinator.AcquireLease(c, sessionID, s.ownerID, s.leaseTTL)
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: session %q is owned by another instance", ErrSessionLeaseConflict, sessionID)
	}
	return nil
}

// retryAcquireLease 在 (false, nil) 瞬时冲突上做指数退避 + 抖动短重试，
// 真错误（attempt 返回 non-nil err）立刻冒泡不重试。
// 总耗时上限由 sessionLeaseAcquireDeadline 控制：若下一个 sleep 会超过总预算就停下，
// 让调用方把 (false, nil) 翻成 ErrSessionLeaseConflict，由客户端按 Retry-After 长重试。
// ctx 取消会立刻终止 sleep 并把 ctx.Err() 冒泡到上层。
func (s *InterviewService) retryAcquireLease(ctx context.Context, attempt func(context.Context) (bool, error)) (bool, error) {
	start := time.Now()
	delay := sessionLeaseAcquireBaseBackoff

	for i := 0; i < sessionLeaseAcquireMaxAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		ok, err := attempt(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if i == sessionLeaseAcquireMaxAttempts-1 {
			break
		}

		sleep := jitteredBackoff(delay)
		if time.Since(start)+sleep > sessionLeaseAcquireDeadline {
			break
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}

		delay *= 2
		if delay > sessionLeaseAcquireMaxBackoff {
			delay = sessionLeaseAcquireMaxBackoff
		}
	}
	return false, nil
}

// jitteredBackoff 返回 [d/2, d) 之间的随机 sleep 时长，d <= 0 时直接返回 0。
// full-jitter 风格能在多实例并发冲突时把重试时刻打散，避免雪崩同步。
func jitteredBackoff(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	half := int64(d / 2)
	if half <= 0 {
		return d
	}
	return time.Duration(half) + time.Duration(rand.Int63n(half))
}

func (s *InterviewService) renewSessionLease(ctx context.Context, sessionID string) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	ok, err := s.coordinator.RenewLease(ctx, sessionID, s.ownerID, s.leaseTTL)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.acquireSessionLease(ctx, sessionID)
}

func (s *InterviewService) saveSessionSnapshot(ctx context.Context, sess *domain.Session) bool {
	if s == nil || s.coordinator == nil {
		return false
	}
	if err := s.coordinator.SaveSnapshot(ctx, sess, s.snapshotTTL); err != nil {
		markSessionDegraded(sess, "redis_snapshot", err.Error())
		return true
	}
	return false
}

func (s *InterviewService) releaseSessionLease(ctx context.Context, sessionID string) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	_, err := s.coordinator.ReleaseLease(ctx, sessionID, s.ownerID)
	return err
}

func markSessionDegraded(sess *domain.Session, component, reason string) {
	if sess == nil {
		return
	}
	if sess.WorkingMemory == nil {
		sess.WorkingMemory = domain.NewWorkingMemory()
	}
	if sess.WorkingMemory.DegradedReasons == nil {
		sess.WorkingMemory.DegradedReasons = map[string]string{}
	}
	sess.WorkingMemory.DegradedReasons[component] = reason
}
