package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/memory"
)

const longTermMemoryWriteMaxAttempts = 3

type longTermMemoryPersistStatus string

const (
	longTermMemoryPersistSuccess                longTermMemoryPersistStatus = "success"
	longTermMemoryPersistSkipped                longTermMemoryPersistStatus = "skipped"
	longTermMemoryPersistFailed                 longTermMemoryPersistStatus = "failed"
	longTermMemoryPersistConflictRetryExhausted longTermMemoryPersistStatus = "conflict_retry_exhausted"
)

type longTermMemoryPersistObservation struct {
	UserID     string
	SessionID  string
	Status     longTermMemoryPersistStatus
	Reason     string
	ErrorClass string
	Attempts   int
	Elapsed    time.Duration
}

type longTermMemoryPersistObserver func(context.Context, longTermMemoryPersistObservation)

func (s *InterviewService) persistLongTermMemory(ctx context.Context, sess *domain.Session) error {
	started := time.Now()
	if s == nil {
		observeLongTermMemoryPersist(ctx, nil, longTermMemoryPersistObservation{
			Status:  longTermMemoryPersistSkipped,
			Reason:  "service_missing",
			Elapsed: elapsedLongTermMemoryPersist(started),
		})
		return nil
	}
	if s.memoryStore == nil {
		s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
			UserID:    longTermMemoryObservationUserID(sess),
			SessionID: longTermMemoryObservationSessionID(sess),
			Status:    longTermMemoryPersistSkipped,
			Reason:    "store_missing",
			Elapsed:   elapsedLongTermMemoryPersist(started),
		})
		return nil
	}
	if sess == nil {
		s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
			Status:  longTermMemoryPersistSkipped,
			Reason:  "session_missing",
			Elapsed: elapsedLongTermMemoryPersist(started),
		})
		return nil
	}
	if sess.Report == nil {
		s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
			UserID:    strings.TrimSpace(sess.UserID),
			SessionID: sess.ID,
			Status:    longTermMemoryPersistSkipped,
			Reason:    "report_missing",
			Elapsed:   elapsedLongTermMemoryPersist(started),
		})
		return nil
	}
	update, err := memory.BuildUpdateFromSession(sess, time.Now())
	if err != nil {
		status := longTermMemoryPersistFailed
		reason := ""
		errorClass := "build_update_failed"
		if strings.TrimSpace(sess.UserID) == "" {
			status = longTermMemoryPersistSkipped
			reason = "user_missing"
			errorClass = ""
		}
		s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
			UserID:     strings.TrimSpace(sess.UserID),
			SessionID:  sess.ID,
			Status:     status,
			Reason:     reason,
			ErrorClass: errorClass,
			Elapsed:    elapsedLongTermMemoryPersist(started),
		})
		return err
	}
	s.memoryMu.Lock()
	defer s.memoryMu.Unlock()
	var lastConflict error
	for attempt := 0; attempt < longTermMemoryWriteMaxAttempts; attempt++ {
		current, err := s.memoryStore.GetUserMemory(ctx, update.UserID)
		if err != nil && !errors.Is(err, memory.ErrUserMemoryNotFound) {
			s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
				UserID:     update.UserID,
				SessionID:  sess.ID,
				Status:     longTermMemoryPersistFailed,
				ErrorClass: "store_read_failed",
				Attempts:   attempt + 1,
				Elapsed:    elapsedLongTermMemoryPersist(started),
			})
			return err
		}
		if errors.Is(err, memory.ErrUserMemoryNotFound) {
			current = nil
		}
		next, err := memory.ApplyUpdate(current, update)
		if err != nil {
			s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
				UserID:     update.UserID,
				SessionID:  sess.ID,
				Status:     longTermMemoryPersistFailed,
				ErrorClass: "apply_update_failed",
				Attempts:   attempt + 1,
				Elapsed:    elapsedLongTermMemoryPersist(started),
			})
			return err
		}
		err = s.memoryStore.UpsertUserMemory(ctx, next)
		if err == nil {
			s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
				UserID:    update.UserID,
				SessionID: sess.ID,
				Status:    longTermMemoryPersistSuccess,
				Attempts:  attempt + 1,
				Elapsed:   elapsedLongTermMemoryPersist(started),
			})
			return nil
		}
		if !errors.Is(err, memory.ErrUserMemoryConflict) {
			s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
				UserID:     update.UserID,
				SessionID:  sess.ID,
				Status:     longTermMemoryPersistFailed,
				ErrorClass: "store_write_failed",
				Attempts:   attempt + 1,
				Elapsed:    elapsedLongTermMemoryPersist(started),
			})
			return err
		}
		lastConflict = err
	}
	s.observeLongTermMemoryPersist(ctx, longTermMemoryPersistObservation{
		UserID:     update.UserID,
		SessionID:  sess.ID,
		Status:     longTermMemoryPersistConflictRetryExhausted,
		ErrorClass: "cas_conflict",
		Attempts:   longTermMemoryWriteMaxAttempts,
		Elapsed:    elapsedLongTermMemoryPersist(started),
	})
	return lastConflict
}

