package nodes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/retriever"
)

// NewAnalyzeProfileNode 生成 JD/简历匹配分析。
//
// 这个节点故意不调 LLM：它只解释前面已经抽出来的结构化数据。
// 好处是稳定、可单测、mock/real 模式表现一致。
func NewAnalyzeProfileNode() graph.NodeFunc {
	return func(ctx context.Context, sess *domain.Session) error {
		if sess.JobProfile == nil {
			return fmt.Errorf("analyze_profile: job_profile required: %w", graph.ErrPermanent)
		}
		if sess.CandProfile == nil {
			return fmt.Errorf("analyze_profile: candidate_profile required: %w", graph.ErrPermanent)
		}
		if sess.GapReport == nil {
			return fmt.Errorf("analyze_profile: gap_report required: %w", graph.ErrPermanent)
		}
		sess.ProfileAnalysis = buildProfileAnalysis(sess.JobProfile, sess.CandProfile, sess.GapReport)
		return nil
	}
}

func buildProfileAnalysis(job *domain.JobProfile, cand *domain.CandidateProfile, gap *domain.GapReport) *domain.ProfileAnalysis {
	matched := retriever.CanonicalizeTags(gap.MatchedSkills)
	missing := retriever.CanonicalizeTags(gap.MissingSkills)
	sort.Strings(matched)
	sort.Strings(missing)

	yearsGap := cand.Years - job.YearsRequired
	score := profileMatchScore(gap.OverlapScore, yearsGap, projectEvidenceCount(cand.Projects, matched))
	out := &domain.ProfileAnalysis{
		MatchScore:          score,
		Summary:             profileSummary(score, gap.Strategy),
		YearsGap:            yearsGap,
		MatchedRequirements: matched,
		MissingRequirements: missing,
		Strengths:           profileStrengths(job, cand, matched),
		RiskPoints:          profileRisks(job, cand, missing, yearsGap),
		ResumeSuggestions:   resumeSuggestions(job, cand, missing, yearsGap),
		QuestionFocus:       questionFocus(job, matched, missing),
		ProjectProbePlan:    projectProbePlan(cand.Projects, matched, job.KeySkills),
	}
	return out
}

func profileMatchScore(overlap float64, yearsGap, evidenceProjects int) int {
	skillScore := int(overlap*70 + 0.5)
	yearsScore := 20
	if yearsGap < 0 {
		yearsScore = 5
		if yearsGap == -1 {
			yearsScore = 12
		}
	}
	projectScore := evidenceProjects * 5
	if projectScore > 10 {
		projectScore = 10
	}
	return clampInt(skillScore+yearsScore+projectScore, 0, 100)
}

func profileSummary(score int, strategy domain.GapStrategy) string {
	switch {
	case score >= 80:
		return "岗位匹配度高，面试重点应放在简历真实性和深度验证。"
	case score >= 55:
		return "岗位匹配度中等，建议围绕重叠技能和项目细节继续探索。"
	default:
		if strategy == domain.GapStrategyCoverGap {
			return "岗位匹配度偏低，面试应优先覆盖缺失技能并定位基础水平。"
		}
		return "岗位匹配度偏低，简历需要补充与岗位要求直接相关的证据。"
	}
}

func profileStrengths(job *domain.JobProfile, cand *domain.CandidateProfile, matched []string) []string {
	var out []string
	if len(matched) > 0 {
		out = append(out, fmt.Sprintf("技能命中：%s", strings.Join(matched, "、")))
	}
	if job.YearsRequired == 0 {
		out = append(out, fmt.Sprintf("简历年限为 %d 年，JD 未设置硬性年限。", cand.Years))
	} else if cand.Years >= job.YearsRequired {
		out = append(out, fmt.Sprintf("年限满足要求：%d/%d 年。", cand.Years, job.YearsRequired))
	}
	for _, p := range cand.Projects {
		if overlap := intersectStrings(retriever.CanonicalizeTags(p.Stack), matched); len(overlap) > 0 {
			out = append(out, fmt.Sprintf("项目「%s」可支撑 %s 追问。", projectName(p), strings.Join(overlap, "、")))
		}
	}
	return compactLimit(out, 5)
}

