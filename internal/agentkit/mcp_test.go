package agentkit

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockMCPClient struct {
	result MCPResult
	err    error
}

func (m mockMCPClient) CallTool(context.Context, MCPCall) (MCPResult, error) {
	return m.result, m.err
}

func TestMCPToolAdapterCallsClient(t *testing.T) {
	tool := NewMCPToolAdapter(ToolSpec{Name: "questionbank.search", Permission: PermissionReadOnly}, mockMCPClient{
		result: MCPResult{Output: map[string]any{"items": 2}, Summary: "2 items"},
	})
	result, err := tool.Call(context.Background(), ToolCall{Name: "questionbank.search", Input: map[string]string{"q": "redis"}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.Summary != "2 items" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCPToolAdapterReturnsClientError(t *testing.T) {
	want := errors.New("mcp failed")
	tool := NewMCPToolAdapter(ToolSpec{Name: "metrics.query", Permission: PermissionReadOnly}, mockMCPClient{err: want})
	if _, err := tool.Call(context.Background(), ToolCall{Name: "metrics.query"}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestRegisterDefaultMCPToolsRegistersMockTools(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	if err := RegisterDefaultMCPTools(reg, NewMockMCPClient()); err != nil {
		t.Fatalf("register default tools: %v", err)
	}
	specs := reg.List()
	if len(specs) != 2 {
		t.Fatalf("specs = %+v", specs)
	}
	if specs[0].Name != "github.project_analyze" || specs[1].Name != "web.fetch" {
		t.Fatalf("unexpected specs = %+v", specs)
	}
}

func TestRegisterDefaultMCPToolsUsesMockClientWhenNil(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	if err := RegisterDefaultMCPTools(reg, nil); err != nil {
		t.Fatalf("register default tools: %v", err)
	}
	result, err := reg.Call(context.Background(), ToolCall{
		Name:       "web.fetch",
		Permission: PermissionReadOnly,
		Input:      map[string]any{"url": "https://example.com/job"},
	})
	if err != nil {
		t.Fatalf("call default mock tool: %v", err)
	}
	if result.Summary == "" {
		t.Fatalf("expected mock summary, got %+v", result)
	}
}

func TestMockMCPClientGithubProjectAnalyze(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	if err := RegisterDefaultMCPTools(reg, NewMockMCPClient()); err != nil {
		t.Fatalf("register default tools: %v", err)
	}
	result, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Input:      map[string]any{"url": "https://github.com/acme/interview-agent"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	out, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T", result.Output)
	}
	if out["summary"] == "" || out["primary_language"] == "" {
		t.Fatalf("missing summary/language: %+v", out)
	}
	if highlights, ok := out["highlights"].([]string); !ok || len(highlights) == 0 {
		t.Fatalf("missing highlights: %+v", out)
	}
	if risks, ok := out["risk_points"].([]string); !ok || len(risks) == 0 {
		t.Fatalf("missing risk points: %+v", out)
	}
	if !strings.Contains(result.Summary, "interview-agent") {
		t.Fatalf("summary = %q", result.Summary)
	}
}

func TestMockMCPClientWebFetch(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	if err := RegisterDefaultMCPTools(reg, NewMockMCPClient()); err != nil {
		t.Fatalf("register default tools: %v", err)
	}
	result, err := reg.Call(context.Background(), ToolCall{
		Name:       "web.fetch",
		Permission: PermissionReadOnly,
		Input:      map[string]any{"url": "https://example.com/job"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	out, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T", result.Output)
	}
	if out["url"] != "https://example.com/job" || out["title"] == "" || out["content_summary"] == "" {
		t.Fatalf("bad web output: %+v", out)
	}
}

func TestDefaultMCPToolPermissionDeniedDoesNotCallClient(t *testing.T) {
	client := &countingMCPClient{result: MCPResult{Output: map[string]any{}, Summary: "ok"}}
	reg := NewToolRegistry(NoopHook{})
	if err := RegisterDefaultMCPTools(reg, client); err != nil {
		t.Fatalf("register default tools: %v", err)
	}
	_, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Permission: PermissionWriteSession,
		Input:      map[string]any{"url": "https://github.com/acme/interview-agent"},
	})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
}

func TestMCPToolAdapterRejectsNilClient(t *testing.T) {
	tool := NewMCPToolAdapter(ToolSpec{Name: "web.fetch", Permission: PermissionReadOnly}, nil)
	_, err := tool.Call(context.Background(), ToolCall{Name: "web.fetch"})
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestMockMCPClientUnknownTool(t *testing.T) {
	client := NewMockMCPClient()
	_, err := client.CallTool(context.Background(), MCPCall{Name: "unknown.tool"})
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}
}

type countingMCPClient struct {
	result MCPResult
	calls  int
}

func (c *countingMCPClient) CallTool(context.Context, MCPCall) (MCPResult, error) {
	c.calls++
	return c.result, nil
}
