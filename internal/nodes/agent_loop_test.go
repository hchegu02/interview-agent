package nodes

import (
	"context"
	"strings"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// agent_loop_test.go: Agent 子图的联合单测。
//
// 用 graph 框架真实组装 pick_next → evaluate → critic →
//   (refine?) → (probe_ask ↔ probe_eval)+ → update_memory → reflection_check
// 形成完整循环, 用 stubChatModel 预装 LLM 响应序列驱动。
//
// 测试目标:
//   1. 节点之间的状态接力是否正确(round / critic / refined_eval / followups / memory)
//   2. 多轮 probe / refine / reflect 分支按 router 设计走通
//   3. suspend/resume 在 pick_next + probe_ask 两个挂起点都对
//   4. 预算约束在多轮 probe / 反思场景下生效

// -----------------------------------------------------------------------------
// 子图组装 + 辅助
// -----------------------------------------------------------------------------

// buildAgentSubgraph 组装一个最小可跑的 Agent 子图(不含 setup 阶段)。
// 入口是 pick_next, 出口是 report。
func buildAgentSubgraph(t *testing.T, model llm.ChatModel) *graph.Runnable {
	t.Helper()
	g, err := graph.New("agent_subgraph").
		AddNode(NodePickNext, NewPickNextNode(model, PickNextOptions{})).
		AddNode(NodeEvaluate, NewEvaluateNode(model, EvaluateOptions{})).
		AddNode("critic", NewCriticNode(model, CriticOptions{})).
		AddNode(NodeRefine, NewRefineNode(model, RefineOptions{})).
		AddNode(NodeProbeAsk, NewProbeAskNode(model, ProbeAskOptions{})).
		AddNode("probe_eval", NewProbeEvalNode(model, ProbeEvalOptions{})).
		AddNode(NodeUpdateMemory, NewUpdateMemoryNode(UpdateMemoryOptions{})).
		AddNode(NodeUpdateDifficulty, NewUpdateDifficultyNode(UpdateDifficultyOptions{})).
		AddNode("reflection_check", NewReflectionCheckNode(model, ReflectionCheckOptions{MinRounds: 1})).
		AddNode(NodeReport, NewReportNode()).
		Entry(NodePickNext).
		AddBranch(NodePickNext, RouteAfterPickNext).
		AddEdge(NodeEvaluate, "critic").
		AddBranch("critic", RouteAfterCritic).
		AddBranch(NodeRefine, RouteAfterRefine).
		AddEdge(NodeProbeAsk, "probe_eval").
		AddBranch("probe_eval", RouteAfterProbeEval).
		AddEdge(NodeUpdateMemory, NodeUpdateDifficulty).
		AddEdge(NodeUpdateDifficulty, "reflection_check").
		AddBranch("reflection_check", RouteAfterReflection).
		AddEdge(NodeReport, graph.EndNode).
		Compile()
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	return g
}

// buildAgentSession 准备一个最小 Session: 候选池 + JobProfile + WorkingMemory。
func buildAgentSession(pool []domain.Question, maxRounds, maxProbes int) *domain.Session {
	mem := domain.NewWorkingMemory()
	mem.MaxRounds = maxRounds
	mem.MaxProbes = maxProbes
	return &domain.Session{
		ID:     "agent-loop-test",
		Status: domain.StatusRunning,
		JobProfile: &domain.JobProfile{
			Title:         "Go 后端",
			YearsRequired: 3,
			KeySkills:     []string{"go", "redis"},
		},
		CandProfile:   &domain.CandidateProfile{Years: 3, Skills: []string{"go"}},
		GapReport:     &domain.GapReport{Strategy: domain.GapStrategyExplore},
		CandidatePool: pool,
		WorkingMemory: mem,
	}
}

func agentSamplePool() []domain.Question {
	return []domain.Question{
		{ID: "q1", Content: "讲一下 GMP", Tags: []string{"go"}, Difficulty: 3, Source: "rag-1", SkillCategory: "go"},
		{ID: "q2", Content: "Redis 分布式锁", Tags: []string{"redis"}, Difficulty: 3, Source: "rag-2", SkillCategory: "redis"},
		{ID: "q3", Content: "MySQL MVCC", Tags: []string{"mysql"}, Difficulty: 4, Source: "rag-3", SkillCategory: "mysql"},
		{ID: "q4", Content: "Kafka 顺序消费", Tags: []string{"kafka"}, Difficulty: 4, Source: "rag-4", SkillCategory: "kafka"},
	}
}

// fillMainAnswer 在 pick_next suspend 之后填主答, 模拟用户提交。
func fillMainAnswer(t *testing.T, sess *domain.Session, answer string) {
	t.Helper()
	if len(sess.Rounds) == 0 {
		t.Fatal("no round to answer")
	}
	sess.Rounds[len(sess.Rounds)-1].Answer = answer
}

// fillFollowUpAnswer 在 probe_ask suspend 之后填追答。
func fillFollowUpAnswer(t *testing.T, sess *domain.Session, answer string) {
	t.Helper()
	if len(sess.Rounds) == 0 {
		t.Fatal("no round")
	}
	r := &sess.Rounds[len(sess.Rounds)-1]
	if len(r.FollowUps) == 0 {
		t.Fatal("no follow-up to answer")
	}
	r.FollowUps[len(r.FollowUps)-1].Answer = answer
}

// 简化构造各类节点的 LLM 响应。
const (
	pickQ1 = `{"next_question_id":"q1","reasoning":"先验证 go 基本功"}`
	pickQ2 = `{"next_question_id":"q2","reasoning":"再看 redis"}`

	evalGood = `{"question_id":"q1","score":75,"strengths":["G/M/P 都讲到"],"weaknesses":["没讲 work stealing"],"suggestion":"补 work stealing"}`
	evalQ2   = `{"question_id":"q2","score":70,"strengths":["分布式锁能讲 setnx"],"weaknesses":["lua 脚本没提"],"suggestion":"补 lua"}`
	evalLow  = `{"question_id":"q1","score":85,"strengths":["看上去答全"],"weaknesses":[],"suggestion":"无"}`

	criticPass   = `{"grounded_score":85,"need_refine":false,"issues":[],"summary":"评估准确","has_probe_signal":false,"probe_topic":""}`
	criticPassQ2 = `{"grounded_score":80,"need_refine":false,"issues":[],"summary":"ok","has_probe_signal":false,"probe_topic":""}`
	criticRefine = `{"grounded_score":40,"need_refine":true,"issues":["原评估过高"],"summary":"虚高","has_probe_signal":false,"probe_topic":""}`
	criticProbe  = `{"grounded_score":80,"need_refine":false,"issues":[],"summary":"还有可挖","has_probe_signal":true,"probe_topic":"work stealing 细节"}`

	refinedDown = `{"question_id":"q1","score":55,"strengths":["G/M/P 名词到位"],"weaknesses":["未讲 work stealing","未讲调度协作"],"suggestion":"补全两点"}`

	probeAskWS  = `{"question":"work stealing 触发时机是怎样的?","reason":"候选人没讲触发逻辑"}`
	probeAskWS2 = `{"question":"那窃取的对象是怎么选的?","reason":"继续深挖"}`

	probeEvalEnd  = `{"score":70,"strengths":["讲了触发"],"weaknesses":["没讲窃取目标"],"suggestion":"够用","has_more_probe":false,"next_probe_topic":""}`
	probeEvalMore = `{"score":70,"strengths":["讲了触发"],"weaknesses":["浅"],"suggestion":"继续问","has_more_probe":true,"next_probe_topic":"窃取目标选择"}`

	reflectAskNew = `{"action":"ask_new","reasoning":"继续覆盖其他技能","reflect_topic":""}`
	reflectEnd    = `{"action":"end","reasoning":"覆盖度已经够","reflect_topic":""}`
)

// -----------------------------------------------------------------------------
// scenario 1: happy path 2 轮 → end
// -----------------------------------------------------------------------------

func TestAgentLoop_HappyPath_TwoRounds(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		// round 1
		pickQ1,        // pick_next
		evalGood,      // evaluate
		criticPass,    // critic
		reflectAskNew, // reflection_check
		// round 2
		pickQ2,       // pick_next
		evalQ2,       // evaluate
		criticPassQ2, // critic
		reflectEnd,   // reflection_check
	}}
	r := buildAgentSubgraph(t, stub)
	sess := buildAgentSession(agentSamplePool(), 8, 4)

	// 第一轮: pick_next suspend
	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if sess.CurrentNode != NodePickNext {
		t.Fatalf("expected suspend at pick_next, got %q", sess.CurrentNode)
	}
	if len(sess.Rounds) != 1 || sess.Rounds[0].Question.ID != "q1" {
		t.Fatalf("round 1 not created: %+v", sess.Rounds)
	}

	// 填主答, resume
	fillMainAnswer(t, sess, "G 是 goroutine, M 是 OS 线程, P 是逻辑处理器")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume r1: %v", err)
	}
	// 第二轮: 应该又 suspend 在 pick_next
	if sess.CurrentNode != NodePickNext {
		t.Fatalf("expected suspend at pick_next for r2, got %q", sess.CurrentNode)
	}
	if len(sess.Rounds) != 2 || sess.Rounds[1].Question.ID != "q2" {
		t.Fatalf("round 2 not created: %+v", sess.Rounds)
	}

	// 验证 round 1 的状态结算完毕
	if sess.Rounds[0].Evaluation == nil || sess.Rounds[0].Evaluation.Score != 75 {
		t.Errorf("r1 evaluation wrong: %+v", sess.Rounds[0].Evaluation)
	}
	if sess.Rounds[0].CompletedAt.IsZero() {
		t.Error("r1 not marked completed")
	}

	// 答 round 2, resume → 走到 reflection end → report
	fillMainAnswer(t, sess, "redis SETNX + EXPIRE,还需要看 watchdog")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume r2: %v", err)
	}
	if sess.Report == nil {
		t.Errorf("expected report to be written")
	}
	if sess.WorkingMemory.AvgScore <= 0 {
		t.Errorf("AvgScore should be updated, got %v", sess.WorkingMemory.AvgScore)
	}
	if sess.WorkingMemory.Difficulty == nil || sess.WorkingMemory.Difficulty.Current != domain.DifficultyMedium {
		t.Errorf("difficulty should stay medium, got %+v", sess.WorkingMemory.Difficulty)
	}
	if stub.idx != 8 {
		t.Errorf("expected 8 LLM calls, got %d", stub.idx)
	}
}

