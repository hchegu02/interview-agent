package skills

import (
	"context"
	"encoding/json"
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
	if len(result.ToolTrace) != 1 {
		t.Fatalf("tool trace len = %d, want 1", len(result.ToolTrace))
	}
	trace := result.ToolTrace[0]
	if trace.Name != "github.project_analyze" || trace.Permission != string(agentkit.PermissionReadOnly) || trace.Status != "success" {
		t.Fatalf("tool trace = %+v, want successful github trace", trace)
	}
	if trace.ElapsedMillis <= 0 || trace.Summary == "" {
		t.Fatalf("tool trace should include elapsed time and summary: %+v", trace)
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
	if len(result.ToolTrace) != 1 {
		t.Fatalf("tool trace len = %d, want 1", len(result.ToolTrace))
	}
	trace := result.ToolTrace[0]
	if trace.Name != "github.project_analyze" || trace.Status != "failed" || trace.ErrorClass != "tool_call_failed" {
		t.Fatalf("tool trace = %+v, want failed github trace", trace)
	}
	if trace.Permission != string(agentkit.PermissionReadOnly) || trace.ElapsedMillis <= 0 {
		t.Fatalf("tool trace should include permission and elapsed time: %+v", trace)
	}
}

func TestProjectPolishSkill_ToolFailureUsesMCPErrorCode(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := tools.Register(failingTool{
		spec: agentkit.ToolSpec{Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		err:  agentkit.MCPToolError{Code: "invalid_github_url", Message: "bad repo"},
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
	if len(result.ToolTrace) != 1 {
		t.Fatalf("tool trace len = %d, want 1", len(result.ToolTrace))
	}
	if result.ToolTrace[0].ErrorClass != "invalid_github_url" {
		t.Fatalf("error class = %q, want invalid_github_url", result.ToolTrace[0].ErrorClass)
	}
}

func TestProjectPolishSkill_CompactsSuccessTraceSummary(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := tools.Register(staticTool{
		spec: agentkit.ToolSpec{Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		result: agentkit.ToolResult{
			Output: map[string]any{
				"summary":     "stable project summary",
				"highlights":  []string{"trace support"},
				"risk_points": []string{"missing tests"},
			},
			Summary: "  " + strings.Repeat("summary ", 40),
		},
	}); err != nil {
		t.Fatalf("register static tool: %v", err)
	}
	reg := NewDefaultRegistryWithTools(tools)

	result, err := reg.Run(context.Background(), "project_polish", SkillInput{
		Message: "帮我润色 https://github.com/acme/interview-agent 这个项目",
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if len(result.ToolTrace) != 1 {
		t.Fatalf("tool trace len = %d, want 1", len(result.ToolTrace))
	}
	summary := result.ToolTrace[0].Summary
	if len(summary) > 160 {
		t.Fatalf("trace summary length = %d, want <= 160: %q", len(summary), summary)
	}
	if strings.Contains(summary, "  ") {
		t.Fatalf("trace summary should collapse whitespace: %q", summary)
	}
}

func TestSkillResult_DoesNotMarshalInternalToolTrace(t *testing.T) {
	body, err := json.Marshal(SkillResult{
		Title:     "项目亮点提炼",
		Content:   "content",
		ToolTrace: []ToolTrace{{Name: "github.project_analyze", Status: "success"}},
	})
	if err != nil {
		t.Fatalf("marshal skill result: %v", err)
	}
	if strings.Contains(string(body), "tool_trace") {
		t.Fatalf("skill result should not marshal internal tool trace: %s", string(body))
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

type staticTool struct {
	spec   agentkit.ToolSpec
	result agentkit.ToolResult
}

func (s staticTool) Spec() agentkit.ToolSpec { return s.spec }
func (s staticTool) Call(context.Context, agentkit.ToolCall) (agentkit.ToolResult, error) {
	return s.result, nil
}
