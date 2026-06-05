package skills

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	reg := NewRegistry()
	reg.Register(quizSkill{})
	reg.Register(explainSkill{})
	reg.Register(projectPolishSkill{})
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

type projectPolishSkill struct{}

func (projectPolishSkill) Name() string { return "project_polish" }

func (projectPolishSkill) Run(ctx context.Context, input SkillInput) (SkillResult, error) {
	topic := topicFrom(input)
	return SkillResult{
		Title:   "项目亮点提炼",
		Content: fmt.Sprintf("围绕 %s 描述时，按“背景问题、你的动作、技术取舍、量化结果、复盘改进”组织，避免只罗列技术名词。", topic),
		Actions: []Action{{Type: "rewrite_resume", Label: "整理成简历表述", Value: topic}},
	}, nil
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
