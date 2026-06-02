package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/retriever"
)

// stubChatModel 是单测用的极简 ChatModel。
// Generate 按调用序列返回预设响应；Stream 不实现（setup 节点不用流）。
//
// 设计意图：节点单测的关注点是"输入 → schema 校验 → Session 写入"，
// 不该依赖 fixture 文件存在或 fingerprint 计算逻辑。
type stubChatModel struct {
	responses []string
	errs      []error
	idx       int
	lastConv  []llm.Message
}

func (s *stubChatModel) Name() string { return "stub" }

func (s *stubChatModel) Generate(ctx context.Context, messages []llm.Message, _ llm.Options) (*llm.Response, error) {
	s.lastConv = messages
	if s.idx >= len(s.responses) {
		return nil, errors.New("stub: no more responses")
	}
	i := s.idx
	s.idx++
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	return &llm.Response{Content: s.responses[i], Model: "stub"}, nil
}

func (s *stubChatModel) Stream(ctx context.Context, messages []llm.Message, _ llm.Options) (<-chan llm.Chunk, error) {
	return nil, errors.New("stub: stream not supported")
}

// -----------------------------------------------------------------------------
// parse_jd
// -----------------------------------------------------------------------------

func TestParseJDNode_Success(t *testing.T) {
	stub := &stubChatModel{responses: []string{`{
		"title": "Go 后端开发工程师",
		"level": "senior",
		"key_skills": ["Golang", "redis", "mysql"],
		"must_have": ["Golang"],
		"nice_to_have": ["kafka"],
		"years_required": 3
	}`}}
	node := NewParseJDNode(stub)

	sess := &domain.Session{
		JobProfile: &domain.JobProfile{JDRawText: "我们要找一个 3 年以上 Go 后端..."},
	}
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}

	if sess.JobProfile.Title == "" || sess.JobProfile.Level != "senior" {
		t.Errorf("profile not filled: %+v", sess.JobProfile)
	}
	// CanonicalizeTags 应该把 "Golang" → "go"
	if !contains(sess.JobProfile.KeySkills, "go") {
		t.Errorf("expected canonicalized 'go' in key_skills, got %v", sess.JobProfile.KeySkills)
	}
	if !contains(sess.JobProfile.MustHave, "go") {
		t.Errorf("expected canonicalized 'go' in must_have, got %v", sess.JobProfile.MustHave)
	}
	if sess.JobProfile.YearsRequired != 3 {
		t.Errorf("years_required = %d, want 3", sess.JobProfile.YearsRequired)
	}
}

func TestParseJDNode_EmptyJD(t *testing.T) {
	node := NewParseJDNode(&stubChatModel{})
	sess := &domain.Session{JobProfile: &domain.JobProfile{JDRawText: ""}}
	err := node(context.Background(), sess)
	if err == nil {
		t.Fatal("expected error for empty JD")
	}
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestParseJDNode_SchemaSelfCorrect(t *testing.T) {
	// 第一次返回缺字段，第二次（自纠正）返回完整 JSON
	stub := &stubChatModel{responses: []string{
		`{"title":"Go 工程师","level":"junior"}`, // 缺多个必填字段
		`{
			"title":"Go 工程师","level":"junior",
			"key_skills":["go"],"must_have":["go"],"nice_to_have":[],"years_required":1
		}`,
	}}
	node := NewParseJDNode(stub)
	sess := &domain.Session{JobProfile: &domain.JobProfile{JDRawText: "..."}}
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("expected self-correct success, got %v", err)
	}
	if stub.idx != 2 {
		t.Errorf("expected 2 LLM calls (orig + self-correct), got %d", stub.idx)
	}
}

func TestParseJDNode_InvalidLevelEnum(t *testing.T) {
	// 两次都给非法 level，应该返回 ErrSchemaInvalid
	bad := `{"title":"t","level":"高级","key_skills":["go"],"must_have":[],"nice_to_have":[],"years_required":0}`
	stub := &stubChatModel{responses: []string{bad, bad}}
	node := NewParseJDNode(stub)
	sess := &domain.Session{JobProfile: &domain.JobProfile{JDRawText: "..."}}
	err := node(context.Background(), sess)
	if err == nil {
		t.Fatal("expected schema error")
	}
	if !strings.Contains(err.Error(), "level") {
		t.Errorf("error should mention 'level', got: %v", err)
	}
}

