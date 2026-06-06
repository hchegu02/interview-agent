package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

// buildPickSession 构造 pick_next 单测用的最小 session.
func buildPickSession(rounds, maxRounds int, pool []domain.Question) *domain.Session {
	mem := domain.NewWorkingMemory()
	mem.MaxRounds = maxRounds
	mem.RoundsAsked = rounds
	mem.SkillCoverage = map[string]float64{}

	return &domain.Session{
		JobProfile: &domain.JobProfile{
			Title:         "Go 后端",
			YearsRequired: 3,
			KeySkills:     []string{"go", "redis"},
		},
		CandProfile: &domain.CandidateProfile{Years: 3, Skills: []string{"go"}},
		GapReport: &domain.GapReport{
			Strategy: domain.GapStrategyExplore,
			Reason:   "中匹配",
		},
		CandidatePool: pool,
		WorkingMemory: mem,
	}
}

func samplePool() []domain.Question {
	return []domain.Question{
		{ID: "go-001", Content: "GMP 调度", Tags: []string{"go_concurrency"}, Difficulty: 3, SkillCategory: "go"},
		{ID: "redis-001", Content: "AOF vs RDB", Tags: []string{"redis_persistence"}, Difficulty: 3, SkillCategory: "redis"},
		{ID: "go-002", Content: "channel 底层", Tags: []string{"channel"}, Difficulty: 3, SkillCategory: "go"},
	}
}

// 终止: RemainingRounds == 0 -> Action=end, 不 suspend
func TestPickNext_EndsWhenMaxRoundsReached(t *testing.T) {
	sess := buildPickSession(8, 8, samplePool())
	node := NewPickNextNode(&stubChatModel{}, PickNextOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("expected nil (end), got %v", err)
	}
	if sess.PendingDecision == nil || sess.PendingDecision.Action != domain.ActionEnd {
		t.Errorf("expected Action=end, got %+v", sess.PendingDecision)
	}
	if len(sess.Rounds) != 0 {
		t.Errorf("should not create round on end, got %d", len(sess.Rounds))
	}
}

// 终止: 候选池过滤后为空 -> Action=end
func TestPickNext_EndsWhenPoolExhausted(t *testing.T) {
	pool := samplePool()
	sess := buildPickSession(2, 8, pool)
	// 把 3 道题都标成"已问过"
	for _, q := range pool {
		sess.Rounds = append(sess.Rounds, domain.AnswerRound{
			RoundID: "r-" + q.ID, Question: q,
		})
	}
	node := NewPickNextNode(&stubChatModel{}, PickNextOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("got %v", err)
	}
	if sess.PendingDecision.Action != domain.ActionEnd {
		t.Errorf("expected end, got %s", sess.PendingDecision.Action)
	}
}