// -----------------------------------------------------------------------------
// scenario 2: refine 分支
// -----------------------------------------------------------------------------

func TestAgentLoop_RefineBranch(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		pickQ1,
		evalLow,      // 原评估 85
		criticRefine, // critic 认为虚高, NeedRefine=true
		refinedDown,  // refine 修正到 55
		reflectEnd,
	}}
	r := buildAgentSubgraph(t, stub)
	sess := buildAgentSession(agentSamplePool(), 8, 4)

	_ = r.Invoke(context.Background(), sess)
	fillMainAnswer(t, sess, "答案")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("did not reach report")
	}

	round := &sess.Rounds[0]
	if round.RefinedEval == nil {
		t.Fatal("refined eval should be written")
	}
	if round.RefinedEval.Score != 55 {
		t.Errorf("refined score = %d, want 55", round.RefinedEval.Score)
	}
	// 原 evaluation 保留
	if round.Evaluation.Score != 85 {
		t.Errorf("original eval should be preserved, got %d", round.Evaluation.Score)
	}
	// SkillCoverage 用的是 refined (FinalEvaluation)
	if cov := sess.WorkingMemory.SkillCoverage["go"]; cov < 0.54 || cov > 0.56 {
		t.Errorf("coverage[go] should be ~0.55 (refined), got %v", cov)
	}
}

