package nodes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"interview-agent/internal/agentkit"
	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

// NewReportNode builds the final interview report from local session state.
//
// The report node deliberately avoids LLM calls: by the time the graph reaches
// report, evaluations and working memory already contain the decision signals.
func NewReportNode() graph.NodeFunc {
	return NewReportNodeWithHook(agentkit.NoopHook{})
}

func NewReportNodeWithHook(hook agentkit.Hook) graph.NodeFunc {
	patchNode := NewReportPatchNodeWithHook(hook)
	return func(ctx context.Context, sess *domain.Session) error {
		patch, err := patchNode(ctx, sess)
		if err != nil {
			return err
		}
		return applyNodePatch(sess, "report", patch)
	}
}

// NewReportPatchNodeWithHook 构造由 Graph runner 统一应用 StatePatch 的 report 节点。
func NewReportPatchNodeWithHook(hook agentkit.Hook) graph.PatchNodeFunc {
	if hook == nil {
		hook = agentkit.NoopHook{}
	}
	return func(ctx context.Context, sess *domain.Session) (patch domain.StatePatch, err error) {
		start := time.Now()
		var report *domain.Report
		_ = hook.HandleHook(ctx, agentkit.HookEvent{
			Type:         agentkit.HookBeforeSkill,
			SessionID:    sess.ID,
			Name:         "report.generate",
			InputSummary: "session rounds and working memory",
			Permission:   agentkit.PermissionWriteReport,
		})
		defer func() {
			summary := "report=missing"
			if report != nil {
				summary = fmt.Sprintf("overall_score=%d drill_plan=%d", report.OverallScore, len(report.DrillPlan))
			}
			ev := agentkit.HookEvent{
				Type:          agentkit.HookAfterSkill,
				SessionID:     sess.ID,
				Name:          "report.generate",
				InputSummary:  "session rounds and working memory",
				OutputSummary: summary,
				Latency:       time.Since(start),
				Permission:    agentkit.PermissionWriteReport,
			}
			if err != nil {
				ev.Error = err.Error()
			}
			_ = hook.HandleHook(ctx, ev)
		}()

		if err := ctx.Err(); err != nil {
			return domain.StatePatch{}, err
		}
		if sess.Status != "" {
			if err := sess.Status.Validate(); err != nil {
				return domain.StatePatch{}, graph.Permanent(fmt.Errorf("report: invalid session status: %w", err))
			}
		}
		workingMemory := sess.WorkingMemory
		if workingMemory == nil {
			workingMemory = domain.NewWorkingMemory()
		}
		reportSess := *sess
		reportSess.WorkingMemory = workingMemory
		reviews := roundReviews(reportSess.Rounds)

		report = &domain.Report{
			SessionID:          sess.ID,
			OverallScore:       domain.OverallScoreFromRoundReviews(reviews),
			SkillBreakdown:     skillBreakdown(workingMemory.SkillCoverage),
			TranscriptAnalysis: transcriptAnalysis(reportSess.Rounds),
			DrillPlan:          drillPlan(&reportSess),
			RoundReviews:       reviews,
			Highlights:         reportHighlights(&reportSess),
			Improvements:       reportImprovements(&reportSess),
			NextSteps:          reportNextSteps(&reportSess),
		}
		status := domain.StatusCompleted
		statePatch := domain.StatePatch{
			ClearPendingDecision: true,
			Report:               report,
			Status:               &status,
		}
		if sess.WorkingMemory == nil {
			statePatch.WorkingMemory = workingMemory
		}
		return statePatch, nil
	}
}

func roundReviews(rounds []domain.AnswerRound) []domain.RoundReview {
	reviews := make([]domain.RoundReview, 0, len(rounds))
	for i := range rounds {
		round := &rounds[i]
		if strings.TrimSpace(round.Answer) == "" {
			continue
		}
		eval := round.FinalEvaluation()
		review := domain.RoundReview{
			RoundID:        round.RoundID,
			Number:         len(reviews) + 1,
			Type:           "main",
			QuestionID:     round.Question.ID,
			Question:       round.Question.Content,
			Answer:         round.Answer,
			ExpectedPoints: append([]string(nil), round.Question.ExpectedPoints...),
			FollowUps:      followUpReviews(round.FollowUps),
		}
		if eval != nil {
			review.Score = intPtr(eval.Score)
			review.HitPoints = append([]string(nil), eval.Strengths...)
			review.MissedPoints = append([]string(nil), eval.Weaknesses...)
			review.Suggestion = eval.Suggestion
			review.CountsTowardOverall = eval.Score >= 0
		}
		reviews = append(reviews, review)
	}
	return reviews
}

