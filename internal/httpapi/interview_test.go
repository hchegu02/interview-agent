package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/config"
	"interview-agent/internal/domain"
	"interview-agent/internal/memory"
	"interview-agent/pkg/traceid"
)

type fakeInterviewRunner struct{}

func (fakeInterviewRunner) Invoke(ctx context.Context, sess *domain.Session) error {
	sess.CurrentNode = "pick_next"
	sess.Rounds = append(sess.Rounds, domain.AnswerRound{
		RoundID: "r1",
		Question: domain.Question{
			ID:      "q1",
			Content: "讲一下 Go GMP",
			Tags:    []string{"go"},
		},
	})
	return nil
}

func (fakeInterviewRunner) Resume(ctx context.Context, sess *domain.Session) error {
	if round := sess.CurrentRound(); round != nil {
		round.Evaluation = &domain.Evaluation{
			QuestionID: round.Question.ID,
			Score:      80,
			Strengths:  []string{"覆盖了 Go 基础"},
			Weaknesses: []string{"调度细节不足"},
			Suggestion: "补充 GMP 协作过程",
		}
		round.CompletedAt = time.Now()
	}
	sess.Status = domain.StatusCompleted
	sess.Report = &domain.Report{
		SessionID:      sess.ID,
		OverallScore:   80,
		SkillBreakdown: map[string]int{"go": 80},
		Highlights:     []string{"Go 基础清楚"},
		Improvements:   []string{"补充调度细节"},
		NextSteps:      []string{"继续练习并发题"},
	}
	return nil
}

type missingReportRunner struct{}

func (missingReportRunner) Invoke(ctx context.Context, sess *domain.Session) error {
	return fakeInterviewRunner{}.Invoke(ctx, sess)
}

func (missingReportRunner) Resume(ctx context.Context, sess *domain.Session) error {
	if round := sess.CurrentRound(); round != nil {
		round.CompletedAt = time.Now()
	}
	sess.Status = domain.StatusCompleted
	sess.Report = nil
	return nil
}

type suspendingInterviewRunner struct{}

func (suspendingInterviewRunner) Invoke(ctx context.Context, sess *domain.Session) error {
	sess.CurrentNode = "pick_next"
	sess.Suspension = &domain.Suspension{
		Node:      "pick_next",
		Awaiting:  domain.SuspensionAwaitingAnswer,
		CreatedAt: time.Now(),
	}
	sess.Rounds = append(sess.Rounds, domain.AnswerRound{
		RoundID:  "r1",
		Question: domain.Question{ID: "q1", Content: "Q"},
	})
	return nil
}

func (suspendingInterviewRunner) Resume(ctx context.Context, sess *domain.Session) error {
	sess.CurrentNode = "probe_ask"
	sess.Suspension = &domain.Suspension{
		Node:      "probe_ask",
		Awaiting:  domain.SuspensionAwaitingAnswer,
		CreatedAt: time.Now(),
	}
	if round := sess.CurrentRound(); round != nil {
		round.FollowUps = append(round.FollowUps, domain.FollowUp{Question: "追问"})
	}
	return nil
}

type failingMemoryStore struct{}

func (failingMemoryStore) GetUserMemory(ctx context.Context, userID string) (*memory.UserMemory, error) {
	return nil, memory.ErrUserMemoryNotFound
}

func (failingMemoryStore) UpsertUserMemory(ctx context.Context, memory *memory.UserMemory) error {
	return errors.New("memory store failed")
}

type delayingMemoryStore struct {
	mu       sync.Mutex
	gets     int
	memory   *memory.UserMemory
	firstHit chan struct{}
}

func newDelayingMemoryStore() *delayingMemoryStore {
	return &delayingMemoryStore{firstHit: make(chan struct{}, 1)}
}

func (s *delayingMemoryStore) GetUserMemory(ctx context.Context, userID string) (*memory.UserMemory, error) {
	s.mu.Lock()
	s.gets++
	isFirst := s.gets == 1 && s.memory == nil
	if isFirst {
		s.firstHit <- struct{}{}
		s.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return nil, memory.ErrUserMemoryNotFound
	}
	mem := cloneTestUserMemory(s.memory)
	s.mu.Unlock()
	if mem == nil {
		return nil, memory.ErrUserMemoryNotFound
	}
	return mem, nil
}

func (s *delayingMemoryStore) UpsertUserMemory(ctx context.Context, mem *memory.UserMemory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memory = cloneTestUserMemory(mem)
	return nil
}

func cloneTestUserMemory(in *memory.UserMemory) *memory.UserMemory {
	if in == nil {
		return nil
	}
	out := *in
	out.Strengths = append([]string(nil), in.Strengths...)
	out.Weaknesses = append([]memory.Weakness(nil), in.Weaknesses...)
	out.LastAdvice = append([]string(nil), in.LastAdvice...)
	out.SkillScores = map[string]float64{}
	for k, v := range in.SkillScores {
		out.SkillScores[k] = v
	}
	return &out
}

type fakeSessionCoordinator struct {
	acquireOK bool
	renewOK   bool
	snapshot  *domain.Session
	saveErr   error

	acquired []string
	renewed  []string
	saved    []string
	released []string
}

func (c *fakeSessionCoordinator) SaveSnapshot(ctx context.Context, sess *domain.Session, ttl time.Duration) error {
	c.saved = append(c.saved, sess.ID+":"+string(sess.Status))
	return c.saveErr
}

func (c *fakeSessionCoordinator) LoadSnapshot(ctx context.Context, sessionID string) (*domain.Session, error) {
	if c.snapshot == nil || c.snapshot.ID != sessionID {
		return nil, fmt.Errorf("snapshot %q not found", sessionID)
	}
	return c.snapshot, nil
}

func (c *fakeSessionCoordinator) AcquireLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error) {
	c.acquired = append(c.acquired, sessionID+":"+ownerID)
	return c.acquireOK, nil
}

func (c *fakeSessionCoordinator) RenewLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error) {
	c.renewed = append(c.renewed, sessionID+":"+ownerID)
	return c.renewOK, nil
}

func (c *fakeSessionCoordinator) ReleaseLease(ctx context.Context, sessionID, ownerID string) (bool, error) {
	c.released = append(c.released, sessionID+":"+ownerID)
	return true, nil
}

type flakyReadSessionStore struct {
	getErr   error
	sessions []*domain.Session
}

