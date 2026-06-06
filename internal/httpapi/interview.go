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
var ErrInvalidSessionState = errors.New("invalid session state")

// interviewRunner 是 InterviewService 看到的 Graph 边界。
//
// HTTP 层不关心 Graph 里有多少节点，只需要区分：
//   - Invoke：新 Session 首次推进到第一个暂停点；
//   - Resume：用户提交回答后，从暂停点继续推进。
type interviewRunner interface {
	Invoke(ctx context.Context, sess *domain.Session) error
	Resume(ctx context.Context, sess *domain.Session) error
}

// SessionCoordinator 是跨实例会话协调边界。
//
// 生产路径通常由 Redis 实现，用来保存短期 snapshot 和 mutation lease。
// 它不替代 SessionStore；PG/内存 store 仍然是 Session 的事实源。
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

// InterviewService 是 HTTP handler 和 Agent Graph 之间的应用服务层。
//
// handler 只负责 HTTP 协议和响应转换；Start/Answer 必须经过这里，
// 统一收口 Session 生命周期、lease、snapshot、持久化、长期记忆和事件发布。
type InterviewService struct {
	runner                interviewRunner   // Graph 执行入口：负责 Invoke/Resume，不暴露具体节点给 HTTP 层。
	store                 SessionStore      // Session 事实源：本地 demo 用内存，PG 模式用 row_version 做最终写保护。
	events                InterviewEventHub // 面试事件总线：驱动 SSE snapshot/replay/progress。
	memoryStore           memory.Store      // 长期记忆存储：Start 前 hydrate，完成后 persist。
	memoryPersistObserver longTermMemoryPersistObserver
	coordinator           SessionCoordinator // 可选跨实例协调器：Redis lease/snapshot；为空时退化成本地模式。
	ownerID               string             // 当前服务实例标识，用于 lease owner 校验和释放保护。
	leaseTTL              time.Duration      // mutation lease TTL，防止单个实例长时间独占 Session。
	snapshotTTL           time.Duration      // Redis snapshot TTL，用于断线恢复和降级读取。
	memoryMu              sync.Mutex         // 串行化长期记忆持久化，避免同一用户画像并发覆盖。
	mu                    sync.Mutex         // 保护本地自增 session id；只用于无外部 ID 的 demo/test 路径。
	nextID                int
}

// SetMemoryStore 注入长期记忆存储。
//
// 启动装配层会根据是否有 PG 选择 memory.Store 实现；测试也可以注入内存实现。
func (s *InterviewService) SetMemoryStore(store memory.Store) {
	s.memoryStore = store
}

// NewInterviewService 构造默认本地服务。
//
// 这个入口服务测试和本地 demo：Session、事件都走内存实现，不依赖 PG/Redis。
func NewInterviewService(runner interviewRunner) *InterviewService {
	return NewInterviewServiceWithStoreAndEvents(runner, NewMemorySessionStore(), NewMemoryInterviewEventHub(64))
}

// NewInterviewServiceWithStore 允许替换 SessionStore，但事件仍走内存 hub。
func NewInterviewServiceWithStore(runner interviewRunner, store SessionStore) *InterviewService {
	return NewInterviewServiceWithStoreAndEvents(runner, store, NewMemoryInterviewEventHub(64))
}

// NewInterviewServiceWithStoreAndEvents 允许同时替换 SessionStore 和事件总线。
func NewInterviewServiceWithStoreAndEvents(runner interviewRunner, store SessionStore, events InterviewEventHub) *InterviewService {
	return NewInterviewServiceWithStoreEventsAndCoordinator(runner, store, events, nil, "")
}

// NewInterviewServiceWithStoreEventsAndCoordinator 是完整构造入口。
//
// cmd/server 装配层会在这里注入 PG store、Redis event hub/coordinator 和 ownerID。
// nil store/events 会回退到内存实现，保证测试和本地模式不需要外部依赖。
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
