package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

// buildProbeAskSession 准备一个 critic 已经给出 probe 信号的会话。
func buildProbeAskSession(hasSignal bool, probesUsed int) *domain.Session {
	wm := domain.NewWorkingMemory()
	wm.ProbesUsed = probesUsed
	return &domain.Session{
		WorkingMemory: wm,
		Rounds: []domain.AnswerRound{
			{
				RoundID:  "r1",
				Question: domain.Question{ID: "go-001", Content: "讲一下 GMP"},
				Answer:   "G 是 goroutine, M 是 thread, P 是 processor",
				Evaluation: &domain.Evaluation{
					QuestionID: "go-001",
					Score:      70,
				},
				CriticResult: &domain.Critic{
					HasProbeSignal: hasSignal,
					ProbeTopic:     "work stealing 的实现细节",
				},
			},
		},
	}
}

// -----------------------------------------------------------------------------
// probe_ask
// -----------------------------------------------------------------------------

func TestProbeAsk_Success_SuspendsAndRecordsFollowUp(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question":"work stealing 的触发时机和窃取目标是怎么选的?","reason":"候选人没讲 work stealing,需要确认深度"}`,
	}}
	sess := buildProbeAskSession(true, 0)
	node := NewProbeAskNode(stub, ProbeAskOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
	r := &sess.Rounds[0]
	if len(r.FollowUps) != 1 {
		t.Fatalf("FollowUps len = %d, want 1", len(r.FollowUps))
	}
	if r.FollowUps[0].Question == "" {
		t.Error("FollowUp question empty")
	}
	if r.FollowUps[0].Answer != "" {
		t.Errorf("FollowUp answer should be empty before resume, got %q", r.FollowUps[0].Answer)
	}
	if sess.WorkingMemory.ProbesUsed != 1 {
		t.Errorf("ProbesUsed = %d, want 1", sess.WorkingMemory.ProbesUsed)
	}
}

func TestProbeAskPatchNode_SuccessReturnsSuspendPatch(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question":"work stealing 的触发时机和窃取目标是怎么选的?","reason":"候选人没讲 work stealing,需要确认深度"}`,
	}}
	sess := buildProbeAskSession(true, 0)
	node := NewProbeAskPatchNode(stub, ProbeAskOptions{})

	patch, err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrSuspended) || !graph.IsPatchSuspend(err) {
		t.Fatalf("expected patch suspend, got %v", err)
	}
	if len(sess.Rounds[0].FollowUps) != 0 || sess.WorkingMemory.ProbesUsed != 0 {
		t.Fatalf("patch node should not apply directly: followups=%+v memory=%+v", sess.Rounds[0].FollowUps, sess.WorkingMemory)
	}
	if patch.AppendCurrentFollowUp == nil || patch.AppendCurrentFollowUp.Question == "" {
		t.Fatalf("patch followup = %+v", patch.AppendCurrentFollowUp)
	}
	if patch.WorkingMemory == nil || patch.WorkingMemory.ProbesUsed != 1 {
		t.Fatalf("patch working memory = %+v, want ProbesUsed=1", patch.WorkingMemory)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if len(sess.Rounds[0].FollowUps) != 1 || sess.Rounds[0].FollowUps[0].Answer != "" {
		t.Fatalf("followups after apply = %+v", sess.Rounds[0].FollowUps)
	}
	if sess.WorkingMemory.ProbesUsed != 1 {
		t.Fatalf("ProbesUsed after apply = %d, want 1", sess.WorkingMemory.ProbesUsed)
	}
}

func TestProbeAsk_NoSignal_Skips(t *testing.T) {
	stub := &stubChatModel{}
	sess := buildProbeAskSession(false, 0)
	node := NewProbeAskNode(stub, ProbeAskOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called, called %d times", stub.idx)
	}
	if len(sess.Rounds[0].FollowUps) != 0 {
		t.Error("FollowUps should remain empty")
	}
}

func TestProbeAsk_BudgetExhausted_Skips(t *testing.T) {
	stub := &stubChatModel{}
	sess := buildProbeAskSession(true, 4) // MaxProbes 默认 4
	node := NewProbeAskNode(stub, ProbeAskOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called when budget exhausted, called %d times", stub.idx)
	}
}