func (s *flakyReadSessionStore) Save(ctx context.Context, sess *domain.Session) error {
	s.sessions = append(s.sessions, sess)
	return nil
}

func (s *flakyReadSessionStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, sess := range s.sessions {
		if sess.ID == id {
			return sess, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrSessionNotFound, id)
}

func (s *flakyReadSessionStore) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error) {
	var out []*domain.Session
	for _, sess := range s.sessions {
		if sess.UserID == userID {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *flakyReadSessionStore) DeleteForUser(ctx context.Context, id, userID string) error {
	return fmt.Errorf("%w: %q", ErrSessionNotFound, id)
}

func TestInterviewStart_ReturnsFirstQuestion(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)

	body := bytes.NewBufferString(`{
		"session_id":"s1",
		"user_id":"u1",
		"jd_text":"需要 Go 后端",
		"resume_text":"两年 Go 经验"
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace-lease-start")

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got interviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SessionID != "s1" {
		t.Errorf("session_id = %q, want s1", got.SessionID)
	}
	if got.Question == nil || got.Question.ID != "q1" {
		t.Fatalf("question = %+v, want q1", got.Question)
	}
	if got.Mode != "exam" {
		t.Fatalf("mode = %q, want default exam", got.Mode)
	}
	if got.Phase != "answering" {
		t.Fatalf("phase = %q, want answering", got.Phase)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps should be returned: %+v", got)
	}
	if got.Report != nil {
		t.Fatalf("report should be empty before answer, got %+v", got.Report)
	}
}

func TestInterviewService_StartStoresQuestionBankFilter(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})

	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "scope-start",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
		QuestionBankFilter: &domain.QuestionBankFilter{
			SkillCategories: []string{"redis", "go"},
			Scenarios:       []string{"troubleshooting"},
			DifficultyMin:   2,
			DifficultyMax:   4,
			Tags:            []string{"cache", "performance"},
		},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.QuestionBankFilter == nil {
		t.Fatal("question bank filter should be stored on session")
	}
	if got := sess.QuestionBankFilter.SkillCategories; len(got) != 2 || got[0] != "redis" || got[1] != "go" {
		t.Fatalf("skill categories = %+v, want [redis go]", got)
	}
	if got := sess.QuestionBankFilter.Scenarios; len(got) != 1 || got[0] != "troubleshooting" {
		t.Fatalf("scenarios = %+v, want [troubleshooting]", got)
	}
	if sess.QuestionBankFilter.DifficultyMin != 2 || sess.QuestionBankFilter.DifficultyMax != 4 {
		t.Fatalf("difficulty range = %d..%d, want 2..4", sess.QuestionBankFilter.DifficultyMin, sess.QuestionBankFilter.DifficultyMax)
	}
	if got := sess.QuestionBankFilter.Tags; len(got) != 2 || got[0] != "cache" || got[1] != "performance" {
		t.Fatalf("tags = %+v, want [cache performance]", got)
	}
}

func TestInterviewResponse_DoesNotExposeInternalRuntimeNames(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)

	body := bytes.NewBufferString(`{
		"session_id":"safe-dto",
		"user_id":"u1",
		"jd_text":"需要 Go 后端",
		"resume_text":"两年 Go 经验"
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace-lease-start")

	server.Router().ServeHTTP(rec, req)

	raw := rec.Body.String()
	for _, forbidden := range []string{"current_node", "pick_next", "probe_ask", "graph.node"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, raw)
		}
	}
}

func TestInterviewService_StartAcquiresLeaseAndSavesSnapshot(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-start",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.ID != "lease-start" {
		t.Fatalf("session id = %q", sess.ID)
	}
	if len(coord.acquired) != 1 || coord.acquired[0] != "lease-start:owner-a" {
		t.Fatalf("acquired = %+v", coord.acquired)
	}
	if len(coord.saved) != 1 || coord.saved[0] != "lease-start:running" {
		t.Fatalf("saved snapshots = %+v", coord.saved)
	}
}

func TestInterviewService_StartReleasesLeaseAfterSuspension(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		suspendingInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-start-suspended",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(coord.released) != 1 || coord.released[0] != "lease-start-suspended:owner-a" {
		t.Fatalf("released = %+v", coord.released)
	}
}

func TestInterviewService_StartDegradesWhenSnapshotSaveFails(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true, saveErr: errors.New("redis down")}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "snapshot-start-degraded",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if err != nil {
		t.Fatalf("start should not fail on snapshot error: %v", err)
	}
	if sess.WorkingMemory.DegradedReasons["redis_snapshot"] == "" {
		t.Fatalf("expected redis_snapshot degraded reason, got %+v", sess.WorkingMemory.DegradedReasons)
	}
	stored, err := svc.Get(context.Background(), "snapshot-start-degraded")
	if err != nil {
		t.Fatalf("session should still be saved to primary store: %v", err)
	}
	if stored.WorkingMemory.DegradedReasons["redis_snapshot"] == "" {
		t.Fatalf("stored session should include degraded reason, got %+v", stored.WorkingMemory.DegradedReasons)
	}
}

func TestInterviewService_AnswerRenewsLeaseAndSavesSnapshot(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-answer",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "lease-answer",
		UserID:    "u1",
		Answer:    "answer",
	}); err != nil {
		t.Fatalf("answer: %v", err)
	}

	if len(coord.renewed) != 1 || coord.renewed[0] != "lease-answer:owner-a" {
		t.Fatalf("renewed = %+v", coord.renewed)
	}
	if len(coord.saved) != 2 || coord.saved[1] != "lease-answer:completed" {
		t.Fatalf("saved snapshots = %+v", coord.saved)
	}
	if len(coord.released) != 1 || coord.released[0] != "lease-answer:owner-a" {
		t.Fatalf("released = %+v", coord.released)
	}
}

func TestInterviewService_AnswerReleasesLeaseAfterSuspension(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		suspendingInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-answer-suspended",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "lease-answer-suspended",
		UserID:    "u1",
		Answer:    "answer",
	}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(coord.released) != 2 {
		t.Fatalf("released = %+v, want start and answer release", coord.released)
	}
}

