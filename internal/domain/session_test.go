package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSessionStatus_Validate(t *testing.T) {
	cases := []struct {
		s       SessionStatus
		wantErr bool
	}{
		{StatusCreated, false},
		{StatusRunning, false},
		{StatusPaused, false},
		{StatusCompleted, false},
		{StatusFailed, false},
		{SessionStatus(""), true},
		{SessionStatus("unknown"), true},
	}
	for _, c := range cases {
		err := c.s.Validate()
		if (err != nil) != c.wantErr {
			t.Errorf("Validate(%q) err=%v wantErr=%v", c.s, err, c.wantErr)
		}
	}
}

// TestSession_JSONRoundTrip 验证整个 Session 聚合可以无损 JSON 序列化。
// 这点很重要：Redis snapshot / PG state_json / HTTP response 都靠它。
func TestSession_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := &Session{
		ID:          "01HXYZ",
		UserID:      "u-1",
		Status:      StatusRunning,
		CurrentNode: "evaluate",
		JobProfile: &JobProfile{
			Title:     "Go Backend",
			Level:     "senior",
			KeySkills: []string{"go", "redis", "pg"},
		},
		CandProfile: &CandidateProfile{
			Years:  3,
			Skills: []string{"go", "mysql"},
		},
		ProfileAnalysis: &ProfileAnalysis{
			MatchScore:          72,
			Summary:             "岗位匹配度中等，建议探索项目细节。",
			YearsGap:            0,
			MatchedRequirements: []string{"go"},
			MissingRequirements: []string{"redis"},
			Strengths:           []string{"技能命中：go"},
			RiskPoints:          []string{"JD 技能未覆盖：redis。"},
			ResumeSuggestions:   []string{"补充 redis 的项目证据。"},
			QuestionFocus:       []string{"redis"},
			ProjectProbePlan: []ProjectProbePlan{
				{
					ProjectName:       "缓存服务",
					Focus:             "redis",
					Evidence:          "做过缓存优化",
					SuggestedQuestion: "你在缓存服务中如何落地 redis？",
				},
			},
		},
		CandidatePool: []Question{
			{ID: "q1", Content: "what is GMP", Tags: []string{"go"}, Difficulty: 3, Source: "rag-1"},
		},
		RetrievalTrace: &RetrievalTrace{
			Query: "Go Backend 岗位面试题，重点考察：go",
			Stages: []RetrievalStageTrace{
				{
					Stage: "vector",
					Count: 1,
					Items: []RetrievalResultTrace{{ID: "q1", Rank: 1, Score: 0.9, Stage: "vector"}},
				},
			},
			Final: []RetrievalResultTrace{{ID: "q1", Rank: 1, Score: 0.9, Stage: "rerank"}},
		},
		WorkingMemory: NewWorkingMemory(),
		Rounds: []AnswerRound{
			{
				RoundID:    "r1",
				Question:   Question{ID: "q1", Content: "GMP?", Difficulty: 3, SkillCategory: "go"},
				PickReason: "Go 是 KeySkill，必须先考",
				Answer:     "GMP is...",
				Evaluation: &Evaluation{QuestionID: "q1", Score: 80, Strengths: []string{"清晰"}},
				CriticResult: &Critic{
					GroundedScore: 75, NeedRefine: false, Summary: "评估合理",
				},
				FollowUps: []FollowUp{
					{Question: "channel 怎么实现的？", Answer: "...", Reason: "用户没提到底层"},
				},
				DecidedAt:   now,
				CompletedAt: now,
			},
		},
		PendingDecision: &Decision{
			Action:    ActionAskNew,
			Reasoning: "Go 已确认，下一题切到 Redis",
			DecidedAt: now,
		},
		Report: &Report{
			SessionID:      "sess-001",
			OverallScore:   80,
			SkillBreakdown: map[string]int{"go": 80},
			TranscriptAnalysis: &TranscriptAnalysis{
				RoundsAnalyzed:     1,
				AverageAnswerChars: 42,
				Dimensions: []TranscriptDimension{
					{Name: "技术相关性", Score: 80, Evidence: []string{"平均分 80"}, Advice: "继续保持"},
				},
				Patterns: []string{"回答有结构"},
			},
			DrillPlan: []DrillPlanItem{
				{
					PracticeOrder:          1,
					Skill:                  "redis",
					Reason:                 "薄弱技能",
					TargetScore:            75,
					RecommendedQuestionIDs: []string{"redis-001"},
					RecommendedQuestions:   []string{"缓存击穿怎么处理？"},
				},
			},
			Highlights:   []string{"亮点"},
			Improvements: []string{"改进"},
			NextSteps:    []string{"下一步"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Session
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != original.ID || got.Status != original.Status {
		t.Errorf("scalar mismatch: %+v", got)
	}
	if got.JobProfile == nil || got.JobProfile.Title != "Go Backend" {
		t.Errorf("job profile lost: %+v", got.JobProfile)
	}
	if got.ProfileAnalysis == nil || got.ProfileAnalysis.MatchScore != 72 {
		t.Errorf("profile analysis lost: %+v", got.ProfileAnalysis)
	}
	if len(got.Rounds) != 1 || got.Rounds[0].Question.ID != "q1" {
		t.Errorf("rounds lost: %+v", got.Rounds)
	}
	if got.PendingDecision == nil || got.PendingDecision.Action != ActionAskNew {
		t.Errorf("pending decision lost: %+v", got.PendingDecision)
	}
	if got.Report == nil || got.Report.TranscriptAnalysis == nil || len(got.Report.DrillPlan) != 1 {
		t.Errorf("report analysis lost: %+v", got.Report)
	}
	if got.RetrievalTrace == nil || got.RetrievalTrace.Stages[0].Stage != "vector" {
		t.Errorf("retrieval trace lost: %+v", got.RetrievalTrace)
	}
	if got.WorkingMemory == nil || got.WorkingMemory.MaxRounds != 8 {
		t.Errorf("working memory lost: %+v", got.WorkingMemory)
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("time mismatch: %v vs %v", got.CreatedAt, original.CreatedAt)
	}
}

// TestSession_OmitEmpty 验证 created 状态的空 Session 不会写一堆 null。
func TestSession_OmitEmpty(t *testing.T) {
	s := &Session{
		ID:     "01HXYZ",
		UserID: "u-1",
		Status: StatusCreated,
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	for _, banned := range []string{
		"job_profile", "candidate_profile", "candidate_pool",
		"profile_analysis", "rounds", "working_memory", "pending_decision", "report",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("expected %q to be omitted, got: %s", banned, out)
		}
	}
}

func TestAction_Validate(t *testing.T) {
	cases := []struct {
		a       Action
		wantErr bool
	}{
		{ActionAskNew, false},
		{ActionProbe, false},
		{ActionRefine, false},
		{ActionReflect, false},
		{ActionEnd, false},
		{Action(""), true},
		{Action("nope"), true},
	}
	for _, c := range cases {
		err := c.a.Validate()
		if (err != nil) != c.wantErr {
			t.Errorf("Validate(%q) err=%v wantErr=%v", c.a, err, c.wantErr)
		}
	}
}

func TestWorkingMemory_Budgets(t *testing.T) {
	m := NewWorkingMemory()
	if m.MaxRounds != 8 || m.MaxProbes != 4 || m.MaxReflections != 1 {
		t.Errorf("default budgets wrong: %+v", m)
	}
	if m.RemainingRounds() != 8 {
		t.Errorf("remaining should be 8, got %d", m.RemainingRounds())
	}
	m.RoundsAsked = 8
	if m.RemainingRounds() != 0 {
		t.Errorf("remaining should be 0 when used up")
	}
	m.RoundsAsked = 100 // 防御性溢出
	if m.RemainingRounds() != 0 {
		t.Errorf("remaining should clamp to 0 on overflow")
	}

	if !m.CanProbe() {
		t.Error("fresh memory should allow probe")
	}
	m.ProbesUsed = 4
	if m.CanProbe() {
		t.Error("should refuse probe when budget exhausted")
	}

	if !m.CanReflect() {
		t.Error("fresh memory should allow reflect")
	}
	m.ReflectionsUsed = 1
	if m.CanReflect() {
		t.Error("should refuse reflect when budget exhausted")
	}
}

func TestAnswerRound_FinalEvaluation(t *testing.T) {
	r := AnswerRound{
		Evaluation: &Evaluation{Score: 50},
	}
	if r.FinalEvaluation().Score != 50 {
		t.Error("should return original eval when no refine")
	}

	r.RefinedEval = &Evaluation{Score: 75}
	if r.FinalEvaluation().Score != 75 {
		t.Error("should prefer refined eval when present")
	}
}

func TestSession_CurrentRound(t *testing.T) {
	s := &Session{}
	if s.CurrentRound() != nil {
		t.Error("empty session: should return nil")
	}

	now := time.Now()
	s.Rounds = []AnswerRound{
		{RoundID: "r1", DecidedAt: now, CompletedAt: now}, // completed
	}
	if s.CurrentRound() != nil {
		t.Error("completed round: should return nil")
	}

	s.Rounds = append(s.Rounds, AnswerRound{RoundID: "r2", DecidedAt: now}) // in progress
	cur := s.CurrentRound()
	if cur == nil || cur.RoundID != "r2" {
		t.Errorf("should return r2, got %+v", cur)
	}
}

func TestSession_MigrateLegacyState_MovesNotesProtocols(t *testing.T) {
	s := &Session{
		WorkingMemory: &WorkingMemory{
			Notes: map[string]string{
				"reflect_topic":              "redis",
				"scored_rounds":              "3",
				"degraded_rounds":            "2",
				"eval_degraded":              "true",
				"eval_degraded_reason":       "llm timeout",
				"rag_degraded":               "true",
				"probe_eval_degraded_reason": "schema retry exhausted",
				"keep_me":                    "still metadata",
			},
		},
	}

	s.MigrateLegacyState()

	mem := s.WorkingMemory
	if mem.ReflectTopic != "redis" {
		t.Errorf("ReflectTopic = %q, want redis", mem.ReflectTopic)
	}
	if mem.ScoredRounds != 3 {
		t.Errorf("ScoredRounds = %d, want 3", mem.ScoredRounds)
	}
	if mem.DegradedRounds != 2 {
		t.Errorf("DegradedRounds = %d, want 2", mem.DegradedRounds)
	}
	if mem.DegradedReasons["eval"] != "llm timeout" {
		t.Errorf("eval degraded reason = %q", mem.DegradedReasons["eval"])
	}
	if mem.DegradedReasons["rag"] == "" {
		t.Errorf("rag degraded marker should migrate even without explicit reason")
	}
	if mem.DegradedReasons["probe_eval"] != "schema retry exhausted" {
		t.Errorf("probe_eval degraded reason = %q", mem.DegradedReasons["probe_eval"])
	}
	for _, key := range []string{
		"reflect_topic", "scored_rounds", "degraded_rounds",
		"eval_degraded", "eval_degraded_reason",
		"rag_degraded", "probe_eval_degraded_reason",
	} {
		if _, ok := mem.Notes[key]; ok {
			t.Errorf("legacy key %q should be removed, notes=%v", key, mem.Notes)
		}
	}
	if mem.Notes["keep_me"] != "still metadata" {
		t.Errorf("unrelated note should remain, notes=%v", mem.Notes)
	}
}

func TestSession_MigrateLegacyState_DoesNotOverwriteTypedFields(t *testing.T) {
	s := &Session{
		WorkingMemory: &WorkingMemory{
			ReflectTopic:    "typed",
			ScoredRounds:    9,
			DegradedRounds:  8,
			DegradedReasons: map[string]string{"eval": "typed reason"},
			Notes: map[string]string{
				"reflect_topic":        "legacy",
				"scored_rounds":        "1",
				"degraded_rounds":      "1",
				"eval_degraded_reason": "legacy reason",
			},
		},
	}

	s.MigrateLegacyState()

	mem := s.WorkingMemory
	if mem.ReflectTopic != "typed" {
		t.Errorf("ReflectTopic overwritten: %q", mem.ReflectTopic)
	}
	if mem.ScoredRounds != 9 || mem.DegradedRounds != 8 {
		t.Errorf("typed counters overwritten: scored=%d degraded=%d", mem.ScoredRounds, mem.DegradedRounds)
	}
	if mem.DegradedReasons["eval"] != "typed reason" {
		t.Errorf("typed degraded reason overwritten: %q", mem.DegradedReasons["eval"])
	}
}