// -----------------------------------------------------------------------------
// scenario 3: 单轮 probe
// -----------------------------------------------------------------------------

func TestAgentLoop_SingleProbe(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		pickQ1,
		evalGood,
		criticProbe,  // HasProbeSignal=true
		probeAskWS,   // probe_ask 生成追问
		probeEvalEnd, // probe_eval has_more=false → 关闭信号
		reflectEnd,
	}}
	r := buildAgentSubgraph(t, stub)
	sess := buildAgentSession(agentSamplePool(), 8, 4)

	// 第一次 invoke → suspend 在 pick_next
	_ = r.Invoke(context.Background(), sess)
	if sess.CurrentNode != NodePickNext {
		t.Fatalf("expected suspend at pick_next, got %q", sess.CurrentNode)
	}
	fillMainAnswer(t, sess, "主答")

	// resume → 评估 + critic 给信号 + probe_ask 挂起
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume to probe_ask: %v", err)
	}
	if sess.CurrentNode != NodeProbeAsk {
		t.Fatalf("expected suspend at probe_ask, got %q", sess.CurrentNode)
	}
	round := &sess.Rounds[0]
	if len(round.FollowUps) != 1 {
		t.Fatalf("expected 1 follow-up created, got %d", len(round.FollowUps))
	}
	if round.FollowUps[0].Question == "" {
		t.Error("follow-up question empty")
	}
	if sess.WorkingMemory.ProbesUsed != 1 {
		t.Errorf("ProbesUsed = %d, want 1", sess.WorkingMemory.ProbesUsed)
	}

	// 填追答 → resume → probe_eval (no more) → update_memory → reflection end
	fillFollowUpAnswer(t, sess, "work stealing 是 P 队列空时从其他 P 偷一半")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume to report: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("did not reach report")
	}
	// 主答 75 + 追答 70 加权 (0.7/0.3) ≈ 73.5 → coverage[go] ≈ 0.735
	if cov := sess.WorkingMemory.SkillCoverage["go"]; cov < 0.7 || cov > 0.76 {
		t.Errorf("coverage[go] = %v, expected ~0.735 (weighted main+followup)", cov)
	}
}

