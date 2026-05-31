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

type SessionStore interface {
	Save(ctx context.Context, sess *domain.Session) error
	Get(ctx context.Context, id string) (*domain.Session, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error)
	DeleteForUser(ctx context.Context, id, userID string) error
}

const (
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