func (s *InterviewService) observeLongTermMemoryPersist(ctx context.Context, ev longTermMemoryPersistObservation) {
	observer := s.memoryPersistObserver
	observeLongTermMemoryPersist(ctx, observer, ev)
}

func observeLongTermMemoryPersist(ctx context.Context, observer longTermMemoryPersistObserver, ev longTermMemoryPersistObservation) {
	slog.DebugContext(ctx, "long-term memory persist",
		"status", string(ev.Status),
		"reason", ev.Reason,
		"error_class", ev.ErrorClass,
		"attempts", ev.Attempts,
		"elapsed_ms", ev.Elapsed.Milliseconds(),
	)
	if observer == nil {
		return
	}
	observer(ctx, ev)
}

func longTermMemoryObservationUserID(sess *domain.Session) string {
	if sess == nil {
		return ""
	}
	return strings.TrimSpace(sess.UserID)
}

func longTermMemoryObservationSessionID(sess *domain.Session) string {
	if sess == nil {
		return ""
	}
	return sess.ID
}

func elapsedLongTermMemoryPersist(started time.Time) time.Duration {
	elapsed := time.Since(started)
	if elapsed <= 0 {
		return time.Nanosecond
	}
	return elapsed
}

func (s *InterviewService) hydrateWorkingMemoryFromLongTermMemory(ctx context.Context, sess *domain.Session) {
	if s == nil || s.memoryStore == nil || sess == nil || sess.WorkingMemory == nil {
		return
	}
	userID := strings.TrimSpace(sess.UserID)
	if userID == "" {
		return
	}
	longTerm, err := s.memoryStore.GetUserMemory(ctx, userID)
	if err != nil {
		if !errors.Is(err, memory.ErrUserMemoryNotFound) {
			markInterviewMemoryDegraded(sess.WorkingMemory, "memory", fmt.Sprintf("long-term memory read failed: %v", err))
		}
		return
	}
	if longTerm == nil {
		return
	}
	applyLongTermMemoryToWorkingMemory(sess.WorkingMemory, longTerm)
}

func applyLongTermMemoryToWorkingMemory(mem *domain.WorkingMemory, longTerm *memory.UserMemory) {
	if mem == nil || longTerm == nil {
		return
	}
	for _, weakness := range longTerm.Weaknesses {
		topic := strings.TrimSpace(weakness.Topic)
		if topic == "" {
			continue
		}
		mem.WeakSkills = appendUniqueInterviewMemoryString(mem.WeakSkills, topic)
	}
	if mem.SkillCoverage == nil {
		mem.SkillCoverage = map[string]float64{}
	}
	for skill, score := range longTerm.SkillScores {
		skill = strings.TrimSpace(skill)
		if skill == "" || score >= 60 {
			continue
		}
		coverage := score / 100
		if coverage < 0 {
			coverage = 0
		}
		if coverage > 1 {
			coverage = 1
		}
		if current, ok := mem.SkillCoverage[skill]; !ok || coverage < current {
			mem.SkillCoverage[skill] = coverage
		}
	}
}

func markInterviewMemoryDegraded(mem *domain.WorkingMemory, component, reason string) {
	if mem == nil {
		return
	}
	if mem.DegradedReasons == nil {
		mem.DegradedReasons = map[string]string{}
	}
	mem.DegradedReasons[component] = reason
}

func appendUniqueInterviewMemoryString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
