package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func (s *InterviewService) Start(ctx context.Context, req startInterviewRequest) (*domain.Session, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("%w: interview runner not configured", graph.ErrInvalidConfig)
	}
	id := req.SessionID
	if id == "" {
		s.mu.Lock()
		s.nextID++
		id = fmt.Sprintf("sess-%d", s.nextID)
		s.mu.Unlock()
	}
	now := time.Now()
	sess := &domain.Session{
		ID:        id,
		UserID:    req.UserID,
		Mode:      normalizeInterviewMode(req.Mode),
		Status:    domain.StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
		JobProfile: &domain.JobProfile{
			JDRawText: req.JDText,
		},
		CandProfile: &domain.CandidateProfile{
			ResumeRawText: req.ResumeText,
		},
		QuestionBankFilter: cloneQuestionBankFilter(req.QuestionBankFilter),
		WorkingMemory:      domain.NewWorkingMemory(),
	}
	leaseAcquired := false
	if err := s.acquireSessionLease(ctx, sess.ID); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		return nil, err
	}
	if s.coordinator != nil {
		leaseAcquired = true
	}
	releaseOnFailure := func() {
		if leaseAcquired {
			_, _ = s.coordinator.ReleaseLease(context.Background(), sess.ID, s.ownerID)
		}
	}
	if err := s.runner.Invoke(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		releaseOnFailure()
		return nil, err
	}

	if err := s.store.Save(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		releaseOnFailure()
		return nil, err
	}
	if s.saveSessionSnapshot(ctx, sess) {
		if err := s.store.Save(ctx, sess); err != nil {
			s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
			releaseOnFailure()
			return nil, err
		}
	}
	s.publishEvent(ctx, interviewEventSessionCreated, sess, "", "")
	return sess, nil
}

func cloneQuestionBankFilter(filter *domain.QuestionBankFilter) *domain.QuestionBankFilter {
	if filter == nil {
		return nil
	}
	out := &domain.QuestionBankFilter{
		SkillCategories: compactInterviewStrings(filter.SkillCategories),
		Scenarios:       compactInterviewStrings(filter.Scenarios),
		DifficultyMin:   normalizeScopeDifficulty(filter.DifficultyMin),
		DifficultyMax:   normalizeScopeDifficulty(filter.DifficultyMax),
		Tags:            compactInterviewStrings(filter.Tags),
	}
	if out.DifficultyMin > 0 && out.DifficultyMax > 0 && out.DifficultyMin > out.DifficultyMax {
		out.DifficultyMin, out.DifficultyMax = out.DifficultyMax, out.DifficultyMin
	}
	if len(out.SkillCategories) == 0 && len(out.Scenarios) == 0 && out.DifficultyMin == 0 && out.DifficultyMax == 0 && len(out.Tags) == 0 {
		return nil
	}
	return out
}

func compactInterviewStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeScopeDifficulty(n int) int {
	if n < 1 || n > 5 {
		return 0
	}
	return n
}

func (s *InterviewService) Answer(ctx context.Context, req answerInterviewRequest) (*domain.Session, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("%w: interview runner not configured", graph.ErrInvalidConfig)
	}

	sess, err := s.getSessionForMutation(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if req.UserID != "" && sess.UserID != req.UserID {
		return nil, fmt.Errorf("session %q not found", req.SessionID)
	}
	if err := s.renewSessionLease(ctx, sess.ID); err != nil {
		return nil, err
	}
	if err := fillPendingAnswer(sess, req.Answer); err != nil {
		return nil, err
	}
	if err := s.runner.Resume(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		return nil, err
	}
	sess.UpdatedAt = nextUpdatedAt(sess.UpdatedAt)
	if err := s.store.Save(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		return nil, err
	}
	if s.saveSessionSnapshot(ctx, sess) {
		if err := s.store.Save(ctx, sess); err != nil {
			s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
			return nil, err
		}
	}
	if sess.Status == domain.StatusCompleted {
		_ = s.releaseSessionLease(ctx, sess.ID)
	}
	eventType := interviewEventSessionUpdated
	if sess.Status == domain.StatusCompleted {
		eventType = interviewEventSessionCompleted
	}
	s.publishEvent(ctx, eventType, sess, "", "")
	return sess, nil
}

func nextUpdatedAt(prev time.Time) time.Time {
	now := time.Now()
	if now.After(prev) {
		return now
	}
	return prev.Add(time.Nanosecond)
}

func fillPendingAnswer(sess *domain.Session, answer string) error {
	switch sess.CurrentNode {
	case "pick_next":
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("no current round for answer")
		}
		round.Answer = answer
		return nil
	case "probe_ask":
		round := sess.CurrentRound()
		if round == nil || len(round.FollowUps) == 0 {
			return fmt.Errorf("no current follow-up for answer")
		}
		round.FollowUps[len(round.FollowUps)-1].Answer = answer
		return nil
	default:
		return fmt.Errorf("session %q is not waiting for answer at node %q", sess.ID, sess.CurrentNode)
	}
}

func (s *InterviewService) publishEvent(ctx context.Context, eventType string, sess *domain.Session, node, errMsg string) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(ctx, buildInterviewEvent(eventType, sess, node, errMsg))
}
