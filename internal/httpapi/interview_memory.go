package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
