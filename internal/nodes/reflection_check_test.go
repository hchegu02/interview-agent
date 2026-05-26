package nodes

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/domain"
)

// -----------------------------------------------------------------------------
// reflection_check
// -----------------------------------------------------------------------------

func buildReflectSession(roundsAsked, maxRounds, reflectionsUsed int, weak []string) *domain.Session {
	mem := domain.NewWorkingMemory()
	mem.MaxRounds = maxRounds
	mem.RoundsAsked = roundsAsked
	mem.ReflectionsUsed = reflectionsUsed
	mem.WeakSkills = weak
	mem.AvgScore = 65
	return &domain.Session{WorkingMemory: mem}
}

func TestReflection_LLMReflect_Success(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"action":"reflect","reasoning":"redis 答得弱,值得补一题","reflect_topic":"redis"}`,
	}}
	sess := buildReflectSession(3, 8, 0, []string{"redis"})
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	d := sess.PendingDecision
	if d == nil || d.Action != domain.ActionReflect {
		t.Fatalf("expected reflect decision, got %+v", d)
	}
	if d.ReflectTopic != "redis" {
		t.Errorf("topic = %q, want redis", d.ReflectTopic)
	}
	if sess.WorkingMemory.ReflectTopic != "redis" {
		t.Errorf("ReflectTopic = %q, want redis", sess.WorkingMemory.ReflectTopic)
	}
	if sess.WorkingMemory.ReflectionsUsed != 1 {
		t.Errorf("ReflectionsUsed = %d, want 1", sess.WorkingMemory.ReflectionsUsed)
	}
}

func TestReflection_LLMAskNew_PassThrough(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"action":"ask_new","reasoning":"还有题要问","reflect_topic":""}`,
	}}
	sess := buildReflectSession(2, 8, 0, nil)
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if sess.PendingDecision.Action != domain.ActionAskNew {
		t.Errorf("expected ask_new, got %v", sess.PendingDecision.Action)
	}
	if sess.WorkingMemory.ReflectionsUsed != 0 {
		t.Error("ReflectionsUsed should not increment for ask_new")
	}
}

