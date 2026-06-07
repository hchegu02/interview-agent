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

func buildReportSession() *domain.Session {
	mem := domain.NewWorkingMemory()
	mem.SkillCoverage["go"] = 0.82
	mem.SkillCoverage["redis"] = 0.46
	mem.ConfirmedSkills = []string{"go"}
	mem.WeakSkills = []string{"redis"}
	mem.DegradedReasons = map[string]string{
		"eval": "llm timeout",
		"rag":  "retrieve returned 0 results",
	}

	return &domain.Session{
		ID:            "sess-001",
		Status:        domain.StatusRunning,
		WorkingMemory: mem,
		CandidatePool: []domain.Question{
			{
				ID:            "redis-001",
				Content:       "Redis 缓存击穿怎么处理？",
				Tags:          []string{"redis", "cache"},
				Difficulty:    3,
				Source:        "rag-redis-001",
				SkillCategory: "redis",
			},
			{
				ID:            "go-001",
				Content:       "讲一下 Go GMP 调度模型。",
				Tags:          []string{"go"},
				Difficulty:    3,
				Source:        "rag-go-001",
				SkillCategory: "go",
			},
		},
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:            "q1",
					SkillCategory: "go",
				},
				Answer: "首先 GMP 包含 G/M/P；然后 P 负责本地队列，线上 1w QPS 场景下要关注调度延迟。",
				Evaluation: &domain.Evaluation{
					QuestionID: "q1",
					Score:      78,
					Strengths:  []string{"GMP 结构清楚"},
					Weaknesses: []string{"没讲 work stealing"},
					Suggestion: "补 work stealing",
				},
			},
			{
				RoundID: "r2",
				Question: domain.Question{
					ID:            "q2",
					SkillCategory: "redis",
				},
				Answer: "用 set nx ex 做互斥，但没有补充 lua 释放锁的校验。",
				Evaluation: &domain.Evaluation{
					QuestionID: "q2",
					Score:      72,
					Strengths:  []string{"知道 set nx ex"},
					Weaknesses: []string{"lua 原子性不完整"},
					Suggestion: "补 lua 锁释放脚本",
				},
				CriticResult: &domain.Critic{
					GroundedScore: 55,
					NeedRefine:    true,
					Issues:        []string{"原评估偏高"},
					Summary:       "需要重评",
				},
				RefinedEval: &domain.Evaluation{
					QuestionID: "q2",
					Score:      55,
					Strengths:  []string{"知道 set nx ex"},
					Weaknesses: []string{"lua 原子性不完整"},
					Suggestion: "补 lua 锁释放脚本",
				},
			},
			{
				RoundID: "r3",
				Question: domain.Question{
					ID:            "q3",
					SkillCategory: "mysql",
				},
				Evaluation: &domain.Evaluation{
					QuestionID: "q3",
					Score:      -1,
					Suggestion: "评估失败(降级)",
				},
			},
		},
	}
}