func TestInterviewService_AnswerDegradesWhenSnapshotSaveFails(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)
	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "snapshot-answer-degraded",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	coord.saveErr = errors.New("redis down")
	sess, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "snapshot-answer-degraded",
		UserID:    "u1",
		Answer:    "answer",
	})
	if err != nil {
		t.Fatalf("answer should not fail on snapshot error: %v", err)
	}
	if sess.WorkingMemory.DegradedReasons["redis_snapshot"] == "" {
		t.Fatalf("expected redis_snapshot degraded reason, got %+v", sess.WorkingMemory.DegradedReasons)
	}
	stored, err := svc.Get(context.Background(), "snapshot-answer-degraded")
	if err != nil {
		t.Fatalf("get stored session: %v", err)
	}
	if stored.WorkingMemory.DegradedReasons["redis_snapshot"] == "" {
		t.Fatalf("stored session should include degraded reason, got %+v", stored.WorkingMemory.DegradedReasons)
	}
}

func TestInterviewService_AnswerAcquiresExpiredLease(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: false}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-expired",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "lease-expired",
		UserID:    "u1",
		Answer:    "answer",
	}); err != nil {
		t.Fatalf("answer should acquire expired lease: %v", err)
	}

	if len(coord.renewed) != 1 || coord.renewed[0] != "lease-expired:owner-a" {
		t.Fatalf("renewed = %+v", coord.renewed)
	}
	if len(coord.acquired) != 2 || coord.acquired[1] != "lease-expired:owner-a" {
		t.Fatalf("acquired = %+v", coord.acquired)
	}
}

func TestInterviewService_AnswerLoadsRedisSnapshotWhenStoreMisses(t *testing.T) {
	snapshot := &domain.Session{
		ID:            "takeover-answer",
		UserID:        "u1",
		Status:        domain.StatusRunning,
		CurrentNode:   "pick_next",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:      "q1",
					Content: "讲一下 Go GMP",
					Tags:    []string{"go"},
				},
			},
		},
	}
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: false, snapshot: snapshot}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-b",
	)

	sess, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "takeover-answer",
		UserID:    "u1",
		Answer:    "answer",
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if sess.Status != domain.StatusCompleted {
		t.Fatalf("status = %q, want completed", sess.Status)
	}
	if len(coord.acquired) != 1 || coord.acquired[0] != "takeover-answer:owner-b" {
		t.Fatalf("acquired = %+v", coord.acquired)
	}

	got, err := svc.Get(context.Background(), "takeover-answer")
	if err != nil {
		t.Fatalf("get recovered session from local store: %v", err)
	}
	if got.Report == nil {
		t.Fatal("recovered session should be saved back to local store after answer")
	}
}

func TestInterviewService_GetLoadsRedisSnapshotWhenStoreMisses(t *testing.T) {
	snapshot := &domain.Session{
		ID:            "takeover-get",
		UserID:        "u1",
		Status:        domain.StatusRunning,
		CurrentNode:   "pick_next",
		WorkingMemory: domain.NewWorkingMemory(),
	}
	coord := &fakeSessionCoordinator{acquireOK: true, renewOK: true, snapshot: snapshot}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-b",
	)

	got, err := svc.Get(context.Background(), "takeover-get")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "takeover-get" {
		t.Fatalf("id = %q, want takeover-get", got.ID)
	}
	got, err = svc.store.Get(context.Background(), "takeover-get")
	if err != nil {
		t.Fatalf("snapshot should be saved back to local store: %v", err)
	}
	if got.UserID != "u1" {
		t.Fatalf("user id = %q, want u1", got.UserID)
	}
}

func TestInterviewService_StartReturnsLeaseConflict(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: false, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)

	_, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "lease-conflict",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if !errors.Is(err, ErrSessionLeaseConflict) {
		t.Fatalf("err = %v, want ErrSessionLeaseConflict", err)
	}
}

func TestInterviewStart_LeaseConflictReturnsHTTP409(t *testing.T) {
	coord := &fakeSessionCoordinator{acquireOK: false, renewOK: true}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)
	server := NewServerWithInterview(&config.Config{}, svc)

	body := bytes.NewBufferString(`{
		"session_id":"lease-http-conflict",
		"user_id":"u1",
		"jd_text":"需要 Go 后端",
		"resume_text":"两年 Go 经验"
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace-lease-start")

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if !strings.Contains(rec.Body.String(), `"retry_after_seconds":1`) {
		t.Fatalf("body should include retry_after_seconds=1, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"lease_conflict"`) {
		t.Fatalf("body should include lease_conflict code, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"trace_id":"trace-lease-start"`) {
		t.Fatalf("body should include trace id, got %s", rec.Body.String())
	}
}

