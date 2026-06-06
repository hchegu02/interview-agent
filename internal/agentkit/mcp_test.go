package agentkit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestRegisterGitHubProjectToolRequiresExplicitClientConfig(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	if err := RegisterGitHubProjectTool(reg, GitHubProjectClient{}); err != nil {
		t.Fatalf("register github project tool: %v", err)
	}

	_, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Input:      map[string]any{"url": "https://github.com/acme/demo"},
		Permission: PermissionReadOnly,
	})
	var toolErr MCPToolError
	if !errors.As(err, &toolErr) || toolErr.Code != "config_missing" {
		t.Fatalf("err = %v, want config_missing MCPToolError", err)
	}
}

func TestRegisterGitHubProjectToolRejectsNilRegistry(t *testing.T) {
	err := RegisterGitHubProjectTool(nil, GitHubProjectClient{})
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
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

func TestGitHubProjectClientSuccessUsesRegistryBoundary(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"full_name":"acme/interview-agent",
			"description":"Mock repo for tests",
			"language":"Go",
			"html_url":"https://github.com/acme/interview-agent"
		}`))
	}))
	defer server.Close()

	hook := NewRecorderHook()
	reg := NewToolRegistry(hook)
	err := reg.Register(NewMCPToolAdapter(ToolSpec{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Timeout:    time.Second,
	}, GitHubProjectClient{HTTPClient: server.Client(), BaseURL: server.URL}))
	if err != nil {
		t.Fatalf("register tool: %v", err)
	}

	result, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Input:      map[string]any{"url": "https://github.com/acme/interview-agent"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if gotPath != "/repos/acme/interview-agent" {
		t.Fatalf("path = %q", gotPath)
	}
	if result.Summary != "github analysis for acme/interview-agent" {
		t.Fatalf("summary = %q", result.Summary)
	}
	out, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T", result.Output)
	}
	if out["project"] != "acme/interview-agent" || out["primary_language"] != "Go" {
		t.Fatalf("output = %+v", out)
	}
	events := hook.Events()
	if len(events) != 2 || events[0].Type != HookBeforeTool || events[1].Type != HookAfterTool {
		t.Fatalf("hook events = %+v, want before/after", events)
	}
	if events[1].Error != "" {
		t.Fatalf("after error = %q, want empty", events[1].Error)
	}
}

func TestGitHubProjectClientRejectsInvalidBaseURL(t *testing.T) {
	client := GitHubProjectClient{BaseURL: ":// bad"}
	_, err := client.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "config_invalid") {
		t.Fatalf("err = %v, want config_invalid", err)
	}
}

func TestGitHubProjectClientRejectsMissingExplicitConfig(t *testing.T) {
	client := GitHubProjectClient{}
	_, err := client.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "config_missing") {
		t.Fatalf("err = %v, want config_missing", err)
	}
}

func TestGitHubProjectClientRejectsInvalidRepoURL(t *testing.T) {
	client := GitHubProjectClient{HTTPClient: &http.Client{}, BaseURL: "https://api.github.test"}
	_, err := client.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://example.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "invalid_github_url") {
		t.Fatalf("err = %v, want invalid_github_url", err)
	}
}

func TestGitHubProjectClientRejectsUserinfoURL(t *testing.T) {
	client := GitHubProjectClient{HTTPClient: &http.Client{}, BaseURL: "https://api.github.test"}
	_, err := client.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://token@github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "invalid_github_url") {
		t.Fatalf("err = %v, want invalid_github_url", err)
	}
	if err != nil && strings.Contains(err.Error(), "token") {
		t.Fatalf("err leaked userinfo token: %v", err)
	}
}

func TestGitHubProjectClientPermissionDeniedDoesNotCallHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	reg := NewToolRegistry(NoopHook{})
	if err := reg.Register(NewMCPToolAdapter(ToolSpec{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Timeout:    time.Second,
	}, GitHubProjectClient{HTTPClient: server.Client(), BaseURL: server.URL})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	_, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Permission: PermissionWriteSession,
		Input:      map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
	if calls != 0 {
		t.Fatalf("http calls = %d, want 0", calls)
	}
}

func TestGitHubProjectClientReturnsCanonicalURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"full_name":"acme/repo","description":"repo","language":"Go"}`))
	}))
	defer server.Close()

	result, err := GitHubProjectClient{HTTPClient: server.Client(), BaseURL: server.URL}.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://github.com/acme/repo.git?token=secret"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	out := result.Output.(map[string]any)
	if out["url"] != "https://github.com/acme/repo" || out["html_url"] != "https://github.com/acme/repo" {
		t.Fatalf("output url fields = %+v, want canonical non-sensitive URL", out)
	}
}

func TestGitHubProjectClientHTTPStatusReturnsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := GitHubProjectClient{HTTPClient: server.Client(), BaseURL: server.URL}.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "http_status") {
		t.Fatalf("err = %v, want http_status", err)
	}
}

func TestGitHubProjectClientInvalidJSONReturnsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := GitHubProjectClient{HTTPClient: server.Client(), BaseURL: server.URL}.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "response_invalid") {
		t.Fatalf("err = %v, want response_invalid", err)
	}
}

func TestGitHubProjectClientClientErrorReturnsStableError(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("client transport failed")
	})}
	_, err := GitHubProjectClient{HTTPClient: client, BaseURL: "https://api.github.test"}.CallTool(context.Background(), MCPCall{
		Name:  "github.project_analyze",
		Input: map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "request_failed") {
		t.Fatalf("err = %v, want request_failed", err)
	}
}

func TestGitHubProjectClientErrorRecordsPairedHooks(t *testing.T) {
	hook := NewRecorderHook()
	reg := NewToolRegistry(hook)
	if err := reg.Register(NewMCPToolAdapter(ToolSpec{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Timeout:    time.Second,
	}, GitHubProjectClient{HTTPClient: &http.Client{}, BaseURL: "https://api.github.test"})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	_, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Input:      map[string]any{"url": "https://example.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "invalid_github_url") {
		t.Fatalf("err = %v, want invalid_github_url", err)
	}
	events := hook.Events()
	if len(events) != 2 || events[0].Type != HookBeforeTool || events[1].Type != HookAfterTool {
		t.Fatalf("hook events = %+v, want before/after", events)
	}
	if !strings.Contains(events[1].Error, "invalid_github_url") {
		t.Fatalf("after error = %q", events[1].Error)
	}
}

func TestGitHubProjectClientTimeoutReturnsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	reg := NewToolRegistry(NoopHook{})
	if err := reg.Register(NewMCPToolAdapter(ToolSpec{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Timeout:    time.Nanosecond,
	}, GitHubProjectClient{HTTPClient: server.Client(), BaseURL: server.URL})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	_, err := reg.Call(context.Background(), ToolCall{
		Name:       "github.project_analyze",
		Permission: PermissionReadOnly,
		Input:      map[string]any{"url": "https://github.com/acme/repo"},
	})
	if !mcpToolErrorCodeIs(err, "request_failed") {
		t.Fatalf("err = %v, want request_failed", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline wrapping", err)
	}
}

func mcpToolErrorCodeIs(err error, code string) bool {
	var toolErr MCPToolError
	if !errors.As(err, &toolErr) {
		return false
	}
	return toolErr.Code == code
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type countingMCPClient struct {
	result MCPResult
	calls  int
}

func (c *countingMCPClient) CallTool(context.Context, MCPCall) (MCPResult, error) {
	c.calls++
	return c.result, nil
}
