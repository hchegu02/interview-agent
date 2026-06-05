package httpapi

import (
	"context"
	"errors"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/memory"
)

func (s *InterviewService) persistLongTermMemory(ctx context.Context, sess *domain.Session) error {
	if s == nil || s.memoryStore == nil || sess == nil || sess.Report == nil {
		return nil
	}
	update, err := memory.BuildUpdateFromSession(sess, time.Now())
	if err != nil {
		return err
	}
	s.memoryMu.Lock()
	defer s.memoryMu.Unlock()
	current, err := s.memoryStore.GetUserMemory(ctx, update.UserID)
	if err != nil && !errors.Is(err, memory.ErrUserMemoryNotFound) {
		return err
	}
	if errors.Is(err, memory.ErrUserMemoryNotFound) {
		current = nil
	}
	next, err := memory.ApplyUpdate(current, update)
	if err != nil {
		return err
	}
	return s.memoryStore.UpsertUserMemory(ctx, next)
}