func TestWriteInterviewError_StaleSessionWriteReturnsHTTP409(t *testing.T) {
	router := gin.New()
	router.Use(TraceIDMiddleware())
	router.GET("/stale", func(c *gin.Context) {
		writeInterviewError(c, fmt.Errorf("%w: s1", ErrStaleSessionWrite))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stale", nil)
	req.Header.Set("X-Trace-Id", "trace-stale")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"code":"stale_session_write"`, `"trace_id":"trace-stale"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestInterviewAnswer_LeaseConflictReturnsHTTP409(t *testing.T) {
	store := NewMemorySessionStore()
	if err := store.Save(context.Background(), &domain.Session{
		ID:            "lease-answer-http-conflict",
		UserID:        "u1",
		Status:        domain.StatusRunning,
		CurrentNode:   "pick_next",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:      "q1",
					Content: "讲一下 Go GMP",
					Tags:    []string{"go"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	coord := &fakeSessionCoordinator{acquireOK: false, renewOK: false}
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		store,
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)
	server := NewServerWithInterview(&config.Config{}, svc)

	body := bytes.NewBufferString(`{
		"session_id":"lease-answer-http-conflict",
		"user_id":"u1",
		"answer":"answer"
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/answer", body)
	req.Header.Set("Content-Type", "application/json")

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if !strings.Contains(rec.Body.String(), `"retry_after_seconds":1`) {
		t.Fatalf("body should include retry_after_seconds=1, got %s", rec.Body.String())
	}
}

func TestIntegration_InterviewService_RedisCoordinatorSnapshots(t *testing.T) {
	coord := openRedisSessionCoordinatorForIntegration(t)
	svc := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-service-it",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sessionID := "svc-coord-" + time.Now().Format("150405.000000000")
	t.Cleanup(func() {
		_ = coord.DeleteSnapshot(context.Background(), sessionID)
		_, _ = coord.ReleaseLease(context.Background(), sessionID, "owner-service-it")
	})

	if _, err := svc.Start(ctx, startInterviewRequest{
		SessionID:  sessionID,
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	startSnapshot, err := coord.LoadSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("load start snapshot: %v", err)
	}
	if startSnapshot.Status != domain.StatusRunning {
		t.Fatalf("start snapshot status = %q, want running", startSnapshot.Status)
	}

	if _, err := svc.Answer(ctx, answerInterviewRequest{
		SessionID: sessionID,
		UserID:    "u1",
		Answer:    "answer",
	}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	answerSnapshot, err := coord.LoadSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("load answer snapshot: %v", err)
	}
	if answerSnapshot.Status != domain.StatusCompleted {
		t.Fatalf("answer snapshot status = %q, want completed", answerSnapshot.Status)
	}
	if answerSnapshot.Report == nil {
		t.Fatal("answer snapshot should include report")
	}
}

func TestIntegration_InterviewService_TakeoverFromRedisSnapshot(t *testing.T) {
	coord := openRedisSessionCoordinatorForIntegration(t)
	sessionID := "svc-takeover-" + time.Now().Format("150405.000000000")
	t.Cleanup(func() {
		_ = coord.DeleteSnapshot(context.Background(), sessionID)
		_, _ = coord.ReleaseLease(context.Background(), sessionID, "owner-a")
		_, _ = coord.ReleaseLease(context.Background(), sessionID, "owner-b")
	})

	svcA := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-a",
	)
	svcA.leaseTTL = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := svcA.Start(ctx, startInterviewRequest{
		SessionID:  sessionID,
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start on owner-a: %v", err)
	}
	time.Sleep(80 * time.Millisecond)

	svcB := NewInterviewServiceWithStoreEventsAndCoordinator(
		fakeInterviewRunner{},
		NewMemorySessionStore(),
		NewMemoryInterviewEventHub(8),
		coord,
		"owner-b",
	)
	got, err := svcB.Answer(ctx, answerInterviewRequest{
		SessionID: sessionID,
		UserID:    "u1",
		Answer:    "answer",
	})
	if err != nil {
		t.Fatalf("answer on owner-b: %v", err)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if got.Report == nil {
		t.Fatal("takeover answer should generate report")
	}
}

func TestInterviewAnswer_ReturnsReport(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	startBody := bytes.NewBufferString(`{"session_id":"s2","jd_text":"jd","resume_text":"resume"}`)
	startReq := httptest.NewRequest(http.MethodPost, "/api/interview/start", startBody)
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), startReq)

	answerBody := bytes.NewBufferString(`{"session_id":"s2","answer":"G 是 goroutine"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/answer", answerBody)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got interviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Report == nil || got.Report.OverallScore != 80 {
		t.Fatalf("report = %+v, want score 80", got.Report)
	}
	if len(got.Rounds) != 1 || got.Rounds[0].Feedback == nil {
		t.Fatalf("completed exam should include round feedback: %+v", got.Rounds)
	}
	if got.Question != nil {
		t.Fatalf("question should be empty after completed report, got %+v", got.Question)
	}
}

func TestFillPendingAnswer_UsesSuspensionBeforeCurrentNode(t *testing.T) {
	sess := &domain.Session{
		ID:          "answer-suspension",
		CurrentNode: "pick_next",
		Suspension: &domain.Suspension{
			Node:     "probe_ask",
			Awaiting: domain.SuspensionAwaitingAnswer,
		},
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", Content: "Q"},
			FollowUps: []domain.FollowUp{{
				Question: "追问",
			}},
		}},
	}

	if err := fillPendingAnswer(sess, "follow-up answer"); err != nil {
		t.Fatalf("fill answer: %v", err)
	}
	if sess.Rounds[0].Answer != "" {
		t.Fatalf("main answer should not be written, got %q", sess.Rounds[0].Answer)
	}
	if got := sess.Rounds[0].FollowUps[0].Answer; got != "follow-up answer" {
		t.Fatalf("follow-up answer = %q", got)
	}
}

func TestFillPendingAnswer_RejectsNonAnswerSuspension(t *testing.T) {
	sess := &domain.Session{
		ID:          "answer-approval",
		CurrentNode: "pick_next",
		Suspension: &domain.Suspension{
			Node:     "pick_next",
			Awaiting: domain.SuspensionAwaitingApproval,
		},
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", Content: "Q"},
		}},
	}

	err := fillPendingAnswer(sess, "answer")
	if !errors.Is(err, ErrInvalidSessionState) {
		t.Fatalf("err = %v, want ErrInvalidSessionState", err)
	}
}

func TestInterviewService_AnswerPersistsLongTermMemory(t *testing.T) {
	memStore := memory.NewMemoryStore()
	svc := NewInterviewService(fakeInterviewRunner{})
	svc.SetMemoryStore(memStore)
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	startBody := bytes.NewBufferString(`{"session_id":"memory-s1","user_id":"u-memory","jd_text":"jd","resume_text":"resume"}`)
	startReq := httptest.NewRequest(http.MethodPost, "/api/interview/start", startBody)
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), startReq)

	answerBody := bytes.NewBufferString(`{"session_id":"memory-s1","user_id":"u-memory","answer":"G 是 goroutine"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/answer", answerBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, err := memStore.GetUserMemory(context.Background(), "u-memory")
	if err != nil {
		t.Fatalf("get user memory: %v", err)
	}
	if got.SkillScores["go"] != 80 {
		t.Fatalf("skill scores = %+v, want go=80", got.SkillScores)
	}
	if len(got.Strengths) == 0 || got.Strengths[0] != "Go 基础清楚" {
		t.Fatalf("strengths = %+v", got.Strengths)
	}
}

func TestInterviewService_AnswerSkipsLongTermMemoryWhenReportMissing(t *testing.T) {
	memStore := memory.NewMemoryStore()
	svc := NewInterviewService(missingReportRunner{})
	svc.SetMemoryStore(memStore)

	sess := &domain.Session{
		ID:          "memory-no-report",
		UserID:      "u-memory",
		Status:      domain.StatusRunning,
		CurrentNode: "pick_next",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", Content: "讲一下 Go", Tags: []string{"go"}},
		}},
	}
	if err := svc.store.Save(context.Background(), sess); err != nil {
		t.Fatalf("save session: %v", err)
	}

	got, err := svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "memory-no-report",
		UserID:    "u-memory",
		Answer:    "answer",
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if got.Status != domain.StatusCompleted || got.Report != nil {
		t.Fatalf("session = %+v, want completed without report", got)
	}
	if _, err := memStore.GetUserMemory(context.Background(), "u-memory"); !errors.Is(err, memory.ErrUserMemoryNotFound) {
		t.Fatalf("memory error = %v, want not found", err)
	}
}

func TestInterviewService_AnswerIgnoresLongTermMemoryStoreFailure(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	svc.SetMemoryStore(failingMemoryStore{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	startBody := bytes.NewBufferString(`{"session_id":"memory-fail","user_id":"u-memory","jd_text":"jd","resume_text":"resume"}`)
	startReq := httptest.NewRequest(http.MethodPost, "/api/interview/start", startBody)
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), startReq)

	answerBody := bytes.NewBufferString(`{"session_id":"memory-fail","user_id":"u-memory","answer":"G 是 goroutine"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/answer", answerBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got interviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != string(domain.StatusCompleted) || got.Report == nil {
		t.Fatalf("response = %+v, want completed report", got)
	}
}

func TestInterviewService_LongTermMemoryMergeIsSerialized(t *testing.T) {
	memStore := newDelayingMemoryStore()
	svc := NewInterviewService(fakeInterviewRunner{})
	svc.SetMemoryStore(memStore)
	sessA := &domain.Session{
		ID:     "memory-concurrent-a",
		UserID: "u-memory",
		Report: &domain.Report{
			Highlights:     []string{"Go 基础清楚"},
			SkillBreakdown: map[string]int{"go": 80},
		},
	}
	sessB := &domain.Session{
		ID:     "memory-concurrent-b",
		UserID: "u-memory",
		Report: &domain.Report{
			Highlights:     []string{"Redis 排障清楚"},
			SkillBreakdown: map[string]int{"redis": 70},
		},
	}

	errs := make(chan error, 2)
	go func() { errs <- svc.persistLongTermMemory(context.Background(), sessA) }()
	<-memStore.firstHit
	go func() { errs <- svc.persistLongTermMemory(context.Background(), sessB) }()

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("persist memory: %v", err)
		}
	}
	got, err := memStore.GetUserMemory(context.Background(), "u-memory")
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.SkillScores["go"] != 80 || got.SkillScores["redis"] != 70 {
		t.Fatalf("skill scores = %+v, want both updates", got.SkillScores)
	}
	if len(got.Strengths) != 2 {
		t.Fatalf("strengths = %+v, want both updates", got.Strengths)
	}
}