func TestAgentLoop_LowInformationWeakRecallClarifiesBeforeProbeLLM(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		pickQ1,
		evalGood,
		criticProbe,
		probeEvalEnd,
		reflectEnd,
	}}
	r := buildAgentSubgraph(t, stub)
	sess := buildAgentSession(agentSamplePool(), 8, 4)
	sess.RetrievalTrace = &domain.RetrievalTrace{
		Final: []domain.RetrievalResultTrace{{ID: "q1", Rank: 1, Score: 0.1}},
	}

	_ = r.Invoke(context.Background(), sess)
	fillMainAnswer(t, sess, "不知道")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume to deterministic clarification: %v", err)
	}
	if sess.CurrentNode != NodeProbeAsk {
		t.Fatalf("expected suspend at probe_ask, got %q", sess.CurrentNode)
	}
	if len(sess.Rounds[0].FollowUps) != 1 {
		t.Fatalf("followups = %+v", sess.Rounds[0].FollowUps)
	}
	if got := sess.Rounds[0].FollowUps[0].Question; !strings.Contains(got, "信息量较少") {
		t.Fatalf("followup question = %q", got)
	}
	if stub.idx != 3 {
		t.Fatalf("probe_ask should not call LLM; calls = %d", stub.idx)
	}

	fillFollowUpAnswer(t, sess, "补充回答")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume to report: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("did not reach report")
	}
	if sess.WorkingMemory.DegradedReasons["retrieval_decision"] != "weak_recall_low_information_answer" {
		t.Fatalf("degraded reasons = %+v", sess.WorkingMemory.DegradedReasons)
	}
}