// -----------------------------------------------------------------------------
// parse_resume
// -----------------------------------------------------------------------------

func TestParseResumeNode_Success(t *testing.T) {
	stub := &stubChatModel{responses: []string{`{
		"years": 4,
		"skills": ["Golang", "Redis", "kafka"],
		"projects": [{
			"name": "秒杀系统",
			"role": "后端主力",
			"highlights": ["用 Redis lua 实现库存预扣支撑 1w QPS"],
			"stack": ["go", "redis"]
		}],
		"highlights": ["主导秒杀系统设计支撑 1w QPS"]
	}`}}
	node := NewParseResumeNode(stub)
	sess := &domain.Session{CandProfile: &domain.CandidateProfile{ResumeRawText: "..."}}
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if sess.CandProfile.Years != 4 {
		t.Errorf("years = %d, want 4", sess.CandProfile.Years)
	}
	if !contains(sess.CandProfile.Skills, "go") {
		t.Errorf("skills should contain canonical 'go', got %v", sess.CandProfile.Skills)
	}
	if len(sess.CandProfile.Projects) != 1 || sess.CandProfile.Projects[0].Name == "" {
		t.Errorf("projects not parsed: %+v", sess.CandProfile.Projects)
	}
	if len(sess.CandProfile.Highlights) == 0 {
		t.Errorf("highlights empty")
	}
}

func TestParseResumeNode_EmptyResume(t *testing.T) {
	node := NewParseResumeNode(&stubChatModel{})
	sess := &domain.Session{CandProfile: &domain.CandidateProfile{ResumeRawText: ""}}
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestParseResumeNode_EmptySkillsRejected(t *testing.T) {
	bad := `{"years":2,"skills":[],"projects":[],"highlights":[]}`
	stub := &stubChatModel{responses: []string{bad, bad}}
	node := NewParseResumeNode(stub)
	sess := &domain.Session{CandProfile: &domain.CandidateProfile{ResumeRawText: "..."}}
	if err := node(context.Background(), sess); err == nil {
		t.Fatal("expected error for empty skills")
	}
}

// -----------------------------------------------------------------------------
// gap_analyze
// -----------------------------------------------------------------------------

func TestGapAnalyze_StrongMatch_RuleOnly(t *testing.T) {
	// overlap >= 0.7,规则法直接 validate,不调 LLM
	stub := &stubChatModel{} // 没塞 response,如果被调用会 error
	node := NewGapAnalyzeNode(stub)
	sess := buildGapSession(
		[]string{"go", "redis", "mysql", "kafka"},
		[]string{"go", "redis", "mysql"},
	)
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if sess.GapReport.Strategy != domain.GapStrategyValidate {
		t.Errorf("expected validate, got %s", sess.GapReport.Strategy)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called for strong match, but was called %d times", stub.idx)
	}
}

func TestGapAnalyze_WeakMatch_RuleOnly(t *testing.T) {
	stub := &stubChatModel{}
	node := NewGapAnalyzeNode(stub)
	sess := buildGapSession(
		[]string{"go", "redis", "mysql", "kafka"},
		[]string{"java"},
	)
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if sess.GapReport.Strategy != domain.GapStrategyCoverGap {
		t.Errorf("expected cover_gap, got %s", sess.GapReport.Strategy)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called for weak match, but was called %d times", stub.idx)
	}
}

func TestGapAnalyze_MidMatch_LLMFallback(t *testing.T) {
	// overlap ≈ 0.5,落入中间地带,LLM 兜底
	stub := &stubChatModel{responses: []string{
		`{"strategy":"explore","reason":"年限够，建议探索式提问"}`,
	}}
	node := NewGapAnalyzeNode(stub)
	sess := buildGapSession(
		[]string{"go", "redis", "mysql", "kafka"},
		[]string{"go", "redis"},
	)
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if sess.GapReport.Strategy != domain.GapStrategyExplore {
		t.Errorf("expected explore, got %s", sess.GapReport.Strategy)
	}
	if stub.idx != 1 {
		t.Errorf("expected 1 LLM call, got %d", stub.idx)
	}
	if !strings.Contains(sess.GapReport.Reason, "探索") {
		t.Errorf("reason should be from LLM, got: %s", sess.GapReport.Reason)
	}
}

func TestGapAnalyze_LLMFails_DegradesToExplore(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("network error")}, responses: []string{""}}
	node := NewGapAnalyzeNode(stub)
	sess := buildGapSession(
		[]string{"go", "redis", "mysql", "kafka"},
		[]string{"go", "redis"},
	)
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should not return error on LLM fail, got: %v", err)
	}
	if sess.GapReport.Strategy != domain.GapStrategyExplore {
		t.Errorf("expected explore (degraded), got %s", sess.GapReport.Strategy)
	}
	if !strings.Contains(sess.GapReport.Reason, "降级") {
		t.Errorf("reason should mention 降级: %s", sess.GapReport.Reason)
	}
}