func TestInterviewResponse_PracticeShowsFeedbackBeforeCompletion(t *testing.T) {
	now := time.Now()
	sess := &domain.Session{
		ID:          "practice-feedback",
		Mode:        "practice",
		Status:      domain.StatusRunning,
		CurrentNode: "pick_next",
		CreatedAt:   now,
		UpdatedAt:   now,
		Rounds: []domain.AnswerRound{{
			RoundID: "r1",
			Question: domain.Question{
				ID:             "q1",
				Content:        "讲一下 channel",
				ExpectedPoints: []string{"hchan", "sendq"},
			},
			Answer: "channel 有缓冲和阻塞语义",
			Evaluation: &domain.Evaluation{
				QuestionID: "q1",
				Score:      75,
				Strengths:  []string{"说到阻塞语义"},
				Weaknesses: []string{"没讲 hchan"},
				Suggestion: "补充底层队列结构",
			},
		}},
	}

	got := buildInterviewResponse(sess)
	if len(got.Rounds) != 1 || got.Rounds[0].Feedback == nil {
		t.Fatalf("practice should expose feedback: %+v", got.Rounds)
	}
	if got.Rounds[0].Feedback.ExpectedPoints[0] != "hchan" {
		t.Fatalf("expected points missing: %+v", got.Rounds[0].Feedback)
	}
}

func TestInterviewResponse_ExamHidesFeedbackBeforeCompletion(t *testing.T) {
	now := time.Now()
	sess := &domain.Session{
		ID:          "exam-feedback",
		Mode:        "exam",
		Status:      domain.StatusRunning,
		CurrentNode: "pick_next",
		CreatedAt:   now,
		UpdatedAt:   now,
		Rounds: []domain.AnswerRound{{
			RoundID: "r1",
			Question: domain.Question{
				ID:             "q1",
				Content:        "讲一下 channel",
				ExpectedPoints: []string{"hchan"},
			},
			Answer: "channel 有阻塞语义",
			Evaluation: &domain.Evaluation{
				QuestionID: "q1",
				Score:      60,
				Strengths:  []string{"基础语义"},
				Weaknesses: []string{"缺底层结构"},
			},
		}},
	}

	got := buildInterviewResponse(sess)
	if len(got.Rounds) != 1 {
		t.Fatalf("rounds missing: %+v", got.Rounds)
	}
	if got.Rounds[0].Feedback != nil {
		t.Fatalf("exam should hide feedback before completion: %+v", got.Rounds[0].Feedback)
	}
	if len(got.Rounds[0].Question.ExpectedPoints) != 0 {
		t.Fatalf("exam should hide expected points before completion: %+v", got.Rounds[0].Question.ExpectedPoints)
	}
}

func TestInterviewResponse_IncludesRetrievalTraceCopy(t *testing.T) {
	now := time.Now()
	sess := &domain.Session{
		ID:        "trace-response",
		Mode:      "exam",
		Status:    domain.StatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
		RetrievalTrace: &domain.RetrievalTrace{
			Query:           "redis aof",
			FallbackReasons: []string{"rerank fallback"},
			Stages: []domain.RetrievalStageTrace{{
				Stage:      "rerank",
				Count:      1,
				DurationMS: 3.5,
				Items: []domain.RetrievalResultTrace{{
					ID:      "redis-001",
					Rank:    1,
					Score:   0.92,
					Stage:   "rerank",
					Reason:  "matched query",
					Sources: map[string]float64{"rrf": 0.8},
				}},
			}},
			Final: []domain.RetrievalResultTrace{{ID: "redis-001", Rank: 1, Score: 0.92}},
		},
	}

	got := buildInterviewResponse(sess)
	if got.RetrievalTrace == nil {
		t.Fatal("retrieval trace should be included")
	}
	if got.RetrievalTrace.Query != "redis aof" || got.RetrievalTrace.Stages[0].Items[0].ID != "redis-001" {
		t.Fatalf("retrieval trace = %+v", got.RetrievalTrace)
	}

	sess.RetrievalTrace.Stages[0].Items[0].Sources["rrf"] = 0.1
	if got.RetrievalTrace.Stages[0].Items[0].Sources["rrf"] != 0.8 {
		t.Fatalf("retrieval trace should be deep copied: %+v", got.RetrievalTrace.Stages[0].Items[0].Sources)
	}
}