// -----------------------------------------------------------------------------
// scenario 4: 多轮 probe 循环 + budget 压制
// -----------------------------------------------------------------------------

func TestAgentLoop_MultiProbe_BudgetClamps(t *testing.T) {
	// MaxProbes=2: 允许 2 次追问, 第 2 次后即使 LLM 想继续, budget 也会压制
	stub := &stubChatModel{responses: []string{
		pickQ1,
		evalGood,
		criticProbe, // → probe_ask #1
		probeAskWS,
		probeEvalMore, // has_more=true → 继续
		probeAskWS2,   // probe_ask #2 (ProbesUsed 此时变 2)
		probeEvalMore, // LLM 还想 more, 但 ProbesUsed=2==MaxProbes → 节点压制
		reflectEnd,
	}}
	r := buildAgentSubgraph(t, stub)
	sess := buildAgentSession(agentSamplePool(), 8, 2) // MaxProbes=2

	_ = r.Invoke(context.Background(), sess) // suspend pick_next
	fillMainAnswer(t, sess, "主答")
	_ = r.Resume(context.Background(), sess) // suspend probe_ask #1
	if sess.CurrentNode != NodeProbeAsk {
		t.Fatalf("expected probe_ask suspend #1, got %q", sess.CurrentNode)
	}
	fillFollowUpAnswer(t, sess, "追答 1")

	_ = r.Resume(context.Background(), sess) // suspend probe_ask #2
	if sess.CurrentNode != NodeProbeAsk {
		t.Fatalf("expected probe_ask suspend #2, got %q", sess.CurrentNode)
	}
	if len(sess.Rounds[0].FollowUps) != 2 {
		t.Fatalf("expected 2 follow-ups, got %d", len(sess.Rounds[0].FollowUps))
	}
	fillFollowUpAnswer(t, sess, "追答 2")

	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("final resume: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("did not reach report")
	}
	// 验证 budget 压制: 即使 LLM 想 more, probe_eval 第二次应把信号关掉
	if sess.WorkingMemory.ProbesUsed != 2 {
		t.Errorf("ProbesUsed = %d, want 2 (clamped)", sess.WorkingMemory.ProbesUsed)
	}
	if sess.Rounds[0].CriticResult.HasProbeSignal {
		t.Error("HasProbeSignal should be cleared by budget pressure")
	}
	// 两次追答都有 evaluation
	for i, f := range sess.Rounds[0].FollowUps {
		if f.Evaluation == nil {
			t.Errorf("followup %d missing evaluation", i)
		}
	}
}

// -----------------------------------------------------------------------------
// 防御: pick_next pool 耗尽 → 直接进 report
// -----------------------------------------------------------------------------

func TestAgentLoop_PoolExhausted_FastEnd(t *testing.T) {
	// 仅 1 题, MaxRounds=8 但 pool 用完后 pick_next 直接 set Action=end
	stub := &stubChatModel{responses: []string{
		pickQ1,
		evalGood,
		criticPass,
		reflectAskNew, // LLM 想继续 ask_new
		// 第二轮 pick_next: 池已空 → 不调 LLM, 直接 Action=end → report
	}}
	r := buildAgentSubgraph(t, stub)
	pool := []domain.Question{samplePool()[0]}
	sess := buildAgentSession(pool, 8, 4)

	_ = r.Invoke(context.Background(), sess)
	fillMainAnswer(t, sess, "答")
	if err := r.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("expected report after pool exhausted")
	}
	// pick_next 第二次进, 但池空走 end 分支不调 LLM, 所以 LLM 总调用 = 4
	if stub.idx != 4 {
		t.Errorf("LLM call count = %d, want 4 (2nd pick_next short-circuits)", stub.idx)
	}
	if sess.Status != domain.StatusCompleted {
		t.Errorf("status = %q, want completed", sess.Status)
	}
	if sess.PendingDecision != nil {
		t.Errorf("report should clear pending decision, got %+v", sess.PendingDecision)
	}
}
