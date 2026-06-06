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
	// Start 只负责创建会话和推进首轮 Graph。真正的面试状态都写进 domain.Session，
	// 这样内存存储、PG 存储和事件快照看到的是同一份数据结构。
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
	// 分布式锁只在 coordinator 存在时生效。拿锁后如果后续任一步失败，必须释放，
	// 否则同一个 session 会被错误地卡成“正在处理”。
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
	// saveSessionSnapshot 可能把 Graph 产生的快照写回 Session，所以需要再 Save 一次。
	// 这里宁可多一次持久化，也不要让恢复流程读到半截状态。
	if s.saveSessionSnapshot(ctx, sess) {
		if err := s.store.Save(ctx, sess); err != nil {
			s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
			releaseOnFailure()
			return nil, err
		}
	}
	if leaseAcquired && shouldReleaseMutationLease(sess) {
		_ = s.releaseSessionLease(ctx, sess.ID)
		leaseAcquired = false
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

	// Answer 是“取会话 -> 填答案 -> Resume Graph -> 保存”的窄通道。
	// 不在 handler 里改 Session，是为了保证所有入口都走同一套并发和持久化规则。
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
		_ = s.persistLongTermMemory(ctx, sess)
	}
	if shouldReleaseMutationLease(sess) {
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
	// Suspension 是新恢复语义；CurrentNode 只作为旧 Session 兼容回退。
	node, err := answerAwaitingNode(sess)
	if err != nil {
		return err
	}
	switch node {
	case "pick_next":
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("%w: no current round for answer", ErrInvalidSessionState)
		}
		round.Answer = answer
		return nil
	case "probe_ask":
		round := sess.CurrentRound()
		if round == nil || len(round.FollowUps) == 0 {
			return fmt.Errorf("%w: no current follow-up for answer", ErrInvalidSessionState)
		}
		round.FollowUps[len(round.FollowUps)-1].Answer = answer
		return nil
	default:
		return fmt.Errorf("%w: session %q is not waiting for answer at node %q", ErrInvalidSessionState, sess.ID, node)
	}
}

func answerAwaitingNode(sess *domain.Session) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("%w: nil session", ErrInvalidSessionState)
	}
	if sess.Suspension != nil {
		if sess.Suspension.Awaiting != "" && sess.Suspension.Awaiting != domain.SuspensionAwaitingAnswer {
			return "", fmt.Errorf("%w: session %q is awaiting %q", ErrInvalidSessionState, sess.ID, sess.Suspension.Awaiting)
		}
		if sess.Suspension.Node != "" {
			return sess.Suspension.Node, nil
		}
	}
	return sess.CurrentNode, nil
}

func shouldReleaseMutationLease(sess *domain.Session) bool {
	if sess == nil {
		return false
	}
	return sess.Status == domain.StatusCompleted || sess.Suspension != nil
}

func (s *InterviewService) publishEvent(ctx context.Context, eventType string, sess *domain.Session, node, errMsg string) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(ctx, buildInterviewEventWithContext(ctx, eventType, sess, node, errMsg))
}
