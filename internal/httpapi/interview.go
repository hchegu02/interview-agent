package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
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

type InterviewService struct {
	runner      interviewRunner
	store       SessionStore
	events      InterviewEventHub
	coordinator SessionCoordinator
	ownerID     string
	leaseTTL    time.Duration
	snapshotTTL time.Duration
	mu          sync.Mutex
	nextID      int
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

type startInterviewRequest struct {
	SessionID  string `json:"session_id"`
	UserID     string `json:"user_id"`
	Mode       string `json:"mode"`
	JDText     string `json:"jd_text" binding:"required"`
	ResumeText string `json:"resume_text" binding:"required"`
}

type answerInterviewRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	UserID    string `json:"user_id"`
	Answer    string `json:"answer"`
}

type interviewResponse struct {
	SessionID        string                   `json:"session_id"`
	UserID           string                   `json:"user_id,omitempty"`
	Mode             string                   `json:"mode"`
	Status           string                   `json:"status"`
	Phase            string                   `json:"phase"`
	Progress         []interviewProgressStep  `json:"progress"`
	JobProfile       *domain.JobProfile       `json:"job_profile,omitempty"`
	CandidateProfile *domain.CandidateProfile `json:"candidate_profile,omitempty"`
	ProfileAnalysis  *domain.ProfileAnalysis  `json:"profile_analysis,omitempty"`
	Question         *interviewQuestion       `json:"question,omitempty"`
	Rounds           []interviewRound         `json:"rounds,omitempty"`
	Report           *domain.Report           `json:"report,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

type interviewProgressStep struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type interviewQuestion struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	Tags           []string `json:"tags,omitempty"`
	Difficulty     int      `json:"difficulty,omitempty"`
	SkillCategory  string   `json:"skill_category,omitempty"`
	ExpectedPoints []string `json:"expected_points,omitempty"`
}

type interviewRound struct {
	RoundID   string              `json:"round_id"`
	Number    int                 `json:"number"`
	Question  interviewQuestion   `json:"question"`
	Answer    string              `json:"answer,omitempty"`
	FollowUps []interviewFollowUp `json:"follow_ups,omitempty"`
	Feedback  *interviewFeedback  `json:"feedback,omitempty"`
	Completed bool                `json:"completed"`
}

type interviewFollowUp struct {
	Question string             `json:"question"`
	Answer   string             `json:"answer,omitempty"`
	Feedback *interviewFeedback `json:"feedback,omitempty"`
}

type interviewFeedback struct {
	Score          int      `json:"score"`
	HitPoints      []string `json:"hit_points,omitempty"`
	MissedPoints   []string `json:"missed_points,omitempty"`
	Suggestion     string   `json:"suggestion,omitempty"`
	ExpectedPoints []string `json:"expected_points,omitempty"`
}

func (s *Server) startInterview(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	var req startInterviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess, err := s.interview.Start(c.Request.Context(), req)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, buildInterviewResponse(sess))
}

func (s *Server) answerInterview(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	var req answerInterviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess, err := s.interview.Answer(c.Request.Context(), req)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, buildInterviewResponse(sess))
}

func (s *Server) listInterviewSessions(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		limit = n
	}
	limit = normalizeSessionListLimit(limit)
	sessions, err := s.interview.ListByUser(c.Request.Context(), userID, limit)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	out := make([]interviewResponse, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, buildInterviewResponse(sess))
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

func (s *Server) getInterviewSession(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	sess, err := s.interview.Get(c.Request.Context(), sessionID)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	if userID := c.Query("user_id"); userID != "" && sess.UserID != userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("session %q not found", sessionID)})
		return
	}
	c.JSON(http.StatusOK, buildInterviewResponse(sess))
}

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
		WorkingMemory: domain.NewWorkingMemory(),
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

func (s *InterviewService) acquireSessionLease(ctx context.Context, sessionID string) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	ok, err := s.retryAcquireLease(ctx, func(c context.Context) (bool, error) {
		return s.coordinator.AcquireLease(c, sessionID, s.ownerID, s.leaseTTL)
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: session %q is owned by another instance", ErrSessionLeaseConflict, sessionID)
	}
	return nil
}

// retryAcquireLease 在 (false, nil) 瞬时冲突上做指数退避 + 抖动短重试，
// 真错误（attempt 返回 non-nil err）立刻冒泡不重试。
// 总耗时上限由 sessionLeaseAcquireDeadline 控制：若下一个 sleep 会超过总预算就停下，
// 让调用方把 (false, nil) 翻成 ErrSessionLeaseConflict，由客户端按 Retry-After 长重试。
// ctx 取消会立刻终止 sleep 并把 ctx.Err() 冒泡到上层。
func (s *InterviewService) retryAcquireLease(ctx context.Context, attempt func(context.Context) (bool, error)) (bool, error) {
	start := time.Now()
	delay := sessionLeaseAcquireBaseBackoff

	for i := 0; i < sessionLeaseAcquireMaxAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		ok, err := attempt(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if i == sessionLeaseAcquireMaxAttempts-1 {
			break
		}

		sleep := jitteredBackoff(delay)
		if time.Since(start)+sleep > sessionLeaseAcquireDeadline {
			break
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}

		delay *= 2
		if delay > sessionLeaseAcquireMaxBackoff {
			delay = sessionLeaseAcquireMaxBackoff
		}
	}
	return false, nil
}

// jitteredBackoff 返回 [d/2, d) 之间的随机 sleep 时长，d <= 0 时直接返回 0。
// full-jitter 风格能在多实例并发冲突时把重试时刻打散，避免雪崩同步。
func jitteredBackoff(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	half := int64(d / 2)
	if half <= 0 {
		return d
	}
	return time.Duration(half) + time.Duration(rand.Int63n(half))
}

func (s *InterviewService) renewSessionLease(ctx context.Context, sessionID string) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	ok, err := s.coordinator.RenewLease(ctx, sessionID, s.ownerID, s.leaseTTL)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.acquireSessionLease(ctx, sessionID)
}

func (s *InterviewService) saveSessionSnapshot(ctx context.Context, sess *domain.Session) bool {
	if s == nil || s.coordinator == nil {
		return false
	}
	if err := s.coordinator.SaveSnapshot(ctx, sess, s.snapshotTTL); err != nil {
		markSessionDegraded(sess, "redis_snapshot", err.Error())
		return true
	}
	return false
}

func (s *InterviewService) releaseSessionLease(ctx context.Context, sessionID string) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	_, err := s.coordinator.ReleaseLease(ctx, sessionID, s.ownerID)
	return err
}

func markSessionDegraded(sess *domain.Session, component, reason string) {
	if sess == nil {
		return
	}
	if sess.WorkingMemory == nil {
		sess.WorkingMemory = domain.NewWorkingMemory()
	}
	if sess.WorkingMemory.DegradedReasons == nil {
		sess.WorkingMemory.DegradedReasons = map[string]string{}
	}
	sess.WorkingMemory.DegradedReasons[component] = reason
}

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

func buildInterviewResponse(sess *domain.Session) interviewResponse {
	mode := sessionMode(sess)
	return interviewResponse{
		SessionID:        sess.ID,
		UserID:           sess.UserID,
		Mode:             mode,
		Status:           string(sess.Status),
		Phase:            interviewPhase(sess),
		Progress:         interviewProgress(sess),
		JobProfile:       cloneJobProfile(sess.JobProfile),
		CandidateProfile: cloneCandidateProfile(sess.CandProfile),
		ProfileAnalysis:  cloneProfileAnalysis(sess.ProfileAnalysis),
		Question:         buildInterviewQuestion(currentQuestion(sess), false),
		Rounds:           buildInterviewRounds(sess, mode),
		Report:           cloneReport(sess.Report),
		CreatedAt:        sess.CreatedAt,
		UpdatedAt:        sess.UpdatedAt,
	}
}

func currentQuestion(sess *domain.Session) *domain.Question {
	if sess.Report != nil {
		return nil
	}
	round := sess.CurrentRound()
	if round == nil {
		return nil
	}
	if sess.CurrentNode == "probe_ask" && len(round.FollowUps) > 0 {
		last := round.FollowUps[len(round.FollowUps)-1]
		return &domain.Question{
			ID:      round.Question.ID + "-followup",
			Content: last.Question,
			Tags:    round.Question.Tags,
			Source:  "probe",
		}
	}
	return &round.Question
}

func normalizeInterviewMode(mode string) string {
	if mode == "practice" {
		return "practice"
	}
	return "exam"
}

func sessionMode(sess *domain.Session) string {
	if sess == nil {
		return "exam"
	}
	return normalizeInterviewMode(sess.Mode)
}

func shouldExposeFeedback(sess *domain.Session, mode string) bool {
	return mode == "practice" || (sess != nil && sess.Status == domain.StatusCompleted)
}

func buildInterviewQuestion(q *domain.Question, includeExpected bool) *interviewQuestion {
	if q == nil {
		return nil
	}
	out := &interviewQuestion{
		ID:            q.ID,
		Content:       q.Content,
		Tags:          append([]string(nil), q.Tags...),
		Difficulty:    q.Difficulty,
		SkillCategory: q.SkillCategory,
	}
	if includeExpected {
		out.ExpectedPoints = append([]string(nil), q.ExpectedPoints...)
	}
	return out
}

func buildInterviewRounds(sess *domain.Session, mode string) []interviewRound {
	if sess == nil || len(sess.Rounds) == 0 {
		return nil
	}
	exposeFeedback := shouldExposeFeedback(sess, mode)
	out := make([]interviewRound, 0, len(sess.Rounds))
	for i := range sess.Rounds {
		round := sess.Rounds[i]
		q := buildInterviewQuestion(&round.Question, exposeFeedback)
		if q == nil {
			continue
		}
		item := interviewRound{
			RoundID:   round.RoundID,
			Number:    i + 1,
			Question:  *q,
			Answer:    round.Answer,
			Completed: !round.CompletedAt.IsZero() || round.FinalEvaluation() != nil,
		}
		if exposeFeedback {
			item.Feedback = buildInterviewFeedback(round.FinalEvaluation(), round.Question.ExpectedPoints)
		}
		if len(round.FollowUps) > 0 {
			item.FollowUps = make([]interviewFollowUp, 0, len(round.FollowUps))
			for _, follow := range round.FollowUps {
				fu := interviewFollowUp{
					Question: follow.Question,
					Answer:   follow.Answer,
				}
				if exposeFeedback {
					fu.Feedback = buildInterviewFeedback(follow.Evaluation, nil)
				}
				item.FollowUps = append(item.FollowUps, fu)
			}
		}
		out = append(out, item)
	}
	return out
}

func buildInterviewFeedback(eval *domain.Evaluation, expected []string) *interviewFeedback {
	if eval == nil {
		return nil
	}
	return &interviewFeedback{
		Score:          eval.Score,
		HitPoints:      append([]string(nil), eval.Strengths...),
		MissedPoints:   append([]string(nil), eval.Weaknesses...),
		Suggestion:     eval.Suggestion,
		ExpectedPoints: append([]string(nil), expected...),
	}
}

func interviewPhase(sess *domain.Session) string {
	if sess == nil {
		return "preparing"
	}
	switch sess.Status {
	case domain.StatusCompleted:
		return "completed"
	case domain.StatusFailed:
		return "failed"
	}
	if currentQuestion(sess) != nil {
		return "answering"
	}
	switch sess.CurrentNode {
	case "parse_jd", "parse_resume", "gap_analyze", "analyze_profile", "retrieve_rag", "":
		return "preparing"
	case "report":
		return "reporting"
	default:
		return "evaluating"
	}
}

func interviewProgress(sess *domain.Session) []interviewProgressStep {
	steps := []interviewProgressStep{
		{Key: "jd", Label: "JD 分析"},
		{Key: "resume", Label: "简历匹配"},
		{Key: "rag", Label: "题库检索"},
		{Key: "question", Label: "出题规划"},
		{Key: "interview", Label: "面试进行"},
		{Key: "report", Label: "评估报告"},
	}
	phase := interviewPhase(sess)
	current := 0
	switch phase {
	case "preparing":
		current = 1
		if sess != nil && sess.GapReport != nil {
			current = 2
		}
		if sess != nil && sess.ProfileAnalysis != nil {
			current = 2
		}
		if sess != nil && len(sess.CandidatePool) > 0 {
			current = 3
		}
	case "answering", "evaluating":
		current = 4
	case "reporting":
		current = 5
	case "completed":
		current = len(steps)
	case "failed":
		current = 4
	}
	for i := range steps {
		switch {
		case phase == "failed" && i == current:
			steps[i].Status = "error"
		case i < current:
			steps[i].Status = "done"
		case i == current:
			steps[i].Status = "current"
		default:
			steps[i].Status = "pending"
		}
	}
	return steps
}

func (s *InterviewService) publishEvent(ctx context.Context, eventType string, sess *domain.Session, node, errMsg string) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(ctx, buildInterviewEvent(eventType, sess, node, errMsg))
}

func writeInterviewError(c *gin.Context, err error) {
	switch {
	case err == nil:
		c.Status(http.StatusOK)
	case errors.Is(err, graph.ErrInvalidConfig):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "面试服务暂不可用，请稍后重试"})
	case errors.Is(err, ErrSessionLeaseConflict):
		retryAfterSeconds := int(sessionLeaseRetryAfter.Seconds())
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		c.JSON(http.StatusConflict, gin.H{
			"error":               "当前会话正在处理中，请稍后重试",
			"retry_after_seconds": retryAfterSeconds,
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求无法处理，请检查会话状态后重试"})
	}
}