// 正常出题: LLM 成功 -> suspend
func TestPickNext_LLMSuccess_Suspends(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"next_question_id":"redis-001","reasoning":"go 已 cover,补 redis"}`,
	}}
	sess := buildPickSession(2, 8, samplePool())
	node := NewPickNextNode(stub, PickNextOptions{})

	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	if sess.PendingDecision == nil || sess.PendingDecision.Action != domain.ActionAskNew {
		t.Errorf("expected ask_new, got %+v", sess.PendingDecision)
	}
	if sess.PendingDecision.NextQuestionID != "redis-001" {
		t.Errorf("picked id=%q, want redis-001", sess.PendingDecision.NextQuestionID)
	}
	if len(sess.Rounds) != 1 || sess.Rounds[0].Question.ID != "redis-001" {
		t.Errorf("rounds = %+v", sess.Rounds)
	}
	if sess.Rounds[0].Answer != "" {
		t.Errorf("answer should be empty, got %q", sess.Rounds[0].Answer)
	}
	if sess.WorkingMemory.RoundsAsked != 3 {
		t.Errorf("RoundsAsked = %d, want 3", sess.WorkingMemory.RoundsAsked)
	}
}

func TestPickNextPrompt_IncludesDynamicDifficultyGuidance(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"next_question_id":"redis-001","reasoning":"动态难度匹配"}`,
	}}
	sess := buildPickSession(2, 8, samplePool())
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{Current: domain.DifficultyHard}
	node := NewPickNextNode(stub, PickNextOptions{})

	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	if len(stub.lastConv) == 0 {
		t.Fatal("expected prompt to be sent to llm")
	}
	prompt := stub.lastConv[0].Content
	for _, want := range []string{"当前动态难度:hard", "目标题目难度:4", "优先选择接近目标题目难度"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestPickNextPatchNode_LLMSuccessReturnsSuspendPatch(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"next_question_id":"redis-001","reasoning":"go 已 cover,补 redis"}`,
	}}
	sess := buildPickSession(2, 8, samplePool())
	node := NewPickNextPatchNode(stub, PickNextOptions{})

	patch, err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) || !graph.IsPatchSuspend(err) {
		t.Fatalf("expected patch suspend, got %v", err)
	}
	if sess.PendingDecision != nil || len(sess.Rounds) != 0 {
		t.Fatalf("patch node should not apply directly: decision=%+v rounds=%+v", sess.PendingDecision, sess.Rounds)
	}
	if patch.PendingDecision == nil || patch.PendingDecision.NextQuestionID != "redis-001" {
		t.Fatalf("patch decision = %+v", patch.PendingDecision)
	}
	if patch.AppendRound == nil || patch.AppendRound.Question.ID != "redis-001" {
		t.Fatalf("patch round = %+v", patch.AppendRound)
	}
	if patch.WorkingMemory == nil || patch.WorkingMemory.RoundsAsked != 3 {
		t.Fatalf("patch working memory = %+v, want RoundsAsked=3", patch.WorkingMemory)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if sess.PendingDecision.NextQuestionID != "redis-001" || len(sess.Rounds) != 1 {
		t.Fatalf("session after apply: decision=%+v rounds=%+v", sess.PendingDecision, sess.Rounds)
	}
}

// LLM 编造 id: 被 schema 自纠正拦下, 自纠正后给合法 id
func TestPickNext_RejectsHallucinatedID(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"next_question_id":"made-up","reasoning":"x"}`,    // 不在池里
		`{"next_question_id":"go-001","reasoning":"重试给合法"}`, // 自纠正
	}}
	sess := buildPickSession(2, 8, samplePool())
	node := NewPickNextNode(stub, PickNextOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	if sess.PendingDecision.NextQuestionID != "go-001" {
		t.Errorf("expected go-001 after self-correct, got %s", sess.PendingDecision.NextQuestionID)
	}
	if stub.idx != 2 {
		t.Errorf("expected 2 LLM calls (orig + correct), got %d", stub.idx)
	}
}

// LLM 失败 -> 规则降级, 还能 suspend
func TestPickNext_LLMFails_DegradesToRule(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("boom")}, responses: []string{""}}
	pool := samplePool()
	sess := buildPickSession(2, 8, pool)
	// 让 redis 的 coverage 比 go 高,规则应该选 go (coverage 低的)
	sess.WorkingMemory.SkillCoverage = map[string]float64{"redis": 5, "go": 1}

	node := NewPickNextNode(stub, PickNextOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended even on fallback, got %v", err)
	}
	picked := sess.Rounds[0].Question
	if picked.SkillCategory != "go" {
		t.Errorf("rule should pick lowest coverage cat (go), got %s", picked.SkillCategory)
	}
	if sess.WorkingMemory.DegradedReasons["pick"] == "" {
		t.Errorf("expected pick degraded reason, got %v", sess.WorkingMemory.DegradedReasons)
	}
	if !strings.Contains(sess.PendingDecision.Reasoning, "降级") {
		t.Errorf("reasoning should mention 降级: %s", sess.PendingDecision.Reasoning)
	}
}