func TestProbeAsk_LLMFails_ClosesSignals(t *testing.T) {
	stub := &stubChatModel{
		errs:      []error{errors.New("boom"), errors.New("boom2")},
		responses: []string{"", ""},
	}
	sess := buildProbeAskSession(true, 0)
	node := NewProbeAskNode(stub, ProbeAskOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should degrade to nil, got %v", err)
	}
	c := sess.Rounds[0].CriticResult
	if c.HasProbeSignal {
		t.Error("LLM fail should close probe signal")
	}
	if c.ProbeTopic != "" {
		t.Errorf("topic should be cleared, got %q", c.ProbeTopic)
	}
	if sess.WorkingMemory.DegradedReasons["probe_ask"] == "" {
		t.Error("expected probe_ask degraded reason")
	}
	if len(sess.Rounds[0].FollowUps) != 0 {
		t.Error("FollowUps should not be appended on failure")
	}
}

func TestProbeAskPatchNode_LLMFailsReturnsFallbackPatch(t *testing.T) {
	stub := &stubChatModel{
		errs:      []error{errors.New("boom"), errors.New("boom2")},
		responses: []string{"", ""},
	}
	sess := buildProbeAskSession(true, 0)
	originalCritic := *sess.Rounds[0].CriticResult
	originalCritic.GroundedScore = 88
	originalCritic.NeedRefine = true
	originalCritic.Issues = []string{"keep"}
	originalCritic.Summary = "keep summary"
	sess.Rounds[0].CriticResult = &originalCritic
	node := NewProbeAskPatchNode(stub, ProbeAskOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("patch node should degrade to nil error, got %v", err)
	}
	if len(sess.Rounds[0].FollowUps) != 0 {
		t.Fatalf("patch node should not append followup directly, got %+v", sess.Rounds[0].FollowUps)
	}
	if !sess.Rounds[0].CriticResult.HasProbeSignal {
		t.Fatalf("patch node should not close critic directly, got %+v", sess.Rounds[0].CriticResult)
	}
	if patch.AppendCurrentFollowUp != nil {
		t.Fatalf("failure should not append followup, got %+v", patch.AppendCurrentFollowUp)
	}
	if patch.CurrentCriticProbeSignal == nil || patch.CurrentCriticProbeSignal.HasProbeSignal {
		t.Fatalf("patch critic probe signal = %+v", patch.CurrentCriticProbeSignal)
	}
	if patch.WorkingMemory == nil || patch.WorkingMemory.DegradedReasons["probe_ask"] == "" {
		t.Fatalf("patch working memory degraded reason missing: %+v", patch.WorkingMemory)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	c := sess.Rounds[0].CriticResult
	if c.HasProbeSignal || c.ProbeTopic != "" {
		t.Fatalf("probe signal should be closed after apply: %+v", c)
	}
	if c.GroundedScore != 88 || !c.NeedRefine || len(c.Issues) != 1 || c.Summary != "keep summary" {
		t.Fatalf("critic audit fields should be preserved: %+v", c)
	}
	if sess.WorkingMemory.DegradedReasons["probe_ask"] == "" {
		t.Fatalf("degraded reason after apply = %+v", sess.WorkingMemory.DegradedReasons)
	}
}

func TestProbeAsk_NoRound_Permanent(t *testing.T) {
	sess := &domain.Session{}
	node := NewProbeAskNode(&stubChatModel{}, ProbeAskOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestProbeAsk_NoCritic_Permanent(t *testing.T) {
	sess := &domain.Session{
		Rounds: []domain.AnswerRound{{RoundID: "r1", Question: domain.Question{ID: "q"}}},
	}
	node := NewProbeAskNode(&stubChatModel{}, ProbeAskOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// probe_eval
// -----------------------------------------------------------------------------

// buildProbeEvalSession 准备一个已经追问完、用户答完追答的会话。
func buildProbeEvalSession(answer string, probesUsed int) *domain.Session {
	wm := domain.NewWorkingMemory()
	wm.ProbesUsed = probesUsed
	return &domain.Session{
		WorkingMemory: wm,
		Rounds: []domain.AnswerRound{
			{
				RoundID:  "r1",
				Question: domain.Question{ID: "go-001", Content: "讲一下 GMP"},
				Answer:   "GMP 主答",
				Evaluation: &domain.Evaluation{
					QuestionID: "go-001",
					Score:      70,
				},
				CriticResult: &domain.Critic{
					HasProbeSignal: true,
					ProbeTopic:     "work stealing",
				},
				FollowUps: []domain.FollowUp{
					{
						Question: "work stealing 是怎么实现的?",
						Answer:   answer,
						Reason:   "需要深挖",
					},
				},
			},
		},
	}
}

func TestProbeEval_Success_WritesEvalAndUpdatesSignals(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"score":75,"strengths":["讲清了 P 的本地队列"],"weaknesses":["没讲全局队列的协作"],"suggestion":"补全两级队列协作","has_more_probe":true,"next_probe_topic":"全局队列的 lock 设计"}`,
	}}
	sess := buildProbeEvalSession("当某个 P 的队列空了,它会从其他 P 偷一半 G", 1)
	node := NewProbeEvalNode(stub, ProbeEvalOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	last := &sess.Rounds[0].FollowUps[0]
	if last.Evaluation == nil {
		t.Fatal("Evaluation not written")
	}
	if last.Evaluation.Score != 75 {
		t.Errorf("score = %d, want 75", last.Evaluation.Score)
	}
	c := sess.Rounds[0].CriticResult
	if !c.HasProbeSignal {
		t.Error("budget available + has_more_probe=true should keep signal on")
	}
	if c.ProbeTopic != "全局队列的 lock 设计" {
		t.Errorf("topic = %q, want updated topic", c.ProbeTopic)
	}
}

func TestProbeEvalPatchNode_SuccessReturnsEvaluationPatch(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"score":75,"strengths":["讲清了 P 的本地队列"],"weaknesses":["没讲全局队列的协作"],"suggestion":"补全两级队列协作","has_more_probe":true,"next_probe_topic":"全局队列的 lock 设计"}`,
	}}
	sess := buildProbeEvalSession("当某个 P 的队列空了,它会从其他 P 偷一半 G", 1)
	node := NewProbeEvalPatchNode(stub, ProbeEvalOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("patch node failed: %v", err)
	}
	if sess.Rounds[0].FollowUps[0].Evaluation != nil {
		t.Fatalf("patch node should not apply evaluation directly, got %+v", sess.Rounds[0].FollowUps[0].Evaluation)
	}
	if patch.CurrentFollowUpEvaluation == nil || patch.CurrentFollowUpEvaluation.Score != 75 {
		t.Fatalf("patch follow-up evaluation = %+v", patch.CurrentFollowUpEvaluation)
	}
	if patch.CurrentCriticProbeSignal == nil || !patch.CurrentCriticProbeSignal.HasProbeSignal {
		t.Fatalf("patch critic probe signal = %+v", patch.CurrentCriticProbeSignal)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if sess.Rounds[0].FollowUps[0].Evaluation == nil || sess.Rounds[0].FollowUps[0].Evaluation.Score != 75 {
		t.Fatalf("evaluation after apply = %+v", sess.Rounds[0].FollowUps[0].Evaluation)
	}
	if sess.Rounds[0].CriticResult.ProbeTopic != "全局队列的 lock 设计" {
		t.Fatalf("critic after apply = %+v", sess.Rounds[0].CriticResult)
	}
}

func TestProbeEval_NoMoreProbe_ClosesSignal(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"score":80,"strengths":["完整"],"weaknesses":[],"suggestion":"无","has_more_probe":false,"next_probe_topic":""}`,
	}}
	sess := buildProbeEvalSession("答得不错", 1)
	node := NewProbeEvalNode(stub, ProbeEvalOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	c := sess.Rounds[0].CriticResult
	if c.HasProbeSignal {
		t.Error("has_more_probe=false should close signal")
	}
	if c.ProbeTopic != "" {
		t.Errorf("topic should be cleared, got %q", c.ProbeTopic)
	}
}

func TestProbeEval_EmptyAnswer_ShortCircuits(t *testing.T) {
	stub := &stubChatModel{} // 不该被调用
	sess := buildProbeEvalSession("   ", 1)
	node := NewProbeEvalNode(stub, ProbeEvalOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called for empty answer, called %d", stub.idx)
	}
	last := &sess.Rounds[0].FollowUps[0]
	if last.Evaluation == nil || last.Evaluation.Score != 0 {
		t.Errorf("expected score=0 short-circuit eval, got %+v", last.Evaluation)
	}
	if sess.Rounds[0].CriticResult.HasProbeSignal {
		t.Error("empty answer should close probe signal")
	}
}

func TestProbeEvalPatchNode_EmptyAnswerReturnsCloseSignalPatch(t *testing.T) {
	stub := &stubChatModel{}
	sess := buildProbeEvalSession("   ", 1)
	node := NewProbeEvalPatchNode(stub, ProbeEvalOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("patch node failed: %v", err)
	}
	if stub.idx != 0 {
		t.Fatalf("LLM should not be called, called %d", stub.idx)
	}
	if patch.CurrentFollowUpEvaluation == nil || patch.CurrentFollowUpEvaluation.Score != 0 {
		t.Fatalf("patch follow-up evaluation = %+v", patch.CurrentFollowUpEvaluation)
	}
	if patch.CurrentCriticProbeSignal == nil || patch.CurrentCriticProbeSignal.HasProbeSignal {
		t.Fatalf("patch should close probe signal, got %+v", patch.CurrentCriticProbeSignal)
	}
	if sess.Rounds[0].FollowUps[0].Evaluation != nil || !sess.Rounds[0].CriticResult.HasProbeSignal {
		t.Fatalf("patch node should not apply directly: followup=%+v critic=%+v",
			sess.Rounds[0].FollowUps[0], sess.Rounds[0].CriticResult)
	}
}

func TestProbeEval_BudgetExhausted_SuppressesHasMoreProbe(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"score":70,"strengths":["x"],"weaknesses":["y"],"suggestion":"z","has_more_probe":true,"next_probe_topic":"再深挖"}`,
	}}
	sess := buildProbeEvalSession("答案", 4) // MaxProbes 默认 4,已耗尽
	node := NewProbeEvalNode(stub, ProbeEvalOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	c := sess.Rounds[0].CriticResult
	if c.HasProbeSignal {
		t.Error("budget exhausted should suppress has_more_probe")
	}
	if c.ProbeTopic != "" {
		t.Errorf("topic should be cleared, got %q", c.ProbeTopic)
	}
}

func TestProbeEval_LLMFails_DegradesAndClosesSignals(t *testing.T) {
	stub := &stubChatModel{
		errs:      []error{errors.New("boom"), errors.New("boom2")},
		responses: []string{"", ""},
	}
	sess := buildProbeEvalSession("答案", 1)
	node := NewProbeEvalNode(stub, ProbeEvalOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should degrade to nil, got %v", err)
	}
	last := &sess.Rounds[0].FollowUps[0]
	if last.Evaluation == nil || last.Evaluation.Score != -1 {
		t.Errorf("expected degraded eval score=-1, got %+v", last.Evaluation)
	}
	if !strings.Contains(last.Evaluation.Suggestion, "降级") {
		t.Errorf("suggestion should mention 降级, got %q", last.Evaluation.Suggestion)
	}
	if sess.Rounds[0].CriticResult.HasProbeSignal {
		t.Error("LLM fail should close probe signal")
	}
	if sess.WorkingMemory.DegradedReasons["probe_eval"] == "" {
		t.Error("expected probe_eval degraded reason")
	}
}

func TestProbeEvalPatchNode_LLMFailsReturnsDegradedPatch(t *testing.T) {
	stub := &stubChatModel{
		errs:      []error{errors.New("boom"), errors.New("boom2")},
		responses: []string{"", ""},
	}
	sess := buildProbeEvalSession("答案", 1)
	node := NewProbeEvalPatchNode(stub, ProbeEvalOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("patch node should degrade to nil error, got %v", err)
	}
	if sess.Rounds[0].FollowUps[0].Evaluation != nil {
		t.Fatalf("patch node should not apply evaluation directly, got %+v", sess.Rounds[0].FollowUps[0].Evaluation)
	}
	if patch.CurrentFollowUpEvaluation == nil || patch.CurrentFollowUpEvaluation.Score != -1 {
		t.Fatalf("patch follow-up evaluation = %+v", patch.CurrentFollowUpEvaluation)
	}
	if patch.CurrentCriticProbeSignal == nil || patch.CurrentCriticProbeSignal.HasProbeSignal {
		t.Fatalf("patch should close signal, got %+v", patch.CurrentCriticProbeSignal)
	}
	if patch.WorkingMemory == nil || patch.WorkingMemory.DegradedReasons["probe_eval"] == "" {
		t.Fatalf("patch working memory degraded reason missing: %+v", patch.WorkingMemory)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if sess.Rounds[0].FollowUps[0].Evaluation == nil || sess.Rounds[0].FollowUps[0].Evaluation.Score != -1 {
		t.Fatalf("evaluation after apply = %+v", sess.Rounds[0].FollowUps[0].Evaluation)
	}
	if sess.Rounds[0].CriticResult.HasProbeSignal {
		t.Fatalf("critic signal should be closed after apply: %+v", sess.Rounds[0].CriticResult)
	}
	if sess.WorkingMemory.DegradedReasons["probe_eval"] == "" {
		t.Fatalf("degraded reason after apply = %+v", sess.WorkingMemory)
	}
}

func TestProbeEval_NoFollowUp_Permanent(t *testing.T) {
	sess := &domain.Session{
		Rounds: []domain.AnswerRound{{RoundID: "r1", Question: domain.Question{ID: "q"}}},
	}
	node := NewProbeEvalNode(&stubChatModel{}, ProbeEvalOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestProbeEval_NoRound_Permanent(t *testing.T) {
	sess := &domain.Session{}
	node := NewProbeEvalNode(&stubChatModel{}, ProbeEvalOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}