func TestInterviewResponse_IncludesSuspensionCopy(t *testing.T) {
	now := time.Now()
	sess := &domain.Session{
		ID:        "suspension-response",
		Mode:      "exam",
		Status:    domain.StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
		Suspension: &domain.Suspension{
			Node:      "pick_next",
			Reason:    "waiting for answer",
			Awaiting:  domain.SuspensionAwaitingAnswer,
			Payload:   map[string]interface{}{"round_id": "r1"},
			CreatedAt: now,
		},
	}

	got := buildInterviewResponse(sess)
	if got.Suspension == nil {
		t.Fatal("suspension should be included")
	}
	if got.Suspension.Node != "pick_next" || got.Suspension.Awaiting != domain.SuspensionAwaitingAnswer {
		t.Fatalf("suspension = %+v", got.Suspension)
	}

	sess.Suspension.Payload["round_id"] = "changed"
	if got.Suspension.Payload["round_id"] != "r1" {
		t.Fatalf("suspension payload should be copied: %+v", got.Suspension.Payload)
	}
}

func TestInterviewAnswer_UserMismatch(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	startReq := httptest.NewRequest(http.MethodPost, "/api/interview/start",
		bytes.NewBufferString(`{"session_id":"s-answer-user-check","user_id":"u1","jd_text":"jd","resume_text":"resume"}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), startReq)

	answerBody := bytes.NewBufferString(`{"session_id":"s-answer-user-check","user_id":"u2","answer":"answer"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/answer", answerBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterviewService_MaintainsTimestamps(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "ts-1",
		UserID:     "u1",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set on start")
	}
	if sess.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set on start")
	}
	createdAt := sess.CreatedAt
	updatedAt := sess.UpdatedAt

	sess, err = svc.Answer(context.Background(), answerInterviewRequest{
		SessionID: "ts-1",
		Answer:    "answer",
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if !sess.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt changed: got %v want %v", sess.CreatedAt, createdAt)
	}
	if !sess.UpdatedAt.After(updatedAt) && sess.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt should advance on answer: before=%v after=%v", updatedAt, sess.UpdatedAt)
	}
}

func TestInterviewListSessions_ReturnsUserSessions(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	for _, body := range []string{
		`{"session_id":"s-list-1","user_id":"u-list","jd_text":"jd","resume_text":"resume"}`,
		`{"session_id":"s-list-2","user_id":"u-list","jd_text":"jd","resume_text":"resume"}`,
		`{"session_id":"s-list-other","user_id":"other","jd_text":"jd","resume_text":"resume"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/interview/start", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/interview/sessions?user_id=u-list&limit=10", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Sessions []interviewResponse `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got.Sessions), got.Sessions)
	}
	for _, sess := range got.Sessions {
		if sess.SessionID == "s-list-other" {
			t.Fatalf("should not include other user's session: %+v", got.Sessions)
		}
		if sess.UpdatedAt.IsZero() {
			t.Fatalf("list response should include UpdatedAt: %+v", sess)
		}
	}
	if len(got.Sessions) == 2 && got.Sessions[0].UpdatedAt.Before(got.Sessions[1].UpdatedAt) {
		t.Fatalf("sessions should be ordered by updated_at desc: %+v", got.Sessions)
	}
}

