package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"interview-agent/internal/agentkit"
)

func TestProjectPolishSkill_NoToolsKeepsGenericAdvice(t *testing.T) {
	reg := NewDefaultRegistry()

	result, err := reg.Run(context.Background(), "project_polish", SkillInput{
		Message: "帮我润色简历项目亮点",
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if result.Title != "项目亮点提炼" {
		t.Fatalf("title = %q", result.Title)
	}
	if !strings.Contains(result.Content, "背景问题、你的动作、技术取舍") {
		t.Fatalf("content should keep generic advice: %s", result.Content)
	}
	if strings.Contains(result.Content, "mock GitHub project analysis") {
		t.Fatalf("generic path should not include tool output: %s", result.Content)
	}
}

func TestProjectPolishSkill_NoGithubURLDoesNotCallTool(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := tools.Register(failingTool{
		spec: agentkit.ToolSpec{Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		err:  errors.New("tool should not be called"),
	}); err != nil {
		t.Fatalf("register failing tool: %v", err)
	}
	reg := NewDefaultRegistryWithTools(tools)

	result, err := reg.Run(context.Background(), "project_polish", SkillInput{
		Message: "帮我润色简历项目亮点",
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if !strings.Contains(result.Content, "背景问题、你的动作、技术取舍") {
		t.Fatalf("content should keep generic advice: %s", result.Content)
	}
}

func TestProjectPolishSkill_UsesGithubAnalyzeToolFromContext(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := agentkit.RegisterDefaultMCPTools(tools, agentkit.NewMockMCPClient()); err != nil {
		t.Fatalf("register tools: %v", err)
	}
	reg := NewDefaultRegistryWithTools(tools)

	result, err := reg.Run(context.Background(), "project_polish", SkillInput{
		Message: "帮我润色这个项目",
		Context: map[string]string{"github_url": "https://github.com/acme/interview-agent"},
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	for _, want := range []string{"interview-agent mock GitHub project analysis", "uses layered backend structure", "mock"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("content missing %q: %s", want, result.Content)
		}
	}
}

func TestProjectPolishSkill_UsesGithubAnalyzeToolFromMessage(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := agentkit.RegisterDefaultMCPTools(tools, agentkit.NewMockMCPClient()); err != nil {
		t.Fatalf("register tools: %v", err)
	}
	reg := NewDefaultRegistryWithTools(tools)

	result, err := reg.Run(context.Background(), "project_polish", SkillInput{
		Message: "帮我润色 https://github.com/acme/interview-agent 这个项目",
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if !strings.Contains(result.Content, "interview-agent mock GitHub project analysis") {
		t.Fatalf("content should include tool output: %s", result.Content)
	}
}

func TestProjectPolishSkill_ToolFailureFallsBack(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := tools.Register(failingTool{
		spec: agentkit.ToolSpec{Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		err:  errors.New("tool down"),
	}); err != nil {
		t.Fatalf("register failing tool: %v", err)
	}
	reg := NewDefaultRegistryWithTools(tools)

	result, err := reg.Run(context.Background(), "project_polish", SkillInput{
		Message: "帮我润色 https://github.com/acme/interview-agent 这个项目",
	})
	if err != nil {
		t.Fatalf("tool failure should degrade, got: %v", err)
	}
	if !strings.Contains(result.Content, "背景问题、你的动作、技术取舍") {
		t.Fatalf("fallback content missing generic advice: %s", result.Content)
	}
}

type failingTool struct {
	spec agentkit.ToolSpec
	err  error
}

func (f failingTool) Spec() agentkit.ToolSpec { return f.spec }
func (f failingTool) Call(context.Context, agentkit.ToolCall) (agentkit.ToolResult, error) {
	return agentkit.ToolResult{}, f.err
}