func followUpReviews(followUps []domain.FollowUp) []domain.FollowUpReview {
	out := make([]domain.FollowUpReview, 0, len(followUps))
	for i := range followUps {
		follow := followUps[i]
		if strings.TrimSpace(follow.Answer) == "" {
			continue
		}
		review := domain.FollowUpReview{
			Question: follow.Question,
			Answer:   follow.Answer,
		}
		if follow.Evaluation != nil {
			review.Score = intPtr(follow.Evaluation.Score)
			review.HitPoints = append([]string(nil), follow.Evaluation.Strengths...)
			review.MissedPoints = append([]string(nil), follow.Evaluation.Weaknesses...)
			review.Suggestion = follow.Evaluation.Suggestion
		}
		out = append(out, review)
	}
	return out
}

func intPtr(v int) *int {
	return &v
}

func transcriptAnalysis(rounds []domain.AnswerRound) *domain.TranscriptAnalysis {
	var scored, scoreSum, answerChars int
	var evidenceRounds, structuredRounds, followUpCount, followUpScoreSum, followUpScored int
	var refinedCount, criticLow int

	for i := range rounds {
		round := &rounds[i]
		ev := round.FinalEvaluation()
		if ev == nil || ev.Score < 0 {
			continue
		}
		scored++
		scoreSum += ev.Score
		answerChars += runeLen(round.Answer)
		if hasConcreteEvidence(round.Answer) {
			evidenceRounds++
		}
		if hasAnswerStructure(round.Answer) {
			structuredRounds++
		}
		if round.RefinedEval != nil {
			refinedCount++
		}
		if round.CriticResult != nil && round.CriticResult.GroundedScore > 0 && round.CriticResult.GroundedScore < 60 {
			criticLow++
		}
		for j := range round.FollowUps {
			followUpCount++
			fev := round.FollowUps[j].Evaluation
			if fev == nil || fev.Score < 0 {
				continue
			}
			followUpScored++
			followUpScoreSum += fev.Score
		}
	}

	if scored == 0 {
		return &domain.TranscriptAnalysis{
			Dimensions: []domain.TranscriptDimension{
				{Name: "样本量", Score: 0, Advice: "先完成至少一轮有效答题，再生成稳定分析。"},
			},
			Patterns: []string{"暂无有效评分样本"},
		}
	}

	avgScore := roundedDiv(scoreSum, scored)
	avgChars := roundedDiv(answerChars, scored)
	evidenceScore := roundedDiv(evidenceRounds*100, scored)
	structureScore := roundedDiv(structuredRounds*100, scored)
	followScore := 0
	if followUpScored > 0 {
		followScore = roundedDiv(followUpScoreSum, followUpScored)
	} else if followUpCount == 0 {
		followScore = 60
	}
	reliabilityScore := clampScore(100 - refinedCount*18 - criticLow*14)

	return &domain.TranscriptAnalysis{
		RoundsAnalyzed:     scored,
		AverageAnswerChars: avgChars,
		Dimensions: []domain.TranscriptDimension{
			{
				Name:     "技术相关性",
				Score:    clampScore(avgScore),
				Evidence: []string{fmt.Sprintf("有效评分轮次 %d，平均分 %d", scored, avgScore)},
				Advice:   dimensionAdvice(avgScore, "回答要更贴题，先覆盖题目核心概念，再补项目经验。", "继续保持题目对齐度，下一步提高深度。"),
			},
			{
				Name:     "证据具体度",
				Score:    clampScore(evidenceScore),
				Evidence: []string{fmt.Sprintf("%d/%d 轮回答包含数字、指标或明确技术证据", evidenceRounds, scored)},
				Advice:   dimensionAdvice(evidenceScore, "每个项目回答补充指标、边界条件或故障结果。", "证据表达不错，继续补充取舍过程。"),
			},
			{
				Name:     "表达结构",
				Score:    clampScore(structureScore),
				Evidence: []string{fmt.Sprintf("%d/%d 轮回答有分点、步骤或因果结构", structuredRounds, scored)},
				Advice:   dimensionAdvice(structureScore, "用“背景-方案-结果-反思”组织回答。", "结构已经可读，继续压缩铺垫。"),
			},
			{
				Name:     "追问承接",
				Score:    clampScore(followScore),
				Evidence: []string{fmt.Sprintf("追问 %d 次，有效评分 %d 次", followUpCount, followUpScored)},
				Advice:   dimensionAdvice(followScore, "追问时直接回答追问点，不要回到主题题泛讲。", "追问承接稳定，可以增加边界场景深挖。"),
			},
			{
				Name:     "评估稳定性",
				Score:    reliabilityScore,
				Evidence: []string{fmt.Sprintf("重评 %d 次，低 grounded critic %d 次", refinedCount, criticLow)},
				Advice:   dimensionAdvice(reliabilityScore, "回答和评分锚点偏散，后续训练要强化要点覆盖。", "评估稳定，说明回答与参考要点基本对齐。"),
			},
		},
		Patterns: transcriptPatterns(avgChars, evidenceRounds, structuredRounds, scored),
	}
}