func TestGapAnalyze_MatchedMissingSetOps(t *testing.T) {
	stub := &stubChatModel{}
	node := NewGapAnalyzeNode(stub)
	sess := buildGapSession(
		[]string{"go", "redis", "mysql", "kafka"},
		[]string{"go", "redis", "mysql"},
	)
	_ = node(context.Background(), sess)

	matched := sess.GapReport.MatchedSkills
	missing := sess.GapReport.MissingSkills
	if len(matched) != 3 {
		t.Errorf("matched count = %d, want 3 (got %v)", len(matched), matched)
	}
	if len(missing) != 1 || missing[0] != "kafka" {
		t.Errorf("missing = %v, want [kafka]", missing)
	}
	// 字典序
	if !sortedAsc(matched) {
		t.Errorf("matched not sorted: %v", matched)
	}
}

func TestGapAnalyze_NilLLM_MidMatchExplore(t *testing.T) {
	node := NewGapAnalyzeNode(nil)
	sess := buildGapSession(
		[]string{"go", "redis", "mysql", "kafka"},
		[]string{"go", "redis"},
	)
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.GapReport.Strategy != domain.GapStrategyExplore {
		t.Errorf("expected explore for nil LLM mid match, got %s", sess.GapReport.Strategy)
	}
}

// -----------------------------------------------------------------------------
// analyze_profile
// -----------------------------------------------------------------------------

func TestAnalyzeProfile_BuildsExplainableMatchReport(t *testing.T) {
	node := NewAnalyzeProfileNode()
	sess := buildGapSession(
		[]string{"go", "redis", "kafka"},
		[]string{"go", "redis"},
	)
	sess.JobProfile.MustHave = []string{"go", "kafka"}
	sess.JobProfile.YearsRequired = 3
	sess.CandProfile.Years = 2
	sess.CandProfile.Projects = []domain.ResumeProject{
		{
			Name:       "秒杀系统",
			Role:       "后端主力",
			Highlights: []string{"用 Redis lua 实现库存预扣支撑 1w QPS"},
			Stack:      []string{"go", "redis"},
		},
	}
	sess.CandProfile.Highlights = []string{"用 Redis lua 实现库存预扣支撑 1w QPS"}
	_ = NewGapAnalyzeNode(nil)(context.Background(), sess)

	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	a := sess.ProfileAnalysis
	if a == nil {
		t.Fatal("profile analysis was not written")
	}
	if a.MatchScore <= 0 || a.MatchScore > 100 {
		t.Fatalf("bad score: %+v", a)
	}
	if !contains(a.MatchedRequirements, "go") || !contains(a.MissingRequirements, "kafka") {
		t.Fatalf("matched/missing wrong: %+v", a)
	}
	if len(a.RiskPoints) == 0 || !strings.Contains(strings.Join(a.RiskPoints, " "), "kafka") {
		t.Fatalf("risk points should mention kafka: %+v", a.RiskPoints)
	}
	if len(a.ResumeSuggestions) == 0 {
		t.Fatalf("resume suggestions empty: %+v", a)
	}
	if len(a.ProjectProbePlan) != 1 || !strings.Contains(a.ProjectProbePlan[0].SuggestedQuestion, "秒杀系统") {
		t.Fatalf("project probe plan wrong: %+v", a.ProjectProbePlan)
	}
}