func TestReportNode_AggregatesSessionReport(t *testing.T) {
	sess := buildReportSession()
	node := NewReportNode()

	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("report node failed: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("expected report to be written")
	}
	if sess.Status != domain.StatusCompleted {
		t.Errorf("status = %q, want completed", sess.Status)
	}
	if sess.PendingDecision != nil {
		t.Errorf("pending decision should be cleared, got %+v", sess.PendingDecision)
	}
	if sess.Report.SessionID != "sess-001" {
		t.Errorf("session_id = %q, want sess-001", sess.Report.SessionID)
	}
	// (78 + 55) / 2 = 66.5 -> 67
	if sess.Report.OverallScore != 67 {
		t.Errorf("overall_score = %d, want 67", sess.Report.OverallScore)
	}
	if sess.Report.SkillBreakdown["go"] != 82 {
		t.Errorf("go breakdown = %d, want 82", sess.Report.SkillBreakdown["go"])
	}
	if sess.Report.SkillBreakdown["redis"] != 46 {
		t.Errorf("redis breakdown = %d, want 46", sess.Report.SkillBreakdown["redis"])
	}
	if !contains(sess.Report.Highlights, "已确认技能：go") {
		t.Errorf("highlights = %v", sess.Report.Highlights)
	}
	if !contains(sess.Report.Improvements, "待加强技能：redis") {
		t.Errorf("improvements = %v", sess.Report.Improvements)
	}
	if !contains(sess.Report.NextSteps, "优先补强 redis 相关题目与知识点") {
		t.Errorf("next_steps = %v", sess.Report.NextSteps)
	}
	if !contains(sess.Report.NextSteps, "评估过程中部分环节降级：eval、rag；建议复测这些环节以提高报告可信度") {
		t.Errorf("next_steps should mention degraded components, got %v", sess.Report.NextSteps)
	}
	if sess.Report.TranscriptAnalysis == nil {
		t.Fatal("transcript analysis should be written")
	}
	if sess.Report.TranscriptAnalysis.RoundsAnalyzed != 2 {
		t.Errorf("rounds_analyzed = %d, want 2", sess.Report.TranscriptAnalysis.RoundsAnalyzed)
	}
	if len(sess.Report.TranscriptAnalysis.Dimensions) < 5 {
		t.Errorf("expected transcript dimensions, got %+v", sess.Report.TranscriptAnalysis)
	}
	if len(sess.Report.DrillPlan) == 0 || sess.Report.DrillPlan[0].Skill != "redis" {
		t.Errorf("drill_plan = %+v, want redis first", sess.Report.DrillPlan)
	}
	if got := sess.Report.DrillPlan[0].RecommendedQuestionIDs; len(got) != 1 || got[0] != "redis-001" {
		t.Errorf("recommended question ids = %v, want [redis-001]", got)
	}
	if got := sess.Report.DrillPlan[0].RecommendedQuestions; len(got) == 0 || got[0] == "缓存击穿怎么处理？" {
		t.Errorf("recommended questions should include pool question with id, got %v", got)
	}
}

func TestReportNode_BuildsRoundReviewsWithOriginalAnswers(t *testing.T) {
	sess := buildReportSession()
	sess.Rounds = append(sess.Rounds,
		domain.AnswerRound{
			RoundID: "r4",
			Question: domain.Question{
				ID:             "redis-aof-rdb",
				Content:        "Redis AOF 和 RDB 怎么取舍？",
				SkillCategory:  "redis",
				ExpectedPoints: []string{"aof", "rdb"},
			},
			Answer: "AOF 更偏实时恢复，RDB 更适合快照备份。",
			Evaluation: &domain.Evaluation{
				QuestionID: "redis-aof-rdb",
				Score:      70,
				Strengths:  []string{"区分了恢复和快照"},
				Weaknesses: []string{"缺少 fsync 策略"},
				Suggestion: "补充 appendfsync always/everysec/no 的取舍。",
			},
			FollowUps: []domain.FollowUp{
				{
					Question: "AOF everysec 丢数据窗口是多少？",
					Answer:   "最多大约 1 秒。",
					Evaluation: &domain.Evaluation{
						QuestionID: "redis-aof-rdb-followup",
						Score:      80,
						Strengths:  []string{"回答了窗口"},
					},
				},
				{
					Question: "RDB 触发方式有哪些？",
				},
			},
		},
		domain.AnswerRound{
			RoundID: "r5",
			Question: domain.Question{
				ID:            "mq-001",
				Content:       "消息队列如何处理重复消费？",
				SkillCategory: "mq",
			},
			Evaluation: &domain.Evaluation{
				QuestionID: "mq-001",
				Score:      99,
			},
		},
	)

	err := NewReportNode()(context.Background(), sess)
	if err != nil {
		t.Fatalf("report node: %v", err)
	}
	if got := len(sess.Report.RoundReviews); got != answeredMainRounds(sess.Rounds) {
		t.Fatalf("round_reviews = %d, want answered main rounds", got)
	}
	review := findRoundReview(sess.Report.RoundReviews, "redis-aof-rdb")
	if review == nil {
		t.Fatal("missing redis review")
	}
	if review.Answer != "AOF 更偏实时恢复，RDB 更适合快照备份。" {
		t.Fatalf("answer = %q", review.Answer)
	}
	if review.Score == nil || *review.Score != 70 {
		t.Fatalf("score = %+v", review.Score)
	}
	if len(review.HitPoints) != 1 || review.HitPoints[0] != "区分了恢复和快照" {
		t.Fatalf("hit_points = %+v", review.HitPoints)
	}
	if len(review.MissedPoints) != 1 || review.MissedPoints[0] != "缺少 fsync 策略" {
		t.Fatalf("missed_points = %+v", review.MissedPoints)
	}
	if len(review.ExpectedPoints) != 2 || review.ExpectedPoints[0] != "aof" || review.ExpectedPoints[1] != "rdb" {
		t.Fatalf("expected_points = %+v", review.ExpectedPoints)
	}
	if len(review.FollowUps) != 1 || review.FollowUps[0].Answer != "最多大约 1 秒。" {
		t.Fatalf("followups = %+v", review.FollowUps)
	}
	if review.FollowUps[0].Score == nil || *review.FollowUps[0].Score != 80 {
		t.Fatalf("followup score = %+v", review.FollowUps[0].Score)
	}
	if missing := findRoundReview(sess.Report.RoundReviews, "mq-001"); missing != nil {
		t.Fatalf("unanswered round should not be reviewed: %+v", missing)
	}
	if want := domain.OverallScoreFromRoundReviews(sess.Report.RoundReviews); sess.Report.OverallScore != want {
		t.Fatalf("overall_score = %d, want %d from round reviews", sess.Report.OverallScore, want)
	}
}