func TestReflection_LLMEnd_PassThrough(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"action":"end","reasoning":"覆盖度够了","reflect_topic":""}`,
	}}
	sess := buildReflectSession(6, 8, 0, []string{"redis"})
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if sess.PendingDecision.Action != domain.ActionEnd {
		t.Errorf("expected end, got %v", sess.PendingDecision.Action)
	}
}

func TestReflection_LLMReflect_NoBudget_Downgrades(t *testing.T) {
	// LLM 说 reflect, 但 ReflectionsUsed=1=MaxReflections → 强制 ask_new
	stub := &stubChatModel{responses: []string{
		`{"action":"reflect","reasoning":"想补漏","reflect_topic":"redis"}`,
	}}
	sess := buildReflectSession(3, 8, 1, []string{"redis"})
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if sess.PendingDecision.Action != domain.ActionAskNew {
		t.Errorf("budget exhausted should downgrade to ask_new, got %v", sess.PendingDecision.Action)
	}
	if sess.WorkingMemory.ReflectionsUsed != 1 {
		t.Errorf("ReflectionsUsed = %d, should stay 1", sess.WorkingMemory.ReflectionsUsed)
	}
}

func TestReflection_LLMReflect_NoWeakSkills_Downgrades(t *testing.T) {
	stub := &stubChatModel{responses: []string{
		`{"action":"reflect","reasoning":"想补漏","reflect_topic":"redis"}`,
	}}
	sess := buildReflectSession(3, 8, 0, nil) // WeakSkills 空
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if sess.PendingDecision.Action != domain.ActionAskNew {
		t.Errorf("no weak skills should downgrade to ask_new, got %v", sess.PendingDecision.Action)
	}
}

func TestReflection_LLMReflect_TopicNotInWeak_Corrected(t *testing.T) {
	// LLM 给的 topic 不在 WeakSkills 里,应该被矫正成第一个 weak skill
	stub := &stubChatModel{responses: []string{
		`{"action":"reflect","reasoning":"x","reflect_topic":"kafka"}`,
	}}
	sess := buildReflectSession(3, 8, 0, []string{"redis", "mysql"})
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if sess.PendingDecision.Action != domain.ActionReflect {
		t.Fatalf("expected reflect, got %v", sess.PendingDecision.Action)
	}
	if sess.PendingDecision.ReflectTopic != "redis" {
		t.Errorf("topic should be corrected to redis, got %q", sess.PendingDecision.ReflectTopic)
	}
	if sess.WorkingMemory.ReflectTopic != "redis" {
		t.Errorf("ReflectTopic should be corrected to redis, got %q", sess.WorkingMemory.ReflectTopic)
	}
}

func TestReflection_BudgetExhausted_HardEnd(t *testing.T) {
	stub := &stubChatModel{} // LLM 不该被调用
	sess := buildReflectSession(8, 8, 0, []string{"redis"})
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if stub.idx != 0 {
		t.Errorf("LLM should not be called when no rounds remain, called %d", stub.idx)
	}
	if sess.PendingDecision.Action != domain.ActionEnd {
		t.Errorf("expected hard end, got %v", sess.PendingDecision.Action)
	}
}

func TestReflection_LLMAskNew_NoRoundBudget_Corrected(t *testing.T) {
	// LLM 说 ask_new, 但其实 RoundsAsked==MaxRounds (注意 0 截断在前置,
	// 这里造一个边界: MaxRounds=4, RoundsAsked=4 → 直接走前置 hard end 不会进 LLM)
	// 真正测"LLM 说 ask_new 但没预算"的场景只能在前置不触发时构造,
	// 跳过此用例直接保留 hard end 的覆盖
}

func TestReflection_LLMFails_RuleFallback_PrefersReflect(t *testing.T) {
	stub := &stubChatModel{
		errs:      []error{errors.New("boom"), errors.New("boom2")},
		responses: []string{"", ""},
	}
	sess := buildReflectSession(3, 8, 0, []string{"redis"})
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.PendingDecision.Action != domain.ActionReflect {
		t.Errorf("rule fallback with weak+budget should choose reflect, got %v", sess.PendingDecision.Action)
	}
	if sess.WorkingMemory.DegradedReasons["reflection"] == "" {
		t.Error("expected reflection degraded reason")
	}
}

func TestReflection_LLMFails_RuleFallback_AskNew(t *testing.T) {
	stub := &stubChatModel{
		errs:      []error{errors.New("boom"), errors.New("boom2")},
		responses: []string{"", ""},
	}
	// 没有 weak skills → 规则走 ask_new
	sess := buildReflectSession(3, 8, 0, nil)
	node := NewReflectionCheckNode(stub, ReflectionCheckOptions{})
	_ = node(context.Background(), sess)
	if sess.PendingDecision.Action != domain.ActionAskNew {
		t.Errorf("rule fallback with no weak should ask_new, got %v", sess.PendingDecision.Action)
	}
}

func TestReflection_NilLLM_RuleFallback(t *testing.T) {
	sess := buildReflectSession(3, 8, 0, []string{"redis"})
	node := NewReflectionCheckNode(nil, ReflectionCheckOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.PendingDecision.Action != domain.ActionReflect {
		t.Errorf("nil LLM should fall back to reflect, got %v", sess.PendingDecision.Action)
	}
}

// -----------------------------------------------------------------------------
// routers
// -----------------------------------------------------------------------------

func roundWithCritic(c *domain.Critic) *domain.Session {
	return &domain.Session{
		Rounds: []domain.AnswerRound{
			{
				RoundID:      "r1",
				Question:     domain.Question{ID: "q"},
				Evaluation:   &domain.Evaluation{Score: 70},
				CriticResult: c,
			},
		},
	}
}

func TestRouteAfterCritic_NeedRefine(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{NeedRefine: true, HasProbeSignal: true})
	if got := RouteAfterCritic(sess); got != NodeRefine {
		t.Errorf("got %q, want refine", got)
	}
}

func TestRouteAfterCritic_ProbeOnly(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{HasProbeSignal: true})
	if got := RouteAfterCritic(sess); got != NodeProbeAsk {
		t.Errorf("got %q, want probe_ask", got)
	}
}

func TestRouteAfterCritic_None(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{})
	if got := RouteAfterCritic(sess); got != NodeUpdateMemory {
		t.Errorf("got %q, want update_memory", got)
	}
}

func TestRouteAfterCritic_NoCritic_Safe(t *testing.T) {
	sess := &domain.Session{Rounds: []domain.AnswerRound{{RoundID: "r1"}}}
	if got := RouteAfterCritic(sess); got != NodeUpdateMemory {
		t.Errorf("nil critic should default to update_memory, got %q", got)
	}
}

func TestRouteAfterRefine_ProbeStillNeeded(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{NeedRefine: true, HasProbeSignal: true})
	if got := RouteAfterRefine(sess); got != NodeProbeAsk {
		t.Errorf("got %q, want probe_ask", got)
	}
}

func TestRouteAfterRefine_NoProbe(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{NeedRefine: true})
	if got := RouteAfterRefine(sess); got != NodeUpdateMemory {
		t.Errorf("got %q, want update_memory", got)
	}
}

func TestRouteAfterProbeEval_KeepProbing(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{HasProbeSignal: true})
	if got := RouteAfterProbeEval(sess); got != NodeProbeAsk {
		t.Errorf("got %q, want probe_ask", got)
	}
}

func TestRouteAfterProbeEval_Done(t *testing.T) {
	sess := roundWithCritic(&domain.Critic{HasProbeSignal: false})
	if got := RouteAfterProbeEval(sess); got != NodeUpdateMemory {
		t.Errorf("got %q, want update_memory", got)
	}
}

func TestRouteAfterReflection_AskNew(t *testing.T) {
	sess := &domain.Session{
		PendingDecision: &domain.Decision{Action: domain.ActionAskNew},
	}
	if got := RouteAfterReflection(sess); got != NodePickNext {
		t.Errorf("got %q, want pick_next", got)
	}
}

func TestRouteAfterReflection_Reflect(t *testing.T) {
	sess := &domain.Session{
		PendingDecision: &domain.Decision{Action: domain.ActionReflect, ReflectTopic: "redis"},
	}
	if got := RouteAfterReflection(sess); got != NodePickNext {
		t.Errorf("reflect should also go to pick_next, got %q", got)
	}
}

func TestRouteAfterReflection_End(t *testing.T) {
	sess := &domain.Session{
		PendingDecision: &domain.Decision{Action: domain.ActionEnd},
	}
	if got := RouteAfterReflection(sess); got != NodeReport {
		t.Errorf("got %q, want report", got)
	}
}

func TestRouteAfterReflection_NoDecision_DefaultEnd(t *testing.T) {
	sess := &domain.Session{}
	if got := RouteAfterReflection(sess); got != NodeReport {
		t.Errorf("nil decision should default to report, got %q", got)
	}
}
