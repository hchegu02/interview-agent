package httpapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"interview-agent/internal/domain"
)

// sequencedSessionCoordinator 是单测专用的 coordinator stub：
// AcquireLease / RenewLease 按 results 顺序返回；用完后默认返回 (true, nil)，
// 这样在 Answer 等多步路径里"成功之后的后续调用"不会因 results 用完而失败。
type sequencedSessionCoordinator struct {
	acquireResults []leaseResult
	renewResults   []leaseResult
	snapshot       *domain.Session

	acquireCalls int
	renewCalls   int
	saved        []string
	released     []string
}

type leaseResult struct {
	ok  bool
	err error
}

func (c *sequencedSessionCoordinator) SaveSnapshot(ctx context.Context, sess *domain.Session, ttl time.Duration) error {
	c.saved = append(c.saved, sess.ID+":"+string(sess.Status))
	return nil
}

func (c *sequencedSessionCoordinator) LoadSnapshot(ctx context.Context, sessionID string) (*domain.Session, error) {
	if c.snapshot == nil || c.snapshot.ID != sessionID {
		return nil, fmt.Errorf("snapshot %q not found", sessionID)
	}
	return c.snapshot, nil
}

func (c *sequencedSessionCoordinator) AcquireLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error) {
	c.acquireCalls++
	if len(c.acquireResults) == 0 {
		return true, nil
	}
	r := c.acquireResults[0]
	c.acquireResults = c.acquireResults[1:]
	return r.ok, r.err
}

func (c *sequencedSessionCoordinator) RenewLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error) {
	c.renewCalls++
	if len(c.renewResults) == 0 {
		return true, nil
	}
	r := c.renewResults[0]
	c.renewResults = c.renewResults[1:]
	return r.ok, r.err
}

func (c *sequencedSessionCoordinator) ReleaseLease(ctx context.Context, sessionID, ownerID string) (bool, error) {
	c.released = append(c.released, sessionID+":"+ownerID)
	return true, nil
}

func TestInterviewService_StartRetriesTransientLeaseConflict(t *testing.T) {
	coord := &sequencedSessionCoordinator{
		acquireResults: []leaseResult{
			{ok: false}, {ok: false}, {ok: true},
		},
	}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-retry-ok",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.ID != "lease-retry-ok" {
		t.Fatalf("session id = %q", sess.ID)
	}
	if coord.acquireCalls != 3 {
		t.Fatalf("acquireCalls = %d, want 3 (2 conflicts + 1 success)", coord.acquireCalls)
	}
}

func TestInterviewService_StartFailsAfterMaxRetries(t *testing.T) {
	coord := &sequencedSessionCoordinator{
		acquireResults: []leaseResult{
			{ok: false}, {ok: false}, {ok: false},
		},
	}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	start := time.Now()
	_, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-retry-exhausted",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrSessionLeaseConflict) {
		t.Fatalf("err = %v, want ErrSessionLeaseConflict", err)
	}
	if coord.acquireCalls != sessionLeaseAcquireMaxAttempts {
		t.Fatalf("acquireCalls = %d, want %d", coord.acquireCalls, sessionLeaseAcquireMaxAttempts)
	}
	// 总耗时不该超过 deadline + 一点 schedule 抖动。
	if elapsed > sessionLeaseAcquireDeadline+200*time.Millisecond {
		t.Fatalf("elapsed = %v, exceeded deadline %v", elapsed, sessionLeaseAcquireDeadline)
	}
}

func TestInterviewService_StartPropagatesCoordinatorErrorImmediately(t *testing.T) {
	redisDown := errors.New("redis down")
	coord := &sequencedSessionCoordinator{
		acquireResults: []leaseResult{
			{ok: false, err: redisDown},
			{ok: true}, // 不应被调到
		},
	}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	_, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-coord-err",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if !errors.Is(err, redisDown) {
		t.Fatalf("err = %v, want wrapping redisDown", err)
	}
	if errors.Is(err, ErrSessionLeaseConflict) {
		t.Fatalf("coordinator error should not be mapped to lease conflict")
	}
	if coord.acquireCalls != 1 {
		t.Fatalf("acquireCalls = %d, want 1 (no retry on real error)", coord.acquireCalls)
	}
}

func TestInterviewService_AnswerRenewFallbackRetriesAcquire(t *testing.T) {
	// 先 seed 一个已经 running 的 session，模拟 Start 已完成、需要 Answer 续期的场景。
	store := NewMemorySessionStore()
	seed := &domain.Session{
		ID:            "lease-renew-fallback",
		UserID:        "u1",
		Status:        domain.StatusRunning,
		CurrentNode:   "pick_next",
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", Content: "Q"},
		}},
	}
	if err := store.Save(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	coord := &sequencedSessionCoordinator{
		// renew 直接冲突 → fallback 到 acquire
		renewResults: []leaseResult{{ok: false}},
		// acquire 前 2 次冲突，第 3 次成功
		acquireResults: []leaseResult{{ok: false}, {ok: false}, {ok: true}},
	}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		store,
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	sess, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "lease-renew-fallback",
		UserID:    "u1",
		Answer:    "ok",
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if sess.Status != domain.StatusCompleted {
		t.Fatalf("status = %q, want completed", sess.Status)
	}
	if coord.renewCalls != 1 {
		t.Fatalf("renewCalls = %d, want 1 (renew not retried)", coord.renewCalls)
	}
	if coord.acquireCalls != 3 {
		t.Fatalf("acquireCalls = %d, want 3 (fallback retried 2 conflicts + 1 success)", coord.acquireCalls)
	}
}

func TestJitteredBackoff_RangeAndZero(t *testing.T) {
	if got := jitteredBackoff(0); got != 0 {
		t.Fatalf("jitteredBackoff(0) = %v, want 0", got)
	}
	if got := jitteredBackoff(-time.Millisecond); got != 0 {
		t.Fatalf("jitteredBackoff(-1ms) = %v, want 0", got)
	}
	// 多次采样确认始终落在 [d/2, d)
	d := 80 * time.Millisecond
	for i := 0; i < 200; i++ {
		got := jitteredBackoff(d)
		if got < d/2 || got >= d {
			t.Fatalf("jitteredBackoff(%v) = %v, out of [%v, %v)", d, got, d/2, d)
		}
	}
}
