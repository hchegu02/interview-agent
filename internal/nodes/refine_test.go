package nodes

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func buildRefineSession(needRefine bool, origScore int) *domain.Session {
	return &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{
			{
				RoundID:  "r1",
				Question: domain.Question{ID: "go-001", Content: "GMP", ExpectedPoints: []string{"G/M/P"}},
				Answer:   "G 是 goroutine",
				Evaluation: &domain.Evaluation{
					QuestionID: "go-001",
					Score:      origScore,
					Strengths:  []string{"提到 G"},
					Weaknesses: []string{"没讲完整"},
					Suggestion: "需补充",
				},
				CriticResult: &domain.Critic{
					GroundedScore: 40,
					NeedRefine:    needRefine,
					Issues:        []string{"原评估太宽容,答了一半就给 80 分"},
					Summary:       "评估偏高",
				},
			},
		},
	}
}

func TestRefine_Success_WritesRefinedEval(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question_id":"go-001","score":55,"strengths":["提到 G"],"weaknesses":["未讲 M/P","未提 work stealing"],"suggestion":"补全 GMP 三者关系"}`,
	}}
	sess := buildRefineSession(true, 80)
	node := NewRefineNode(stub, RefineOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	r := &sess.Rounds[0]
	if r.RefinedEval == nil {
		t.Fatal("RefinedEval not written")
	}
	if r.RefinedEval.Score != 55 {
		t.Errorf("refined score = %d, want 55", r.RefinedEval.Score)
	}
	// 原 Evaluation 保留
	if r.Evaluation.Score != 80 {
		t.Errorf("original eval should be preserved, got %d", r.Evaluation.Score)
	}
	// FinalEvaluation 返回 refined
	if r.FinalEvaluation().Score != 55 {
		t.Errorf("FinalEvaluation should return refined, got %d", r.FinalEvaluation().Score)
	}
}

func TestRefinePatchNode_ReturnsPatchWithoutMutatingSession(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question_id":"go-001","score":55,"strengths":["提到 G"],"weaknesses":["未讲 M/P"],"suggestion":"补全 GMP 三者关系"}`,
	}}
	sess := buildRefineSession(true, 80)
	node := NewRefinePatchNode(stub, RefineOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Rounds[0].RefinedEval != nil {
		t.Fatalf("patch node should not mutate session directly: %+v", sess.Rounds[0].RefinedEval)
	}
	if patch.CurrentRefinedEvaluation == nil || patch.CurrentRefinedEvaluation.Score != 55 {
		t.Fatalf("refined patch = %+v", patch.CurrentRefinedEvaluation)
	}

	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if sess.Rounds[0].RefinedEval == nil || sess.Rounds[0].FinalEvaluation().Score != 55 {
		t.Fatalf("refined after apply = %+v", sess.Rounds[0].RefinedEval)
	}
}

func TestRefine_NotNeeded_SkipsLLM(t *testing.T) {
	stub := &stubChatModel{}
	sess := buildRefineSession(false, 80)
	node := NewRefineNode(stub, RefineOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called when need_refine=false, called %d", stub.idx)
	}
	if sess.Rounds[0].RefinedEval != nil {
		t.Error("RefinedEval should remain nil")
	}
}

func TestRefinePatchNode_NotNeededReturnsEmptyPatch(t *testing.T) {
	stub := &stubChatModel{}
	sess := buildRefineSession(false, 80)
	node := NewRefinePatchNode(stub, RefineOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called when need_refine=false, called %d", stub.idx)
	}
	if patch.CurrentRefinedEvaluation != nil || patch.WorkingMemory != nil {
		t.Fatalf("expected empty patch, got %+v", patch)
	}
}

func TestRefine_LLMFails_KeepsOriginal(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("boom"), errors.New("boom2")}, responses: []string{"", ""}}
	sess := buildRefineSession(true, 80)
	node := NewRefineNode(stub, RefineOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should not return err, got %v", err)
	}
	r := &sess.Rounds[0]
	if r.RefinedEval != nil {
		t.Error("RefinedEval should be nil on failure")
	}
	if r.Evaluation.Score != 80 {
		t.Error("original eval should be preserved")
	}
	if r.FinalEvaluation().Score != 80 {
		t.Error("FinalEvaluation should fall back to original")
	}
	if sess.WorkingMemory.DegradedReasons["refine"] == "" {
		t.Error("expected refine degraded reason")
	}
}

func TestRefinePatchNode_LLMFailsReturnsWorkingMemoryPatch(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("boom")}, responses: []string{""}}
	sess := buildRefineSession(true, 80)
	node := NewRefinePatchNode(stub, RefineOptions{})

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("should not return err, got %v", err)
	}
	if patch.CurrentRefinedEvaluation != nil {
		t.Fatalf("refined eval should remain nil on failure, got %+v", patch.CurrentRefinedEvaluation)
	}
	if patch.WorkingMemory == nil || patch.WorkingMemory.DegradedReasons["refine"] == "" {
		t.Fatalf("expected refine degraded reason patch, got %+v", patch.WorkingMemory)
	}
	if sess.Rounds[0].RefinedEval != nil || sess.WorkingMemory.DegradedReasons["refine"] != "" {
		t.Fatalf("patch node should not mutate session directly: refined=%+v memory=%+v",
			sess.Rounds[0].RefinedEval, sess.WorkingMemory)
	}
}

func TestRefine_NoCritic_Permanent(t *testing.T) {
	sess := &domain.Session{
		Rounds: []domain.AnswerRound{
			{RoundID: "r1", Question: domain.Question{ID: "q"},
				Evaluation: &domain.Evaluation{Score: 50}},
		},
	}
	node := NewRefineNode(&stubChatModel{}, RefineOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestRefine_NilLLM_Degrades(t *testing.T) {
	sess := buildRefineSession(true, 80)
	node := NewRefineNode(nil, RefineOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.Rounds[0].RefinedEval != nil {
		t.Error("nil LLM should leave RefinedEval nil")
	}
}
