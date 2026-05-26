package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/retriever"
)

// resumeProjectShape 是简历项目 JSON 形状（与 domain.ResumeProject 同构）。
type resumeProjectShape struct {
	Name       string   `json:"name"`
	Role       string   `json:"role"`
	Highlights []string `json:"highlights"`
	Stack      []string `json:"stack"`
}

// resumeSchemaShape 是 parse_resume LLM 输出形状。
type resumeSchemaShape struct {
	Years      int                  `json:"years"`
	Skills     []string             `json:"skills"`
	Projects   []resumeProjectShape `json:"projects"`
	Highlights []string             `json:"highlights"`
}

// validateResumeSchema 校验 parse_resume LLM 输出。
//
// 注意 projects 字段允许空数组——有些简历没有项目章节（应届生/纯学术背景）。
// 但 skills 不允许空，年限不能 < 0。
func validateResumeSchema(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw, "years", "skills", "projects", "highlights"); err != nil {
		return err
	}
	var shape resumeSchemaShape
	if err := json.Unmarshal(raw, &shape); err != nil {
		return fmt.Errorf("parse resume shape: %w", err)
	}
	if shape.Years < 0 {
		return fmt.Errorf("years must be >= 0, got %d", shape.Years)
	}
	if len(shape.Skills) == 0 {
		return errors.New("skills must not be empty")
	}
	return nil
}

// NewParseResumeNode 构造 parse_resume 节点。
//
// 节点契约：
//   输入：sess.CandProfile.ResumeRawText
//   输出：sess.CandProfile.{Years, Skills, Projects, Highlights}
//
// 关于 WeakSkills：
//   这里 **不** 计算 WeakSkills——它需要 JobProfile.KeySkills 作输入，
//   是 gap_analyze 节点的职责。setup 阶段 parse_jd / parse_resume 并行执行，
//   parse_resume 跑的时候 JobProfile 可能还没填好，强行算就有读写竞态。
func NewParseResumeNode(model llm.ChatModel) graph.NodeFunc {
	return func(ctx context.Context, sess *domain.Session) error {
		if sess.CandProfile == nil || strings.TrimSpace(sess.CandProfile.ResumeRawText) == "" {
			return fmt.Errorf("parse_resume: resume raw text is required: %w", graph.ErrPermanent)
		}

		messages := []llm.Message{
			{Role: "system", Content: fmt.Sprintf(promptParseResume, sess.CandProfile.ResumeRawText)},
		}
		opts := llm.Options{
			Temperature: 0.1,
			MaxTokens:   1200, // 比 JD 大,简历项目数量可能多
		}

		resp, err := llm.CallWithSchema(ctx, model, messages, opts, validateResumeSchema, 1)
		if err != nil {
			return fmt.Errorf("parse_resume: %w", err)
		}

		var shape resumeSchemaShape
		if err := json.Unmarshal([]byte(resp.Content), &shape); err != nil {
			return fmt.Errorf("parse_resume: unmarshal: %w", err)
		}

		// 归一化技能标签
		shape.Skills = retriever.CanonicalizeTags(shape.Skills)
		projects := make([]domain.ResumeProject, 0, len(shape.Projects))
		for _, p := range shape.Projects {
			projects = append(projects, domain.ResumeProject{
				Name:       p.Name,
				Role:       p.Role,
				Highlights: p.Highlights,
				Stack:      retriever.CanonicalizeTags(p.Stack),
			})
		}

		sess.CandProfile.Years = shape.Years
		sess.CandProfile.Skills = shape.Skills
		sess.CandProfile.Projects = projects
		sess.CandProfile.Highlights = shape.Highlights
		return nil
	}
}
