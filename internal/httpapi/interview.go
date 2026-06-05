package httpapi

import (
	"context"
	"errors"
	"sync"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/memory"
)

const (
	defaultSessionLeaseTTL    = 30 * time.Second
	defaultSessionSnapshotTTL = 24 * time.Hour
	sessionLeaseRetryAfter    = time.Second

	// 短重试用于吸收"实例 A 刚 crash / lease 还差几十毫秒过期"这种瞬时冲突，
	// 不替代客户端按 Retry-After 的长重试——上限设得很紧避免占着 HTTP 连接。
	sessionLeaseAcquireMaxAttempts = 3
	sessionLeaseAcquireBaseBackoff = 25 * time.Millisecond
	sessionLeaseAcquireMaxBackoff  = 100 * time.Millisecond
	sessionLeaseAcquireDeadline    = 250 * time.Millisecond
)

var ErrSessionLeaseConflict = errors.New("session lease conflict")

type interviewRunner interface {
	Invoke(ctx context.Context, sess *domain.Session) error
	Resume(ctx context.Context, sess *domain.Session) error
}

type SessionCoordinator interface {
	SaveSnapshot(ctx context.Context, sess *domain.Session, ttl time.Duration) error
	LoadSnapshot(ctx context.Context, sessionID string) (*domain.Session, error)
	AcquireLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error)
	RenewLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error)
	ReleaseLease(ctx context.Context, sessionID, ownerID string) (bool, error)
}

type sessionSnapshotDeleter interface {
	DeleteSnapshot(ctx context.Context, sessionID string) error
}

type InterviewService struct {
	runner      interviewRunner
	store       SessionStore
	events      InterviewEventHub
	memoryStore memory.Store
	coordinator SessionCoordinator
	ownerID     string
	leaseTTL    time.Duration
	snapshotTTL time.Duration
	memoryMu    sync.Mutex
	mu          sync.Mutex
	nextID      int
}

func (s *InterviewService) SetMemoryStore(store memory.Store) {
	s.memoryStore = store
}

func NewInterviewService(runner interviewRunner) *InterviewService {
	return NewInterviewServiceWithStoreAndEvents(runner, NewMemorySessionStore(), NewMemoryInterviewEventHub(64))
}

func NewInterviewServiceWithStore(runner interviewRunner, store SessionStore) *InterviewService {
	return NewInterviewServiceWithStoreAndEvents(runner, store, NewMemoryInterviewEventHub(64))
}

func NewInterviewServiceWithStoreAndEvents(runner interviewRunner, store SessionStore, events InterviewEventHub) *InterviewService {
	return NewInterviewServiceWithStoreEventsAndCoordinator(runner, store, events, nil, "")
}

func NewInterviewServiceWithStoreEventsAndCoordinator(runner interviewRunner, store SessionStore, events InterviewEventHub, coordinator SessionCoordinator, ownerID string) *InterviewService {
	if store == nil {
		store = NewMemorySessionStore()
	}
	if events == nil {
		events = NewMemoryInterviewEventHub(64)
	}
	if ownerID == "" {
		ownerID = "local"
	}
	return &InterviewService{
		runner:      runner,
		store:       store,
		events:      events,
		coordinator: coordinator,
		ownerID:     ownerID,
		leaseTTL:    defaultSessionLeaseTTL,
		snapshotTTL: defaultSessionSnapshotTTL,
	}
}