func TestReportNode_EmitsSkillHookEvents(t *testing.T) {
	sess := buildReportSession()
	hook := agentkit.NewRecorderHook()
	node := NewReportNodeWithHook(hook)

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
	if events[0].Name != "report.generate" || events[1].Name != "report.generate" {
		t.Fatalf("event names = %+v", events)
	}
	if events[1].OutputSummary == "report=missing" || events[1].OutputSummary == "" {
		t.Fatalf("output summary = %q", events[1].OutputSummary)
	}
}

func TestReportPatchNode_ReturnsStatusAndNonMissingHookSummary(t *testing.T) {
	sess := buildReportSession()
	hook := agentkit.NewRecorderHook()
	node := NewReportPatchNodeWithHook(hook)

	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("patch node failed: %v", err)
	}
	if sess.Report != nil {
		t.Fatalf("patch node should not apply report directly, got %+v", sess.Report)
	}
	if patch.Report == nil {
		t.Fatal("patch should include report")
	}
	if patch.Status == nil || *patch.Status != domain.StatusCompleted {
		t.Fatalf("patch status = %+v, want completed", patch.Status)
	}
	events := hook.Events()
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[1].OutputSummary == "report=missing" || events[1].OutputSummary == "" {
		t.Fatalf("output summary = %q", events[1].OutputSummary)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if sess.Report == nil || sess.Status != domain.StatusCompleted {
		t.Fatalf("session after apply = %+v", sess)
	}
}

func TestReportNode_NoRounds_StillBuildsEmptyReport(t *testing.T) {
	sess := &domain.Session{
		ID:            "sess-empty",
		Status:        domain.StatusRunning,
		WorkingMemory: domain.NewWorkingMemory(),
	}
	node := NewReportNode()

	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("report node failed: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("expected report to be written")
	}
	if sess.Report.OverallScore != 0 {
		t.Errorf("overall_score = %d, want 0", sess.Report.OverallScore)
	}
	if len(sess.Report.NextSteps) == 0 {
		t.Error("expected a generic next step for empty report")
	}
	if sess.Report.TranscriptAnalysis == nil || len(sess.Report.TranscriptAnalysis.Patterns) == 0 {
		t.Errorf("expected empty transcript analysis, got %+v", sess.Report.TranscriptAnalysis)
	}
	if len(sess.Report.DrillPlan) == 0 {
		t.Errorf("expected generic drill plan, got %+v", sess.Report.DrillPlan)
	}
}

func TestReportNode_InvalidStatus_Permanent(t *testing.T) {
	sess := &domain.Session{
		Status: domain.SessionStatus("weird"),
	}
	node := NewReportNode()

	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}

func answeredMainRounds(rounds []domain.AnswerRound) int {
	n := 0
	for i := range rounds {
		if strings.TrimSpace(rounds[i].Answer) != "" {
			n++
		}
	}
	return n
}

func findRoundReview(reviews []domain.RoundReview, questionID string) *domain.RoundReview {
	for i := range reviews {
		if reviews[i].QuestionID == questionID {
			return &reviews[i]
		}
	}
	return nil
}
