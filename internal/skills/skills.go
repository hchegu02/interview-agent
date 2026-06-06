package skills

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"interview-agent/internal/agentkit"
)

var ErrSkillNotFound = errors.New("skill not found")

type SkillInput struct {
	UserID  string            `json:"user_id,omitempty"`
	Message string            `json:"message"`
	Context map[string]string `json:"context,omitempty"`
}

type SkillResult struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Actions []Action `json:"actions,omitempty"`
}

type Action struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
}

type Skill interface {
	Name() string
	Run(ctx context.Context, input SkillInput) (SkillResult, error)
}

type Registry struct {
	items map[string]Skill
}

func NewRegistry() *Registry {
	return &Registry{items: map[string]Skill{}}
}

func NewDefaultRegistry() *Registry {
	return NewDefaultRegistryWithTools(nil)
}

func NewDefaultRegistryWithTools(tools *agentkit.ToolRegistry) *Registry {
	reg := NewRegistry()
	reg.Register(quizSkill{})
	reg.Register(explainSkill{})
	reg.Register(projectPolishSkill{tools: tools})
	return reg
}

func (r *Registry) Register(skill Skill) {
	if r == nil || skill == nil || skill.Name() == "" {
		return
	}
	r.items[skill.Name()] = skill
}

func (r *Registry) Run(ctx context.Context, name string, input SkillInput) (SkillResult, error) {
	if r == nil {
		return SkillResult{}, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
	}
	skill, ok := r.items[name]
	if !ok {
		return SkillResult{}, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
	}
	return skill.Run(ctx, input)
}

type quizSkill struct{}

func (quizSkill) Name() string { return "quiz" }

func (quizSkill) Run(ctx context.Context, input SkillInput) (SkillResult, error) {
	topic := topicFrom(input)
	return SkillResult{
		Title:   "专项测验",
		Content: fmt.Sprintf("请回答：%s 场景下，核心风险、排查步骤和工程取舍分别是什么？", topic),
		Actions: []Action{{Type: "start_drill", Label: "进入专项训练", Value: topic}},
	}, nil
}

type explainSkill struct{}

func (explainSkill) Name() string { return "explain" }

func (explainSkill) Run(ctx context.Context, input SkillInput) (SkillResult, error) {
	topic := topicFrom(input)
	return SkillResult{
		Title:   "知识讲解",
		Content: fmt.Sprintf("%s 可以按三层理解：概念是什么、为什么会出现、项目里如何排查和权衡。建议先讲定义，再补线上例子。", topic),
		Actions: []Action{{Type: "ask_followup", Label: "继续追问", Value: topic}},
	}, nil
}

type projectPolishSkill struct {
	tools *agentkit.ToolRegistry
}

func (projectPolishSkill) Name() string { return "project_polish" }

func (s projectPolishSkill) Run(ctx context.Context, input SkillInput) (SkillResult, error) {
	topic := topicFrom(input)
	if url := githubURLFrom(input); url != "" && s.tools != nil {
		if result, ok := s.tryAnalyzeProject(ctx, url); ok {
			return result, nil
		}
	}
	return SkillResult{
		Title:   "项目亮点提炼",
		Content: fmt.Sprintf("围绕 %s 描述时，按“背景问题、你的动作、技术取舍、量化结果、复盘改进”组织，避免只罗列技术名词。", topic),
		Actions: []Action{{Type: "rewrite_resume", Label: "整理成简历表述", Value: topic}},
	}, nil
}

func (s projectPolishSkill) tryAnalyzeProject(ctx context.Context, githubURL string) (SkillResult, bool) {
	result, err := s.tools.Call(ctx, agentkit.ToolCall{
		Name:         "github.project_analyze",
		Input:        map[string]any{"url": githubURL},
		InputSummary: "github repository url",
		Permission:   agentkit.PermissionReadOnly,
	})
	if err != nil {
		return SkillResult{}, false
	}
	out, ok := result.Output.(map[string]any)
	if !ok {
		return SkillResult{}, false
	}
	summary := strings.TrimSpace(fmt.Sprint(out["summary"]))
	if summary == "" {
		return SkillResult{}, false
	}
	highlights := stringSliceFromAny(out["highlights"])
	risks := stringSliceFromAny(out["risk_points"])
	var b strings.Builder
	b.WriteString("基于 mock GitHub 项目分析：")
	b.WriteString(summary)
	b.WriteString("。简历表述建议仍按“背景问题、你的动作、技术取舍、量化结果、复盘改进”组织。")
	if len(highlights) > 0 {
		b.WriteString(" 可突出：")
		b.WriteString(strings.Join(highlights, "；"))
		b.WriteString("。")
	}
	if len(risks) > 0 {
		b.WriteString(" 面试风险点：")
		b.WriteString(strings.Join(risks, "；"))
		b.WriteString("。")
	}
	return SkillResult{
		Title:   "项目亮点提炼",
		Content: b.String(),
		Actions: []Action{{Type: "rewrite_resume", Label: "整理成简历表述", Value: githubURL}},
	}, true
}

func topicFrom(input SkillInput) string {
	if input.Context != nil {
		if skill := strings.TrimSpace(input.Context["skill"]); skill != "" {
			return skill
		}
		if topic := strings.TrimSpace(input.Context["topic"]); topic != "" {
			return topic
		}
	}
	msg := strings.TrimSpace(input.Message)
	if msg == "" {
		return "当前主题"
	}
	return msg
}

var githubURLPattern = regexp.MustCompile(`https?://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+`)

func githubURLFrom(input SkillInput) string {
	if input.Context != nil {
		for _, key := range []string{"github_url", "github", "repo_url"} {
			if value := strings.TrimSpace(input.Context[key]); value != "" {
				if url := githubURLPattern.FindString(value); url != "" {
					return url
				}
			}
		}
	}
	return githubURLPattern.FindString(input.Message)
}

func stringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