func TestPickByRule_PrefersQuestionNearCurrentDifficulty(t *testing.T) {
	mem := domain.NewWorkingMemory()
	mem.Difficulty = &domain.DifficultyState{Current: domain.DifficultyHard}
	mem.SkillCoverage = map[string]float64{"go": 0, "redis": 2}
	pool := []domain.Question{
		{ID: "easy-low-coverage", Content: "Go 基础", Difficulty: 1, SkillCategory: "go"},
		{ID: "hard-higher-coverage", Content: "Redis 高可用", Difficulty: 4, SkillCategory: "redis"},
	}

	picked, reasoning := pickByRule(pool, mem)
	if picked.ID != "hard-higher-coverage" {
		t.Fatalf("picked %s, want difficulty-near hard question; reasoning=%s", picked.ID, reasoning)
	}
	if !strings.Contains(reasoning, "难度") {
		t.Fatalf("reasoning should mention difficulty, got %q", reasoning)
	}
}

func TestPickByRule_DoesNotPreferUnknownDifficulty(t *testing.T) {
	mem := domain.NewWorkingMemory()
	mem.Difficulty = &domain.DifficultyState{Current: domain.DifficultyMedium}
	mem.SkillCoverage = map[string]float64{"go": 0, "redis": 2}
	pool := []domain.Question{
		{ID: "unknown-low-coverage", Content: "Go 综合题", Difficulty: 0, SkillCategory: "go"},
		{ID: "medium-higher-coverage", Content: "Redis 持久化", Difficulty: 3, SkillCategory: "redis"},
	}

	picked, reasoning := pickByRule(pool, mem)
	if picked.ID != "medium-higher-coverage" {
		t.Fatalf("picked %s, want known medium difficulty question; reasoning=%s", picked.ID, reasoning)
	}
}

// nil LLM: 走规则降级, 不报错
func TestPickNext_NilLLM_UsesRule(t *testing.T) {
	sess := buildPickSession(0, 8, samplePool())
	node := NewPickNextNode(nil, PickNextOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	if sess.PendingDecision.NextQuestionID == "" {
		t.Error("should still pick a question")
	}
	if sess.WorkingMemory.DegradedReasons["pick"] == "" {
		t.Error("nil LLM should mark degraded")
	}
}

// 已问过的题不再被选
func TestPickNext_FiltersAlreadyAsked(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"next_question_id":"go-002","reasoning":"换一道 go 题"}`,
	}}
	pool := samplePool()
	sess := buildPickSession(1, 8, pool)
	sess.Rounds = []domain.AnswerRound{
		{RoundID: "r1", Question: pool[0]}, // go-001 已问
	}

	node := NewPickNextNode(stub, PickNextOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("got %v", err)
	}
	if sess.PendingDecision.NextQuestionID == "go-001" {
		t.Error("should not re-pick already-asked go-001")
	}
	if len(sess.Rounds) != 2 {
		t.Errorf("rounds = %d, want 2", len(sess.Rounds))
	}
}

func TestPickNext_ReflectTopicNarrowsPoolAndClears(t *testing.T) {
	pool := samplePool()
	sess := buildPickSession(1, 8, pool)
	sess.WorkingMemory.ReflectTopic = "redis"
	sess.WorkingMemory.SkillCoverage = map[string]float64{"go": 0, "redis": 10}
	node := NewPickNextNode(nil, PickNextOptions{})

	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	if got := sess.Rounds[0].Question.SkillCategory; got != "redis" {
		t.Errorf("reflect topic should force redis pool, got %s", got)
	}
	if sess.WorkingMemory.ReflectTopic != "" {
		t.Errorf("ReflectTopic should be consumed, got %q", sess.WorkingMemory.ReflectTopic)
	}
}

func TestPickNext_ConsumesLegacyReflectTopicNote(t *testing.T) {
	sess := buildPickSession(1, 8, samplePool())
	sess.WorkingMemory.Notes = map[string]string{"reflect_topic": "redis"}
	node := NewPickNextNode(nil, PickNextOptions{})

	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	if got := sess.Rounds[0].Question.SkillCategory; got != "redis" {
		t.Errorf("legacy reflect_topic note should still work, got %s", got)
	}
	if _, ok := sess.WorkingMemory.Notes["reflect_topic"]; ok {
		t.Error("legacy reflect_topic note should be consumed")
	}
}

func TestPickNext_NilJobProfile_Permanent(t *testing.T) {
	node := NewPickNextNode(&stubChatModel{}, PickNextOptions{})
	sess := &domain.Session{}
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}