func TestInterviewListSessions_CapsLimit(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	for i := 0; i < 105; i++ {
		body := fmt.Sprintf(`{"session_id":"s-cap-%03d","user_id":"u-cap","jd_text":"jd","resume_text":"resume"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/api/interview/start", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/interview/sessions?user_id=u-cap&limit=1000", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Sessions []interviewResponse `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Sessions) != 100 {
		t.Fatalf("len = %d, want cap 100", len(got.Sessions))
	}
}

func TestInterviewGetSession_ReturnsSession(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	startReq := httptest.NewRequest(http.MethodPost, "/api/interview/start",
		bytes.NewBufferString(`{"session_id":"s-get","user_id":"u1","jd_text":"jd","resume_text":"resume"}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), startReq)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/interview/sessions/s-get", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got interviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SessionID != "s-get" {
		t.Fatalf("session_id = %q, want s-get", got.SessionID)
	}
	if got.Question == nil || got.Question.ID != "q1" {
		t.Fatalf("question = %+v, want q1", got.Question)
	}
}

func TestInterviewGetSession_FallsBackToUserListWhenPointReadFails(t *testing.T) {
	store := &flakyReadSessionStore{
		getErr: errors.New("select session: transient"),
		sessions: []*domain.Session{
			{
				ID:        "s-detail-fallback",
				UserID:    "u1",
				Status:    domain.StatusCompleted,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Report: &domain.Report{
					SessionID:    "s-detail-fallback",
					OverallScore: 80,
				},
			},
		},
	}
	svc := NewInterviewServiceWithStore(fakeInterviewRunner{}, store)
	server := NewServerWithInterview(&config.Config{}, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/interview/sessions/s-detail-fallback?user_id=u1", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got interviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SessionID != "s-detail-fallback" || got.Report == nil {
		t.Fatalf("unexpected session response: %+v", got)
	}
}

func TestInterviewGetSession_NotFound(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/interview/sessions/missing", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterviewGetSession_UserMismatch(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	startReq := httptest.NewRequest(http.MethodPost, "/api/interview/start",
		bytes.NewBufferString(`{"session_id":"s-user-check","user_id":"u1","jd_text":"jd","resume_text":"resume"}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), startReq)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/interview/sessions/s-user-check?user_id=u2", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterviewDeleteSession_RemovesOnlySameUserSession(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	server := NewServerWithInterview(&config.Config{}, svc)
	router := server.Router()

	for _, body := range []string{
		`{"session_id":"s-delete","user_id":"u1","jd_text":"jd","resume_text":"resume"}`,
		`{"session_id":"s-keep","user_id":"u2","jd_text":"jd","resume_text":"resume"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/interview/start", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	wrongUser := httptest.NewRecorder()
	wrongReq := httptest.NewRequest(http.MethodDelete, "/api/interview/sessions/s-delete?user_id=u2", nil)
	router.ServeHTTP(wrongUser, wrongReq)
	if wrongUser.Code != http.StatusBadRequest {
		t.Fatalf("wrong user delete status = %d, want 400, body=%s", wrongUser.Code, wrongUser.Body.String())
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/interview/sessions/s-delete?user_id=u1", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/interview/sessions?user_id=u1", nil)
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var got struct {
		Sessions []interviewResponse `json:"sessions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(got.Sessions) != 0 {
		t.Fatalf("u1 sessions len = %d, want 0", len(got.Sessions))
	}
}

func TestMemoryInterviewEventHub_PublishSubscribe(t *testing.T) {
	hub := NewMemoryInterviewEventHub(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe, err := hub.Subscribe(ctx, "sess-evt", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()

	hub.Publish(context.Background(), InterviewEvent{
		Type:      interviewEventSessionUpdated,
		SessionID: "sess-evt",
	})

	select {
	case got := <-ch:
		if got.SessionID != "sess-evt" {
			t.Fatalf("session_id = %q, want sess-evt", got.SessionID)
		}
		if got.Type != interviewEventSessionUpdated {
			t.Fatalf("type = %q, want %q", got.Type, interviewEventSessionUpdated)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestMemoryInterviewEventHub_ReplayBeforeLiveEvents(t *testing.T) {
	hub := NewMemoryInterviewEventHub(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := hub.Publish(context.Background(), InterviewEvent{
		Type:      interviewEventSessionUpdated,
		SessionID: "sess-replay-order",
	})
	replayed := hub.Publish(context.Background(), InterviewEvent{
		Type:      interviewEventSessionCompleted,
		SessionID: "sess-replay-order",
	})

	ch, unsubscribe, err := hub.Subscribe(ctx, "sess-replay-order", first.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()

	live := hub.Publish(context.Background(), InterviewEvent{
		Type:      interviewEventSessionFailed,
		SessionID: "sess-replay-order",
	})

	for _, want := range []InterviewEvent{replayed, live} {
		select {
		case got := <-ch:
			if got.ID != want.ID {
				t.Fatalf("event id = %q, want %q", got.ID, want.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", want.ID)
		}
	}
}

func TestMemoryInterviewEventHub_CloseStopsSubscribers(t *testing.T) {
	hub := NewMemoryInterviewEventHub(4)
	ch, unsubscribe, err := hub.Subscribe(context.Background(), "sess-close", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()

	if err := hub.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("subscriber channel should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed channel")
	}

	_, _, err = hub.Subscribe(context.Background(), "sess-close", "")
	if err == nil {
		t.Fatal("subscribe after close should fail")
	}
}

func TestMemoryInterviewEventHub_CloseWhilePublishingDoesNotPanic(t *testing.T) {
	hub := NewMemoryInterviewEventHub(16)
	for i := 0; i < 8; i++ {
		ch, unsubscribe, err := hub.Subscribe(context.Background(), "sess-race", "")
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		defer unsubscribe()
		go func() {
			for range ch {
			}
		}()
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			hub.Publish(context.Background(), InterviewEvent{
				Type:      interviewEventSessionUpdated,
				SessionID: "sess-race",
			})
		}
	}()
	if err := hub.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	wg.Wait()
}

func TestMemoryInterviewEventHub_RecordsDroppedEvents(t *testing.T) {
	hub := NewMemoryInterviewEventHub(1)
	ch, unsubscribe, err := hub.Subscribe(context.Background(), "sess-drop", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()

	hub.Publish(context.Background(), InterviewEvent{Type: interviewEventSessionUpdated, SessionID: "sess-drop"})
	hub.Publish(context.Background(), InterviewEvent{Type: interviewEventSessionCompleted, SessionID: "sess-drop"})

	if stats := hub.Stats(); stats.DroppedEvents != 1 {
		t.Fatalf("dropped events = %d, want 1", stats.DroppedEvents)
	}
	<-ch
}

func TestBuildInterviewEvent_ClonesMutableSessionData(t *testing.T) {
	sess := &domain.Session{
		ID:          "sess-clone",
		Status:      domain.StatusCompleted,
		CurrentNode: "report",
		Suspension: &domain.Suspension{
			Node:     "pick_next",
			Awaiting: domain.SuspensionAwaitingAnswer,
			Payload:  map[string]any{"question_id": "q1"},
		},
		Report: &domain.Report{
			SessionID:      "sess-clone",
			OverallScore:   80,
			SkillBreakdown: map[string]int{"go": 80},
			Highlights:     []string{"clear"},
			Improvements:   []string{"more detail"},
			NextSteps:      []string{"practice"},
		},
		Rounds: []domain.AnswerRound{
			{
				Question: domain.Question{
					ID:             "q-clone",
					Content:        "question",
					Tags:           []string{"go"},
					ExpectedPoints: []string{"scheduler"},
				},
			},
		},
	}

	ctx := traceid.Inject(context.Background(), "trace-clone")
	event := buildInterviewEventWithContext(ctx, interviewEventSessionCompleted, sess, "", "")
	sess.Report.SkillBreakdown["go"] = 10
	sess.Report.Highlights[0] = "mutated"
	sess.Rounds[0].Question.Tags[0] = "mutated"
	sess.Rounds[0].Question.ExpectedPoints[0] = "mutated"
	sess.Suspension.Payload["question_id"] = "mutated"

	if event.TraceID != "trace-clone" {
		t.Fatalf("trace id = %q", event.TraceID)
	}
	if event.Report.SkillBreakdown["go"] != 80 {
		t.Fatalf("report score was mutated: %+v", event.Report.SkillBreakdown)
	}
	if event.Report.Highlights[0] != "clear" {
		t.Fatalf("report highlights were mutated: %+v", event.Report.Highlights)
	}
	if event.Question != nil && event.Question.Tags[0] == "mutated" {
		t.Fatalf("question tags were mutated: %+v", event.Question.Tags)
	}
	if event.Question != nil && event.Question.ExpectedPoints[0] == "mutated" {
		t.Fatalf("question expected points were mutated: %+v", event.Question.ExpectedPoints)
	}
	if event.Suspension == nil || event.Suspension.Payload["question_id"] != "q1" {
		t.Fatalf("suspension was mutated: %+v", event.Suspension)
	}
}

func TestInterviewStream_ReturnsSnapshotAndLiveEvent(t *testing.T) {
	hub := NewMemoryInterviewEventHub(8)
	svc := NewInterviewServiceWithStoreAndEvents(fakeInterviewRunner{}, NewMemorySessionStore(), hub)
	server := NewServerWithInterview(&config.Config{}, svc)

	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "s-stream",
		UserID:     "u-stream",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/interview/stream?session_id=s-stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	first := readSSEBlock(t, reader)
	if !strings.Contains(first, "event: snapshot") {
		t.Fatalf("first event = %q, want snapshot", first)
	}
	if !strings.Contains(first, `"session_id":"s-stream"`) {
		t.Fatalf("first event missing session id: %q", first)
	}

	hub.Publish(context.Background(), buildInterviewEvent(interviewEventSessionUpdated, sess, "", ""))
	second := readSSEBlock(t, reader)
	if !strings.Contains(second, "event: interview.progress") {
		t.Fatalf("second event = %q, want interview.progress", second)
	}
}

func TestWriteInterviewSSE_UsesBusinessEventShape(t *testing.T) {
	rec := httptest.NewRecorder()
	event := InterviewEvent{
		ID:          "evt-safe",
		Type:        interviewEventNodeStart,
		SessionID:   "s-safe",
		Status:      string(domain.StatusRunning),
		CurrentNode: "pick_next",
		Node:        "pick_next",
		Phase:       "answering",
		At:          time.Now(),
	}

	if err := writeInterviewSSE(rec, event); err != nil {
		t.Fatalf("write sse: %v", err)
	}
	raw := rec.Body.String()
	if !strings.Contains(raw, "event: interview.progress") {
		t.Fatalf("sse should use business event name: %s", raw)
	}
	for _, forbidden := range []string{"current_node", "pick_next", "graph.node"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("sse leaked %q: %s", forbidden, raw)
		}
	}
}

func TestWriteInterviewSSE_IncludesSuspensionTraceAndReplayGap(t *testing.T) {
	rec := httptest.NewRecorder()
	event := InterviewEvent{
		ID:        "evt-runtime",
		Type:      interviewEventSnapshot,
		SessionID: "s-runtime",
		Status:    string(domain.StatusRunning),
		Phase:     "answering",
		Suspension: &domain.Suspension{
			Awaiting: domain.SuspensionAwaitingAnswer,
		},
		TraceID:   "trace-runtime",
		ReplayGap: true,
		At:        time.Now(),
	}

	if err := writeInterviewSSE(rec, event); err != nil {
		t.Fatalf("write sse: %v", err)
	}
	raw := rec.Body.String()
	for _, want := range []string{`"suspension"`, `"trace_id":"trace-runtime"`, `"replay_gap":true`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("sse missing %s: %s", want, raw)
		}
	}
}

func TestInterviewStream_BackpressureRejectsSecondLongConnection(t *testing.T) {
	hub := NewMemoryInterviewEventHub(8)
	svc := NewInterviewServiceWithStoreAndEvents(fakeInterviewRunner{}, NewMemorySessionStore(), hub)
	server := NewServerWithInterview(&config.Config{
		Server: config.ServerConfig{MaxStreams: 1},
	}, svc)

	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "s-stream-limit",
		UserID:     "u-stream",
		JDText:     "jd",
		ResumeText: "resume",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	firstReq, err := http.NewRequestWithContext(firstCtx, http.MethodGet, ts.URL+"/api/interview/stream?session_id=s-stream-limit", nil)
	if err != nil {
		t.Fatalf("new first request: %v", err)
	}
	firstResp, err := http.DefaultClient.Do(firstReq)
	if err != nil {
		t.Fatalf("first stream request: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d, want 200", firstResp.StatusCode)
	}
	firstBlock := readSSEBlock(t, bufio.NewReader(firstResp.Body))
	if !strings.Contains(firstBlock, "event: snapshot") {
		t.Fatalf("first block = %q, want snapshot", firstBlock)
	}

	secondResp, err := http.Get(ts.URL + "/api/interview/stream?session_id=s-stream-limit")
	if err != nil {
		t.Fatalf("second stream request: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d, want 503", secondResp.StatusCode)
	}
	if got := secondResp.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestInterviewStream_ReplaysAfterLastEventID(t *testing.T) {
	hub := NewMemoryInterviewEventHub(8)
	svc := NewInterviewServiceWithStoreAndEvents(fakeInterviewRunner{}, NewMemorySessionStore(), hub)
	server := NewServerWithInterview(&config.Config{}, svc)

	sess, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "s-stream-replay",
		UserID:     "u-stream",
		JDText:     "jd",
		ResumeText: "resume",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	first := hub.Publish(context.Background(), buildInterviewEvent(interviewEventSessionUpdated, sess, "", ""))
	lastID := first.ID
	second := hub.Publish(context.Background(), buildInterviewEvent(interviewEventSessionCompleted, sess, "", ""))

	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/interview/stream?session_id=s-stream-replay&last_event_id="+lastID, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	firstBlock := readSSEBlock(t, reader)
	if !strings.Contains(firstBlock, "event: snapshot") {
		t.Fatalf("first block = %q, want snapshot", firstBlock)
	}
	secondBlock := readSSEBlock(t, reader)
	if !strings.Contains(secondBlock, "event: interview.completed") {
		t.Fatalf("second block = %q, want interview.completed replay", secondBlock)
	}
	if !strings.Contains(secondBlock, second.ID) {
		t.Fatalf("second block = %q, want replay id %q", secondBlock, second.ID)
	}
}

func TestWriteInterviewSSEComment(t *testing.T) {
	rec := httptest.NewRecorder()

	if err := writeInterviewSSEComment(rec, "ping"); err != nil {
		t.Fatalf("write comment: %v", err)
	}

	if got := rec.Body.String(); got != ": ping\n\n" {
		t.Fatalf("comment = %q, want ping comment", got)
	}
}

func readSSEBlock(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var lines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse block: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
