package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func (s *InterviewService) Subscribe(ctx context.Context, sessionID string, afterID string) (<-chan InterviewEvent, func(), error) {
	if s.events == nil {
		return nil, func() {}, fmt.Errorf("%w: event hub not configured", graph.ErrInvalidConfig)
	}
	return s.events.Subscribe(ctx, sessionID, afterID)
}

func (s *InterviewService) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error) {
	if s.store == nil {
		return nil, fmt.Errorf("%w: session store not configured", graph.ErrInvalidConfig)
	}
	return s.store.ListByUser(ctx, userID, limit)
}

func (s *InterviewService) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	if s.store == nil {
		return nil, fmt.Errorf("%w: session store not configured", graph.ErrInvalidConfig)
	}
	sess, err := s.store.Get(ctx, sessionID)
	if err == nil {
		return sess, nil
	}
	if !errors.Is(err, ErrSessionNotFound) || s.coordinator == nil {
		return nil, err
	}
	sess, err = s.coordinator.LoadSnapshot(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *InterviewService) GetForUser(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	sess, err := s.Get(ctx, sessionID)
	if err == nil {
		if userID != "" && sess.UserID != userID {
			return nil, fmt.Errorf("%w: %q", ErrSessionNotFound, sessionID)
		}
		return sess, nil
	}
	if userID == "" || s.store == nil {
		return nil, err
	}

	// PG 点查偶发失败时，列表读路径仍可能已经能看到同一用户的最新会话。
	// 读详情不能因为一次瞬时点查错误让已完成报告不可见。
	sessions, listErr := s.store.ListByUser(ctx, userID, maxSessionListLimit)
	if listErr != nil {
		return nil, err
	}
	for _, candidate := range sessions {
		if candidate != nil && candidate.ID == sessionID {
			return candidate, nil
		}
	}
	return nil, err
}

func (s *InterviewService) DeleteForUser(ctx context.Context, sessionID, userID string) error {
	if s.store == nil {
		return fmt.Errorf("%w: session store not configured", graph.ErrInvalidConfig)
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user_id is required")
	}
	if err := s.store.DeleteForUser(ctx, sessionID, userID); err != nil {
		return err
	}
	if s.coordinator != nil {
		_, _ = s.coordinator.ReleaseLease(ctx, sessionID, s.ownerID)
		if deleter, ok := s.coordinator.(sessionSnapshotDeleter); ok {
			_ = deleter.DeleteSnapshot(ctx, sessionID)
		}
	}
	return nil
}

func (s *InterviewService) getSessionForMutation(ctx context.Context, sessionID string) (*domain.Session, error) {
	if s.store == nil {
		return nil, fmt.Errorf("%w: session store not configured", graph.ErrInvalidConfig)
	}
	sess, err := s.store.Get(ctx, sessionID)
	if err == nil {
		return sess, nil
	}
	if !errors.Is(err, ErrSessionNotFound) || s.coordinator == nil {
		return nil, err
	}
	return s.coordinator.LoadSnapshot(ctx, sessionID)
}
