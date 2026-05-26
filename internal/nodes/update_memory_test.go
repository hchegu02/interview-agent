package nodes

import (
	"context"
	"errors"
	"math"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func buildUpdateMemSession(score int, skillCat string, tags []string) *domain.Session {
	return &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:            "go-001",
					Content:       "GMP",
					Tags:          tags,
					SkillCategory: skillCat,
				},
				Answer: "答案",
				Evaluation: &domain.Evaluation{
					QuestionID: "go-001",
					Score:      score,
				},
			},
		},
	}
}

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestUpdateMemory_HighScore_ConfirmsSkill(t *testing.T) {
	sess := buildUpdateMemSession(85, "go", []string{"go", "concurrency"})
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	mem := sess.WorkingMemory
	if !approxEq(mem.SkillCoverage["go"], 0.85) {
		t.Errorf("coverage[go] = %v, want 0.85", mem.SkillCoverage["go"])
	}
	// SkillCategory 非空, tags 不参与
	if _, ok := mem.SkillCoverage["concurrency"]; ok {
		t.Error("SkillCategory should take precedence over Tags")
	}
	if !contains(mem.ConfirmedSkills, "go") {
		t.Errorf("expected go in ConfirmedSkills, got %v", mem.ConfirmedSkills)
	}
	if contains(mem.WeakSkills, "go") {
		t.Error("go should not be in WeakSkills")
	}
	if !approxEq(mem.AvgScore, 85) {
		t.Errorf("AvgScore = %v, want 85", mem.AvgScore)
	}
	if sess.Rounds[0].CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
}

func TestUpdateMemory_LowScore_MarksWeak(t *testing.T) {
	sess := buildUpdateMemSession(30, "redis", nil)
	// 预置 redis 在 ConfirmedSkills (假设之前一题答对了)
	sess.WorkingMemory.ConfirmedSkills = []string{"redis"}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	mem := sess.WorkingMemory
	if !contains(mem.WeakSkills, "redis") {
		t.Errorf("expected redis in WeakSkills, got %v", mem.WeakSkills)
	}
	if contains(mem.ConfirmedSkills, "redis") {
		t.Errorf("redis should be removed from ConfirmedSkills, got %v", mem.ConfirmedSkills)
	}
}

func TestUpdateMemory_MidScore_NoSetChange(t *testing.T) {
	sess := buildUpdateMemSession(60, "go", nil)
	sess.WorkingMemory.ConfirmedSkills = []string{"foo"}
	sess.WorkingMemory.WeakSkills = []string{"bar"}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	_ = node(context.Background(), sess)
	mem := sess.WorkingMemory
	// go 既不进 Confirmed 也不进 Weak
	if contains(mem.ConfirmedSkills, "go") || contains(mem.WeakSkills, "go") {
		t.Errorf("mid score should not change set membership: %+v", mem)
	}
	// 已有标签未被破坏
	if !contains(mem.ConfirmedSkills, "foo") || !contains(mem.WeakSkills, "bar") {
		t.Error("existing labels should remain")
	}
}

func TestUpdateMemory_WithFollowUps_WeightedCombined(t *testing.T) {
	sess := buildUpdateMemSession(60, "go", nil)
	// 追答评估给 90 分, combined = 60*0.7 + 90*0.3 / (0.7+0.3) = 69
	sess.Rounds[0].FollowUps = []domain.FollowUp{
		{
			Question:   "深挖问题",
			Answer:     "深挖答案",
			Evaluation: &domain.Evaluation{Score: 90},
		},
	}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	_ = node(context.Background(), sess)
	mem := sess.WorkingMemory
	// 期望 coverage[go] ≈ 0.69
	if !approxEq(mem.SkillCoverage["go"], 0.69) {
		t.Errorf("coverage[go] = %v, want 0.69", mem.SkillCoverage["go"])
	}
	if !approxEq(mem.AvgScore, 69) {
		t.Errorf("AvgScore = %v, want 69", mem.AvgScore)
	}
}

