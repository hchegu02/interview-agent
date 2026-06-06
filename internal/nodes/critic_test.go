package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func buildCriticSession(evalScore int, answer string) *domain.Session {
	sess := &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:             "go-001",
					Content:        "讲一下 GMP",
					ExpectedPoints: []string{"G/M/P 定义", "work stealing"},
				},
				Answer: answer,
				Evaluation: &domain.Evaluation{
					QuestionID: "go-001",
					Score:      evalScore,
					Strengths:  []string{"提到 G/M/P"},
					Weaknesses: []string{"没讲 work stealing"},
					Suggestion: "补充 work stealing",
				},
			},
		},
	}
	return sess
}

func TestCritic_GoodEval_NoRefine_NoProbe(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"grounded_score":85,"need_refine":false,"issues":[],"summary":"评估准确","has_probe_signal":false,"probe_topic":""}`,
	}}
	sess := buildCriticSession(78, "G 是 goroutine, M 是 thread, P 是 processor")
	node := NewCriticNode(stub, CriticOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	c := sess.Rounds[0].CriticResult
	if c == nil {
		t.Fatal("critic result not written")
	}
	if c.NeedRefine || c.HasProbeSignal {
		t.Errorf("unexpected signals: %+v", c)
	}
	if c.GroundedScore != 85 {
		t.Errorf("grounded = %d, want 85", c.GroundedScore)
	}
}

func TestCritic_LowGroundedScore_TriggersRefine(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		// LLM 自己说 need_refine=false, 但 grounded_score=40 低于 threshold=60
		`{"grounded_score":40,"need_refine":false,"issues":["x"],"summary":"评估太松","has_probe_signal":false,"probe_topic":""}`,
	}}
	sess := buildCriticSession(80, "答案")
	node := NewCriticNode(stub, CriticOptions{RefineThreshold: 60})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if !sess.Rounds[0].CriticResult.NeedRefine {
		t.Error("grounded_score<threshold should force need_refine=true")
	}
}

func TestCritic_ProbeSignal_BudgetExhausted_Suppressed(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"grounded_score":80,"need_refine":false,"issues":[],"summary":"ok","has_probe_signal":true,"probe_topic":"秒杀场景细节"}`,
	}}
	sess := buildCriticSession(70, "答案")
	sess.WorkingMemory.ProbesUsed = sess.WorkingMemory.MaxProbes // 预算耗尽
	node := NewCriticNode(stub, CriticOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	c := sess.Rounds[0].CriticResult
	if c.HasProbeSignal {
		t.Error("budget exhausted should suppress probe signal")
	}
	if c.ProbeTopic != "" {
		t.Errorf("topic should be cleared when probe disabled, got %q", c.ProbeTopic)
	}
}

func TestCritic_ProbeSignal_WithBudget_KeptOn(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"grounded_score":80,"need_refine":false,"issues":[],"summary":"ok","has_probe_signal":true,"probe_topic":"Redis lua 库存"}`,
	}}
	sess := buildCriticSession(70, "答案")
	// 预算还有
	node := NewCriticNode(stub, CriticOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	c := sess.Rounds[0].CriticResult
	if !c.HasProbeSignal {
		t.Error("budget available should keep probe signal")
	}
	if c.ProbeTopic == "" {
		t.Error("topic should be kept")
	}
}

func TestCriticPatchNode_ReturnsPatchWithoutMutatingSession(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"grounded_score":82,"need_refine":false,"issues":[],"summary":"评估可信","has_probe_signal":true,"probe_topic":"work stealing"}`,
	}}
	sess := buildCriticSession(75, "答案")
	node := NewCriticPatchNode(stub, CriticOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Rounds[0].CriticResult != nil {
		t.Fatalf("patch node should not mutate session directly: %+v", sess.Rounds[0].CriticResult)
	}
	if patch.CurrentCriticResult == nil {
		t.Fatal("expected critic result patch")
	}
	if patch.CurrentCriticResult.GroundedScore != 82 || !patch.CurrentCriticResult.HasProbeSignal {
		t.Fatalf("critic patch = %+v", patch.CurrentCriticResult)
	}

	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if sess.Rounds[0].CriticResult == nil || sess.Rounds[0].CriticResult.ProbeTopic != "work stealing" {
		t.Fatalf("critic after apply = %+v", sess.Rounds[0].CriticResult)
	}
}