func profileRisks(job *domain.JobProfile, cand *domain.CandidateProfile, missing []string, yearsGap int) []string {
	var out []string
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf("JD 技能未覆盖：%s。", strings.Join(missing, "、")))
	}
	if mustMissing := intersectStrings(retriever.CanonicalizeTags(job.MustHave), missing); len(mustMissing) > 0 {
		out = append(out, fmt.Sprintf("硬性要求缺少简历证据：%s。", strings.Join(mustMissing, "、")))
	}
	if job.YearsRequired > 0 && yearsGap < 0 {
		out = append(out, fmt.Sprintf("年限低于 JD 要求：%d/%d 年。", cand.Years, job.YearsRequired))
	}
	if len(cand.Projects) == 0 {
		out = append(out, "简历缺少可追问的项目经历。")
	}
	if len(cand.Highlights) == 0 {
		out = append(out, "简历缺少可验证的量化亮点。")
	}
	for _, p := range cand.Projects {
		if strings.TrimSpace(p.Role) == "" {
			out = append(out, fmt.Sprintf("项目「%s」没有说明个人角色。", projectName(p)))
			break
		}
	}
	return compactLimit(out, 5)
}

func resumeSuggestions(job *domain.JobProfile, cand *domain.CandidateProfile, missing []string, yearsGap int) []string {
	var out []string
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf("补充 %s 的项目证据、故障案例或学习实践记录。", strings.Join(missing, "、")))
	}
	if job.YearsRequired > 0 && yearsGap < 0 {
		out = append(out, "用更具体的项目复杂度和职责边界弥补年限不足。")
	}
	if len(cand.Highlights) == 0 || hasVagueHighlights(cand.Highlights) {
		out = append(out, "把项目亮点改成“动作 + 技术方案 + 指标/结果”的句式。")
	}
	if len(cand.Projects) == 0 {
		out = append(out, "至少补 1-2 个能被技术追问的项目，写清角色、难点和取舍。")
	}
	for _, p := range cand.Projects {
		if strings.TrimSpace(p.Role) == "" {
			out = append(out, "为每个项目补充个人角色，避免看起来像团队成果堆砌。")
			break
		}
	}
	if len(out) == 0 {
		out = append(out, "保持现有技能主线，补充更多可量化指标提升可信度。")
	}
	return compactLimit(out, 5)
}

func questionFocus(job *domain.JobProfile, matched, missing []string) []string {
	if len(missing) > 0 {
		return compactLimit(missing, 6)
	}
	if len(matched) > 0 {
		return compactLimit(matched, 6)
	}
	return compactLimit(retriever.CanonicalizeTags(job.KeySkills), 6)
}

func projectProbePlan(projects []domain.ResumeProject, matched, jobSkills []string) []domain.ProjectProbePlan {
	var out []domain.ProjectProbePlan
	targets := matched
	if len(targets) == 0 {
		targets = retriever.CanonicalizeTags(jobSkills)
	}
	for _, p := range projects {
		focus := firstNonEmpty(intersectStrings(retriever.CanonicalizeTags(p.Stack), targets))
		if focus == "" {
			focus = firstNonEmpty(retriever.CanonicalizeTags(p.Stack))
		}
		if focus == "" {
			focus = "项目实现细节"
		}
		evidence := firstNonEmpty(p.Highlights)
		out = append(out, domain.ProjectProbePlan{
			ProjectName:       projectName(p),
			Focus:             focus,
			Evidence:          evidence,
			SuggestedQuestion: fmt.Sprintf("你在「%s」中如何落地 %s，遇到的关键问题是什么？", projectName(p), focus),
		})
		if len(out) == 3 {
			break
		}
	}
	return out
}

func projectEvidenceCount(projects []domain.ResumeProject, matched []string) int {
	count := 0
	for _, p := range projects {
		if len(intersectStrings(retriever.CanonicalizeTags(p.Stack), matched)) > 0 || len(p.Highlights) > 0 {
			count++
		}
	}
	return count
}

func intersectStrings(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, s := range b {
		if s != "" {
			set[s] = struct{}{}
		}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, s := range a {
		if _, ok := set[s]; !ok {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func compactLimit(items []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func hasVagueHighlights(items []string) bool {
	for _, item := range items {
		if strings.Contains(item, "优秀") || strings.Contains(item, "熟悉") || strings.Contains(item, "负责") {
			return true
		}
	}
	return false
}

func projectName(p domain.ResumeProject) string {
	if strings.TrimSpace(p.Name) == "" {
		return "未命名项目"
	}
	return strings.TrimSpace(p.Name)
}

func firstNonEmpty(items []string) string {
	for _, item := range items {
		if s := strings.TrimSpace(item); s != "" {
			return s
		}
	}
	return ""
}

func clampInt(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
