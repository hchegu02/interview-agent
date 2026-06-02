package httpapi

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"interview-agent/internal/domain"
)

var ErrSessionNotFound = errors.New("session not found")

// SessionStore 是 HTTP 面试服务看到的会话存储边界。
//
// 实现可以是内存 map，也可以是 PG state_json；上层不关心具体介质。
// 这里故意只暴露面试接口真正需要的方法，避免 handler 绕过 InterviewService 直接改状态。
type SessionStore interface {
	Save(ctx context.Context, sess *domain.Session) error
	Get(ctx context.Context, id string) (*domain.Session, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error)
	DeleteForUser(ctx context.Context, id, userID string) error
}

const (
	// 列表接口必须有上限。会话对象包含 rounds/report，单条 JSON 可能不小；
	// 不限制 limit 会让一个用户的历史会话把 HTTP 响应和数据库查询拖垮。
	defaultSessionListLimit = 20
	maxSessionListLimit     = 100
)

func normalizeSessionListLimit(limit int) int {
	if limit <= 0 {
		return defaultSessionListLimit
	}
	if limit > maxSessionListLimit {
		return maxSessionListLimit
	}
	return limit
}

type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{sessions: map[string]*domain.Session{}}
}

func (s *MemorySessionStore) Save(ctx context.Context, sess *domain.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("session store: session id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 内存实现保存的是指针，不做深拷贝；它只服务本地 demo/测试。
	// 生产路径应使用 PG store，让进程重启和多实例场景都能恢复。
	s.sessions[sess.ID] = sess
	return nil
}

func (s *MemorySessionStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrSessionNotFound, id)
	}
	// 读取时迁移旧快照，保证老版本 state_json 也能被新代码继续推进。
	sess.MigrateLegacyState()
	return sess, nil
}

func (s *MemorySessionStore) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit = normalizeSessionListLimit(limit)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*domain.Session, 0)
	for _, sess := range s.sessions {
		if sess.UserID != userID {
			continue
		}
		// 列表接口也会触发迁移，否则用户点开历史会话前看到的摘要可能还是旧字段。
		sess.MigrateLegacyState()
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemorySessionStore) DeleteForUser(ctx context.Context, id, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok || sess.UserID != userID {
		return fmt.Errorf("%w: %q", ErrSessionNotFound, id)
	}
	delete(s.sessions, id)
	return nil
}