func TestAnalyzeProfile_MissingInputsPermanent(t *testing.T) {
	node := NewAnalyzeProfileNode()
	err := node(context.Background(), &domain.Session{})
	if !errors.Is(err, graph.ErrPermanent) {
		t.Fatalf("expected permanent error, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// retrieve_rag
// -----------------------------------------------------------------------------

// stubEmbedder 返回固定向量,用于测试。
type stubEmbedder struct {
	dim int
	err error
}

func (e *stubEmbedder) Name() string   { return "stub" }
func (e *stubEmbedder) Dimension() int { return e.dim }
func (e *stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.err != nil {
		return nil, e.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, e.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}

// fakeRetriever 实现 retriever.Retriever 接口,返回预设结果。
type fakeRetriever struct {
	results []retriever.Result
	err     error
	calls   int
	lastQ   retriever.Query
}

func (r *fakeRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	r.calls++
	r.lastQ = q
	return r.results, r.err
}

type fakePipelineRetriever struct {
	fakeRetriever
	searchResult retriever.PipelineResult
	searchErr    error
	searchCalls  int
}

func (r *fakePipelineRetriever) Search(ctx context.Context, q retriever.Query) (retriever.PipelineResult, error) {
	r.searchCalls++
	r.lastQ = q
	return r.searchResult, r.searchErr
}

// newFakeRetriever 用 id 列表快速构造结果。
func newFakeRetriever(ids []string, err error) *fakeRetriever {
	out := make([]retriever.Result, len(ids))
	for i, id := range ids {
		out[i] = retriever.Result{
			ID:             id,
			Content:        "测试题 " + id,
			Tags:           []string{"go_concurrency"},
			Difficulty:     3,
			Category:       "go",
			ExpectedPoints: []string{"要点 " + id},
			Score:          0.9 - 0.05*float64(i),
		}
	}
	return &fakeRetriever{results: out, err: err}
}
func TestRetrieveRAG_Success(t *testing.T) {
	embedder := &stubEmbedder{dim: 1024}
	r := newFakeRetriever([]string{"go-001", "go-002", "redis-001"}, nil)
	node := NewRetrieveRAGNode(embedder, r, RetrieveRAGOptions{TopK: 10})

	sess := buildRAGSession([]string{"go", "redis"}, []string{"go"})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if len(sess.CandidatePool) != 3 {
		t.Errorf("pool size = %d, want 3", len(sess.CandidatePool))
	}
	for _, q := range sess.CandidatePool {
		if !strings.HasPrefix(q.Source, "rag-") {
			t.Errorf("expected rag- prefix, got source=%s", q.Source)
		}
	}
	if got := sess.CandidatePool[0].ExpectedPoints; len(got) != 1 || got[0] != "要点 go-001" {
		t.Fatalf("expected points should be copied from retriever result, got %+v", got)
	}
}

func TestRetrieveRAG_SavesRetrievalTraceWhenSearcherAvailable(t *testing.T) {
	embedder := &stubEmbedder{dim: 1024}
	r := &fakePipelineRetriever{
		searchResult: retriever.PipelineResult{
			Results: []retriever.Result{{
				ID:             "go-rrf-001",
				Content:        "Go GMP 调度",
				Tags:           []string{"go_concurrency"},
				Difficulty:     3,
				Category:       "go",
				ExpectedPoints: []string{"GMP"},
			}},
			Trace: retriever.RetrievalTrace{
				Query: "go",
				Stages: []retriever.StageTrace{{
					Stage: retriever.StageRerank,
					Count: 1,
					Items: []retriever.ResultTrace{{ID: "go-rrf-001", Rank: 1, Score: 0.9, Stage: retriever.StageRerank}},
				}},
				Final: []retriever.ResultTrace{{ID: "go-rrf-001", Rank: 1, Score: 0.9, Stage: retriever.StageRerank}},
			},
		},
	}
	node := NewRetrieveRAGNode(embedder, r, RetrieveRAGOptions{TopK: 10})

	sess := buildRAGSession([]string{"go"}, []string{"go"})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if r.searchCalls != 1 || r.calls != 0 {
		t.Fatalf("calls: search=%d retrieve=%d, want search only", r.searchCalls, r.calls)
	}
	if len(sess.CandidatePool) != 1 || sess.CandidatePool[0].ID != "go-rrf-001" {
		t.Fatalf("candidate pool = %+v", sess.CandidatePool)
	}
	if sess.RetrievalTrace == nil || len(sess.RetrievalTrace.Stages) != 1 {
		t.Fatalf("retrieval trace missing: %+v", sess.RetrievalTrace)
	}
	if sess.RetrievalTrace.Stages[0].Stage != retriever.StageRerank {
		t.Fatalf("trace stage = %+v, want rerank", sess.RetrievalTrace.Stages[0])
	}
}

func TestRetrieveRAG_PassesQuestionBankFilterToRetriever(t *testing.T) {
	embedder := &stubEmbedder{dim: 1024}
	r := newFakeRetriever([]string{"redis-001"}, nil)
	node := NewRetrieveRAGNode(embedder, r, RetrieveRAGOptions{TopK: 10})

	sess := buildRAGSession([]string{"go", "redis"}, []string{"redis"})
	sess.QuestionBankFilter = &domain.QuestionBankFilter{
		SkillCategories: []string{"redis"},
		Scenarios:       []string{"troubleshooting"},
		DifficultyMin:   2,
		DifficultyMax:   4,
		Tags:            []string{"cache"},
	}

	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	if got := r.lastQ.SkillCategories; len(got) != 1 || got[0] != "redis" {
		t.Fatalf("skill categories = %+v, want [redis]", got)
	}
	if got := r.lastQ.Scenarios; len(got) != 1 || got[0] != "troubleshooting" {
		t.Fatalf("scenarios = %+v, want [troubleshooting]", got)
	}
	if r.lastQ.DifficultyMin != 2 || r.lastQ.DifficultyMax != 4 {
		t.Fatalf("difficulty range = %d..%d, want 2..4", r.lastQ.DifficultyMin, r.lastQ.DifficultyMax)
	}
	if got := r.lastQ.FilterTags; len(got) != 1 || got[0] != "cache" {
		t.Fatalf("filter tags = %+v, want [cache]", got)
	}
}

func TestRetrieveRAG_EmbedderFails_Fallback(t *testing.T) {
	embedder := &stubEmbedder{dim: 1024, err: errors.New("embed boom")}
	r := newFakeRetriever(nil, nil)
	node := NewRetrieveRAGNode(embedder, r, RetrieveRAGOptions{TopK: 10})

	sess := buildRAGSession([]string{"go"}, []string{"go"})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should degrade, not fail: %v", err)
	}
	if len(sess.CandidatePool) == 0 {
		t.Fatal("expected fallback pool, got empty")
	}
	if sess.CandidatePool[0].Source != "fallback" {
		t.Errorf("expected fallback source, got %s", sess.CandidatePool[0].Source)
	}
	if len(sess.CandidatePool[0].ExpectedPoints) == 0 {
		t.Fatal("fallback questions should include expected points")
	}
	if sess.WorkingMemory == nil || sess.WorkingMemory.DegradedReasons["rag"] == "" {
		t.Errorf("expected rag degraded reason, got memory=%v", sess.WorkingMemory)
	}
}

func TestRetrieveRAG_RetrieverEmpty_Fallback(t *testing.T) {
	embedder := &stubEmbedder{dim: 1024}
	r := newFakeRetriever(nil, nil) // 空结果
	node := NewRetrieveRAGNode(embedder, r, RetrieveRAGOptions{TopK: 5})

	sess := buildRAGSession([]string{"go"}, []string{"go"})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.CandidatePool[0].Source != "fallback" {
		t.Errorf("expected fallback source, got %s", sess.CandidatePool[0].Source)
	}
}

func TestRetrieveRAG_FallbackHonorsQuestionBankFilter(t *testing.T) {
	embedder := &stubEmbedder{dim: 1024}
	r := newFakeRetriever(nil, errors.New("pg down"))
	node := NewRetrieveRAGNode(embedder, r, RetrieveRAGOptions{TopK: 5})

	sess := buildRAGSession([]string{"go", "redis"}, []string{"redis"})
	sess.QuestionBankFilter = &domain.QuestionBankFilter{SkillCategories: []string{"redis"}}

	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if len(sess.CandidatePool) == 0 {
		t.Fatal("expected filtered fallback pool")
	}
	for _, q := range sess.CandidatePool {
		if q.SkillCategory != "redis" {
			t.Fatalf("fallback should honor redis scope, got %+v", sess.CandidatePool)
		}
	}
}

func TestRetrieveRAG_DifficultyTuning(t *testing.T) {
	// validate → base+1; cover_gap → base-1; explore → base
	if got := tuneDifficulty(3, domain.GapStrategyValidate); got != 4 {
		t.Errorf("validate: got %d, want 4", got)
	}
	if got := tuneDifficulty(3, domain.GapStrategyCoverGap); got != 2 {
		t.Errorf("cover_gap: got %d, want 2", got)
	}
	if got := tuneDifficulty(3, domain.GapStrategyExplore); got != 3 {
		t.Errorf("explore: got %d, want 3", got)
	}
	// clamp
	if got := tuneDifficulty(5, domain.GapStrategyValidate); got != 5 {
		t.Errorf("clamp upper: got %d, want 5", got)
	}
	if got := tuneDifficulty(1, domain.GapStrategyCoverGap); got != 1 {
		t.Errorf("clamp lower: got %d, want 1", got)
	}
}

func TestRetrieveRAG_BuildQueryTags(t *testing.T) {
	job := &domain.JobProfile{
		KeySkills: []string{"go", "redis"},
		MustHave:  []string{"go"},
	}
	// missing 优先
	gap := &domain.GapReport{MissingSkills: []string{"kafka"}}
	if got := buildQueryTags(gap, job); got[0] != "kafka" {
		t.Errorf("missing should win, got %v", got)
	}
	// 没 missing 用 must_have
	gap = &domain.GapReport{}
	if got := buildQueryTags(gap, job); got[0] != "go" {
		t.Errorf("must_have should win, got %v", got)
	}
	// must_have 空用 key_skills
	job2 := &domain.JobProfile{KeySkills: []string{"redis"}}
	if got := buildQueryTags(&domain.GapReport{}, job2); got[0] != "redis" {
		t.Errorf("key_skills should win, got %v", got)
	}
	// 全空返回 nil
	if got := buildQueryTags(&domain.GapReport{}, &domain.JobProfile{}); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func sortedAsc(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}

func buildGapSession(jdSkills, candSkills []string) *domain.Session {
	return &domain.Session{
		JobProfile: &domain.JobProfile{
			Title:         "Go 后端",
			KeySkills:     jdSkills,
			YearsRequired: 3,
		},
		CandProfile: &domain.CandidateProfile{
			Skills: candSkills,
			Years:  3,
		},
	}
}

func buildRAGSession(jdKey, missing []string) *domain.Session {
	return &domain.Session{
		JobProfile: &domain.JobProfile{
			Title:     "Go 后端",
			KeySkills: jdKey,
			MustHave:  jdKey,
		},
		GapReport: &domain.GapReport{
			MissingSkills: missing,
			Strategy:      domain.GapStrategyExplore,
		},
	}
}