func drillPlan(sess *domain.Session) []domain.DrillPlanItem {
	mem := sess.WorkingMemory
	var skills []string
	skills = append(skills, sortedStrings(mem.WeakSkills)...)
	if sess.ProfileAnalysis != nil {
		skills = append(skills, sortedStrings(sess.ProfileAnalysis.MissingRequirements)...)
	}
	skills = uniqueStrings(skills)
	if len(skills) == 0 {
		skills = lowCoverageSkills(mem.SkillCoverage)
	}
	if len(skills) == 0 {
		skills = []string{"综合表达"}
	}

	var out []domain.DrillPlanItem
	for _, skill := range skills {
		reason := fmt.Sprintf("%s 在本轮表现中仍需加强", skill)
		if containsString(mem.WeakSkills, skill) {
			reason = fmt.Sprintf("%s 被工作记忆标记为薄弱技能", skill)
		}
		if sess.ProfileAnalysis != nil && containsString(sess.ProfileAnalysis.MissingRequirements, skill) {
			reason = fmt.Sprintf("%s 是 JD 要求但简历证据不足", skill)
		}
		out = append(out, domain.DrillPlanItem{
			PracticeOrder:          len(out) + 1,
			Skill:                  skill,
			Reason:                 reason,
			TargetScore:            75,
			RecommendedQuestionIDs: recommendedQuestionIDs(skill, sess.CandidatePool),
			RecommendedQuestions:   recommendedDrills(skill, sess.CandidatePool),
		})
		if len(out) == 5 {
			break
		}
	}
	return out
}

func skillBreakdown(coverage map[string]float64) map[string]int {
	out := make(map[string]int, len(coverage))
	for skill, cov := range coverage {
		if skill == "" {
			continue
		}
		score := int(cov*100 + 0.5)
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		out[skill] = score
	}
	return out
}

func reportHighlights(sess *domain.Session) []string {
	var out []string
	mem := sess.WorkingMemory
	if len(mem.ConfirmedSkills) > 0 {
		out = append(out, "已确认技能："+strings.Join(sortedStrings(mem.ConfirmedSkills), "、"))
	}
	for _, s := range collectEvalText(sess.Rounds, func(ev *domain.Evaluation) []string {
		return ev.Strengths
	}) {
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, "暂无明确亮点，建议补充更多答题样本")
	}
	return uniqueStrings(out)
}

func reportImprovements(sess *domain.Session) []string {
	var out []string
	mem := sess.WorkingMemory
	if len(mem.WeakSkills) > 0 {
		out = append(out, "待加强技能："+strings.Join(sortedStrings(mem.WeakSkills), "、"))
	}
	for _, s := range collectEvalText(sess.Rounds, func(ev *domain.Evaluation) []string {
		return ev.Weaknesses
	}) {
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, "暂无明确短板，建议继续用更高难度题验证深度")
	}
	return uniqueStrings(out)
}

func reportNextSteps(sess *domain.Session) []string {
	mem := sess.WorkingMemory
	var out []string
	for _, skill := range sortedStrings(mem.WeakSkills) {
		out = append(out, fmt.Sprintf("优先补强 %s 相关题目与知识点", skill))
	}
	for _, s := range collectEvalText(sess.Rounds, func(ev *domain.Evaluation) []string {
		if strings.TrimSpace(ev.Suggestion) == "" {
			return nil
		}
		return []string{ev.Suggestion}
	}) {
		out = append(out, s)
	}
	if len(mem.DegradedReasons) > 0 {
		out = append(out, fmt.Sprintf("评估过程中部分环节降级：%s；建议复测这些环节以提高报告可信度",
			strings.Join(sortedMapKeys(mem.DegradedReasons), "、")))
	}
	if len(out) == 0 {
		out = append(out, "继续完成更多模拟面试轮次，积累稳定评估样本")
	}
	return uniqueStrings(out)
}

