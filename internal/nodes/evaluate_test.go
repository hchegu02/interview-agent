package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"interview-agent/internal/agentkit"
	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func buildEvalSession(answer string, expected []string) *domain.Session {
	return &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:             "go-001",
					Content:        "讲一下 GMP",
					ExpectedPoints: expected,
					SkillCategory:  "go",
				},
				Answer: answer,
			},
		},
		PendingDecision: &domain.Decision{Action: domain.ActionAskNew}, // 应被 evaluate 清掉
	}
}

func TestEvaluate_Success(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question_id":"go-001","score":78,"strengths":["讲清 G/M/P 三者"],"weaknesses":["没提 work stealing"],"suggestion":"补充 work stealing 细节"}`,
	}}
	sess := buildEvalSession("G 是 goroutine, M 是线程, P 是 processor...", []string{"G/M/P", "work stealing"})
	node := NewEvaluateNode(stub, EvaluateOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}
	r := sess.CurrentRound()
	if r == nil {
		// CompletedAt 没写,所以 CurrentRound 应该还指向 r1
		// 如果意外被标 completed 则失败
		t.Fatal("round should still be current (CompletedAt not set yet)")
	}
	if r.Evaluation == nil {
		t.Fatal("evaluation not written")
	}
	if r.Evaluation.Score != 78 {
		t.Errorf("score = %d, want 78", r.Evaluation.Score)
	}
	if r.Evaluation.QuestionID != "go-001" {
		t.Errorf("question_id mismatch: %s", r.Evaluation.QuestionID)
	}
	if sess.PendingDecision != nil {
		t.Errorf("PendingDecision should be cleared after eval")
	}
}

func TestEvaluate_EmitsSkillHookEvents(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question_id":"go-001","score":78,"strengths":["讲清 G/M/P 三者"],"weaknesses":["没提 work stealing"],"suggestion":"补充 work stealing 细节"}`,
	}}
	hook := agentkit.NewRecorderHook()
	sess := buildEvalSession("G 是 goroutine, M 是线程, P 是 processor...", []string{"G/M/P", "work stealing"})
	sess.ID = "sess-eval-hook"
	node := NewEvaluateNode(stub, EvaluateOptions{Hook: hook})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("node failed: %v", err)
	}

	events := hook.Events()
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Type != agentkit.HookBeforeSkill || events[1].Type != agentkit.HookAfterSkill {
		t.Fatalf("event types = %+v", events)
	}
	if events[0].Name != "answer.evaluate" || events[1].Name != "answer.evaluate" {
		t.Fatalf("event names = %+v", events)
	}
	if events[1].OutputSummary != "question_id=go-001 score=78" {
		t.Fatalf("output summary = %q", events[1].OutputSummary)
	}
}

func TestEvaluate_EmptyAnswer_ShortCircuits(t *testing.T) {
	stub := &stubChatModel{} // 不放 response,如果被调用会 err
	sess := buildEvalSession("   ", []string{"x", "y"})
	node := NewEvaluateNode(stub, EvaluateOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if stub.idx != 0 {
		t.Errorf("LLM should not be called for empty answer, called %d times", stub.idx)
	}
	r := sess.Rounds[0]
	if r.Evaluation == nil || r.Evaluation.Score != 0 {
		t.Errorf("expected score=0 for empty answer, got %+v", r.Evaluation)
	}
	if !strings.Contains(r.Evaluation.Suggestion, "未作答") {
		t.Errorf("suggestion should mention 未作答, got: %s", r.Evaluation.Suggestion)
	}
}

func TestEvaluate_NoExpectedPoints_StillEvaluates(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"question_id":"go-001","score":60,"strengths":["有思路"],"weaknesses":["细节不足"],"suggestion":"展开讲讲"}`,
	}}
	sess := buildEvalSession("我觉得 GMP 是这样...", nil)
	node := NewEvaluateNode(stub, EvaluateOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.Rounds[0].Evaluation.Score != 60 {
		t.Errorf("score = %d, want 60", sess.Rounds[0].Evaluation.Score)
	}
}

func TestEvaluate_InvalidScore_SchemaSelfCorrect(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		// 越界 score,被 validator 拒
		`{"question_id":"go-001","score":150,"strengths":[],"weaknesses":[],"suggestion":"x"}`,
		// 自纠正后合法
		`{"question_id":"go-001","score":80,"strengths":["a"],"weaknesses":["b"],"suggestion":"c"}`,
	}}
	sess := buildEvalSession("answer", []string{"p1"})
	node := NewEvaluateNode(stub, EvaluateOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if stub.idx != 2 {
		t.Errorf("expected 2 LLM calls, got %d", stub.idx)
	}
	if sess.Rounds[0].Evaluation.Score != 80 {
		t.Errorf("score = %d, want 80", sess.Rounds[0].Evaluation.Score)
	}
}

func TestEvaluate_LLMFails_DegradesWithScoreMinusOne(t *testing.T) {
	stub := &stubChatModel{errs: []error{errors.New("boom"), errors.New("boom2")}, responses: []string{"", ""}}
	sess := buildEvalSession("answer", []string{"p1"})
	node := NewEvaluateNode(stub, EvaluateOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("should not return err on degrade, got %v", err)
	}
	r := sess.Rounds[0]
	if r.Evaluation == nil || r.Evaluation.Score != -1 {
		t.Errorf("expected score=-1 (degraded), got %+v", r.Evaluation)
	}
	if !strings.Contains(r.Evaluation.Suggestion, "评估失败") {
		t.Errorf("degraded suggestion expected, got: %s", r.Evaluation.Suggestion)
	}
	if sess.WorkingMemory.DegradedReasons["eval"] == "" {
		t.Errorf("expected eval degraded reason, got %v", sess.WorkingMemory.DegradedReasons)
	}
}

func TestEvaluate_NilLLM_Degrades(t *testing.T) {
	sess := buildEvalSession("有内容", []string{"p1"})
	node := NewEvaluateNode(nil, EvaluateOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.Rounds[0].Evaluation.Score != -1 {
		t.Errorf("nil LLM should degrade, got %+v", sess.Rounds[0].Evaluation)
	}
}

func TestEvaluate_NoCurrentRound_Permanent(t *testing.T) {
	sess := &domain.Session{}
	node := NewEvaluateNode(&stubChatModel{}, EvaluateOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}