func TestCriticPatchNode_LLMFailsReturnsCriticAndWorkingMemoryPatch(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("boom")}, responses: []string{""}}
	sess := buildCriticSession(70, "answer")
	node := NewCriticPatchNode(stub, CriticOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("should not return err, got %v", err)
	}
	if patch.CurrentCriticResult == nil || patch.CurrentCriticResult.GroundedScore != -1 {
		t.Fatalf("expected degraded critic patch, got %+v", patch.CurrentCriticResult)
	}
	if patch.WorkingMemory == nil || patch.WorkingMemory.DegradedReasons["critic"] == "" {
		t.Fatalf("expected critic degraded reason patch, got %+v", patch.WorkingMemory)
	}
	if sess.Rounds[0].CriticResult != nil || sess.WorkingMemory.DegradedReasons["critic"] != "" {
		t.Fatalf("patch node should not mutate session directly: critic=%+v memory=%+v",
			sess.Rounds[0].CriticResult, sess.WorkingMemory)
	}
}

func TestCriticPatchNode_DegradedEvalShortCircuits(t *testing.T) {
	stub := &stubChatModel{}
	sess := buildCriticSession(-1, "answer")
	node := NewCriticPatchNode(stub, CriticOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called when eval score=-1, called %d times", stub.idx)
	}
	if patch.CurrentCriticResult == nil || patch.CurrentCriticResult.GroundedScore != -1 || patch.CurrentCriticResult.HasProbeSignal {
		t.Fatalf("expected passthrough critic patch, got %+v", patch.CurrentCriticResult)
	}
}

func TestCritic_DegradedEval_ShortCircuits(t *testing.T) {
	stub := &stubChatModel{} // 不放 response, 如果被调用会 err
	sess := buildCriticSession(-1, "answer")
	node := NewCriticNode(stub, CriticOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called when eval score=-1, called %d times", stub.idx)
	}
	c := sess.Rounds[0].CriticResult
	if c == nil || c.NeedRefine || c.HasProbeSignal {
		t.Errorf("expected passthrough critic, got %+v", c)
	}
}

func TestCritic_LLMFails_DegradesPassThrough(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("boom"), errors.New("boom2")}, responses: []string{"", ""}}
	sess := buildCriticSession(70, "answer")
	node := NewCriticNode(stub, CriticOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should not return err, got %v", err)
	}
	c := sess.Rounds[0].CriticResult
	if c.NeedRefine || c.HasProbeSignal {
		t.Errorf("degraded critic should not signal refine/probe, got %+v", c)
	}
	if sess.WorkingMemory.DegradedReasons["critic"] == "" {
		t.Errorf("expected critic degraded reason")
	}
	if !strings.Contains(c.Summary, "降级") {
		t.Errorf("summary should mention 降级: %s", c.Summary)
	}
}

func TestCritic_NoEval_Permanent(t *testing.T) {
	sess := &domain.Session{
		Rounds: []domain.AnswerRound{{RoundID: "r1", Question: domain.Question{ID: "q"}}},
	}
	node := NewCriticNode(&stubChatModel{}, CriticOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestCritic_NilLLM_Degrades(t *testing.T) {
	sess := buildCriticSession(70, "answer")
	node := NewCriticNode(nil, CriticOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	c := sess.Rounds[0].CriticResult
	if c.NeedRefine || c.HasProbeSignal {
		t.Errorf("nil LLM critic should pass through, got %+v", c)
	}
}