func collectEvalText(rounds []domain.AnswerRound, pick func(*domain.Evaluation) []string) []string {
	var out []string
	for i := range rounds {
		ev := rounds[i].FinalEvaluation()
		if ev == nil || ev.Score < 0 {
			continue
		}
		for _, s := range pick(ev) {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func lowCoverageSkills(coverage map[string]float64) []string {
	var out []string
	for skill, cov := range coverage {
		if strings.TrimSpace(skill) != "" && cov < 0.7 {
			out = append(out, skill)
		}
	}
	sort.Strings(out)
	return out
}

func recommendedQuestionIDs(skill string, pool []domain.Question) []string {
	matched := questionsForSkill(skill, pool)
	out := make([]string, 0, len(matched))
	for _, q := range matched {
		out = append(out, q.ID)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func recommendedDrills(skill string, pool []domain.Question) []string {
	if matched := questionsForSkill(skill, pool); len(matched) > 0 {
		out := make([]string, 0, len(matched))
		for _, q := range matched {
			out = append(out, fmt.Sprintf("%s：%s", q.ID, q.Content))
			if len(out) == 3 {
				break
			}
		}
		return out
	}
	switch strings.ToLower(strings.TrimSpace(skill)) {
	case "go", "golang":
		return []string{
			"用 3 分钟讲清 Go 并发模型，并补一个线上排障案例。",
			"回答 channel / context / GMP 任一题时必须包含边界条件。",
		}
	case "redis":
		return []string{
			"练习缓存击穿、穿透、雪崩的治理方案对比。",
			"用一个项目案例说明 Redis 一致性和降级策略。",
		}
	case "mysql", "postgres", "postgresql", "pg":
		return []string{
			"解释一次慢查询排查链路：索引、执行计划、锁和数据分布。",
			"准备一个事务隔离或 MVCC 的项目级案例。",
		}
	case "kafka", "mq":
		return []string{
			"练习消息重复、乱序、积压和幂等处理。",
			"用项目案例说明消费失败后的恢复策略。",
		}
	default:
		return []string{
			fmt.Sprintf("围绕 %s 准备 3 道基础题和 2 道项目追问。", skill),
			"每次回答都按“概念-方案-边界-结果”组织。",
		}
	}
}

func questionsForSkill(skill string, pool []domain.Question) []domain.Question {
	skill = strings.ToLower(strings.TrimSpace(skill))
	if skill == "" {
		return nil
	}
	var out []domain.Question
	for _, q := range pool {
		if questionMatchesSkill(q, skill) {
			out = append(out, q)
		}
	}
	return out
}

func questionMatchesSkill(q domain.Question, skill string) bool {
	if strings.EqualFold(q.SkillCategory, skill) {
		return true
	}
	if strings.Contains(strings.ToLower(q.Content), skill) {
		return true
	}
	for _, tag := range q.Tags {
		tag = strings.ToLower(tag)
		if tag == skill || strings.Contains(tag, skill) {
			return true
		}
	}
	return false
}

func transcriptPatterns(avgChars, evidenceRounds, structuredRounds, scored int) []string {
	var out []string
	if avgChars < 80 {
		out = append(out, "回答偏短，容易缺少技术展开和取舍说明")
	}
	if evidenceRounds < scored {
		out = append(out, "部分回答缺少数字、指标或可验证项目证据")
	}
	if structuredRounds < scored {
		out = append(out, "部分回答结构松散，建议固定使用分点表达")
	}
	if len(out) == 0 {
		out = append(out, "回答整体有结构且有证据，后续重点提高技术深度")
	}
	return out
}

func hasConcreteEvidence(answer string) bool {
	answer = strings.ToLower(answer)
	if strings.Contains(answer, "qps") || strings.Contains(answer, "p99") || strings.Contains(answer, "ms") || strings.Contains(answer, "%") {
		return true
	}
	for _, r := range answer {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func hasAnswerStructure(answer string) bool {
	answer = strings.TrimSpace(answer)
	if strings.Contains(answer, "\n") || strings.Contains(answer, "；") || strings.Contains(answer, ";") {
		return true
	}
	markers := []string{"首先", "其次", "然后", "最后", "因为", "所以", "第一", "第二", "1.", "2.", "- "}
	for _, marker := range markers {
		if strings.Contains(answer, marker) {
			return true
		}
	}
	return false
}

func dimensionAdvice(score int, low, high string) string {
	if score < 70 {
		return low
	}
	return high
}

func roundedDiv(sum, n int) int {
	if n <= 0 {
		return 0
	}
	return (sum + n/2) / n
}

func runeLen(s string) int {
	return len([]rune(strings.TrimSpace(s)))
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func sortedMapKeys(in map[string]string) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