func TestUpdateMemory_FollowUpDegraded_Ignored(t *testing.T) {
	sess := buildUpdateMemSession(80, "go", nil)
	// 追答评估 score=-1, 应该被跳过, combined 退化为 80
	sess.Rounds[0].FollowUps = []domain.FollowUp{
		{Evaluation: &domain.Evaluation{Score: -1}},
		{Evaluation: nil},
	}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	_ = node(context.Background(), sess)
	if !approxEq(sess.WorkingMemory.SkillCoverage["go"], 0.80) {
		t.Errorf("coverage[go] = %v, want 0.80 (followups ignored)", sess.WorkingMemory.SkillCoverage["go"])
	}
}

func TestUpdateMemory_RefinedEval_TakesPrecedence(t *testing.T) {
	sess := buildUpdateMemSession(80, "go", nil)
	sess.Rounds[0].RefinedEval = &domain.Evaluation{Score: 55}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	_ = node(context.Background(), sess)
	// FinalEvaluation 应返回 refined => 0.55
	if !approxEq(sess.WorkingMemory.SkillCoverage["go"], 0.55) {
		t.Errorf("coverage[go] = %v, want 0.55 (refined wins)", sess.WorkingMemory.SkillCoverage["go"])
	}
}

func TestUpdateMemory_DegradedRound_Skipped(t *testing.T) {
	sess := buildUpdateMemSession(-1, "go", nil)
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	mem := sess.WorkingMemory
	if len(mem.SkillCoverage) != 0 {
		t.Errorf("degraded round should not touch SkillCoverage, got %v", mem.SkillCoverage)
	}
	if mem.AvgScore != 0 {
		t.Errorf("AvgScore should remain 0, got %v", mem.AvgScore)
	}
	if mem.DegradedRounds != 1 {
		t.Errorf("expected DegradedRounds=1, got %d", mem.DegradedRounds)
	}
	if sess.Rounds[0].CompletedAt.IsZero() {
		t.Error("CompletedAt should still be set on degraded round")
	}
}

func TestUpdateMemory_AvgScore_IncrementalAcrossRounds(t *testing.T) {
	sess := buildUpdateMemSession(80, "go", nil)
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	_ = node(context.Background(), sess)
	// 模拟第二轮: 加一道新 round
	sess.Rounds = append(sess.Rounds, domain.AnswerRound{
		RoundID:  "r2",
		Question: domain.Question{ID: "redis-001", SkillCategory: "redis"},
		Evaluation: &domain.Evaluation{QuestionID: "redis-001", Score: 60},
	})
	_ = node(context.Background(), sess)
	// 两轮 80 + 60, AvgScore 应 = 70
	if !approxEq(sess.WorkingMemory.AvgScore, 70) {
		t.Errorf("AvgScore = %v, want 70", sess.WorkingMemory.AvgScore)
	}
	if sess.WorkingMemory.ScoredRounds != 2 {
		t.Errorf("ScoredRounds = %d, want 2", sess.WorkingMemory.ScoredRounds)
	}
}

func TestUpdateMemory_TagsFallback_WhenNoCategory(t *testing.T) {
	sess := buildUpdateMemSession(85, "", []string{"go", "concurrency"})
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	_ = node(context.Background(), sess)
	mem := sess.WorkingMemory
	if !approxEq(mem.SkillCoverage["go"], 0.85) || !approxEq(mem.SkillCoverage["concurrency"], 0.85) {
		t.Errorf("both tags should be credited, got %v", mem.SkillCoverage)
	}
}

func TestUpdateMemory_NoRound_Permanent(t *testing.T) {
	sess := &domain.Session{}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func TestUpdateMemory_NoEval_Permanent(t *testing.T) {
	sess := &domain.Session{
		Rounds: []domain.AnswerRound{{RoundID: "r1", Question: domain.Question{ID: "q"}}},
	}
	node := NewUpdateMemoryNode(UpdateMemoryOptions{})
	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}
