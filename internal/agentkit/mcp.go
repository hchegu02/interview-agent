package agentkit

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type MCPCall struct {
	Name  string
	Input any
}

type MCPResult struct {
	Output  any
	Summary string
}

type MCPClient interface {
	CallTool(context.Context, MCPCall) (MCPResult, error)
}

type MCPToolAdapter struct {
	spec   ToolSpec
	client MCPClient
}

func NewMCPToolAdapter(spec ToolSpec, client MCPClient) MCPToolAdapter {
	return MCPToolAdapter{spec: spec, client: client}
}

func (m MCPToolAdapter) Spec() ToolSpec {
	return m.spec
}

func (m MCPToolAdapter) Call(ctx context.Context, call ToolCall) (ToolResult, error) {
	if m.client == nil {
		return ToolResult{}, fmt.Errorf("%w: nil mcp client for tool %s", ErrInvalidSpec, m.spec.Name)
	}
	result, err := m.client.CallTool(ctx, MCPCall{Name: call.Name, Input: call.Input})
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Output: result.Output, Summary: result.Summary}, nil
}

// MockMCPClient 是本地 deterministic MCP client。
// 它只服务测试和演示，不访问真实 GitHub、网页或外部 MCP Server。
type MockMCPClient struct{}

// NewMockMCPClient 构造默认 mock MCP client。
func NewMockMCPClient() MockMCPClient {
	return MockMCPClient{}
}

// RegisterDefaultMCPTools 注册阶段 4 默认 mock 工具。
// 所有工具仍通过 ToolRegistry 的权限、超时和 Hook 边界执行。
func RegisterDefaultMCPTools(reg *ToolRegistry, client MCPClient) error {
	if reg == nil {
		return fmt.Errorf("%w: nil tool registry", ErrInvalidSpec)
	}
	if client == nil {
		client = NewMockMCPClient()
	}
	specs := []ToolSpec{
		{
			Name:          "github.project_analyze",
			Description:   "mock analysis for a GitHub project URL",
			InputSummary:  "github repository url",
			OutputSummary: "project summary, language, highlights and risks",
			Permission:    PermissionReadOnly,
			Timeout:       2 * time.Second,
		},
		{
			Name:          "web.fetch",
			Description:   "mock fetch for a web URL",
			InputSummary:  "web url",
			OutputSummary: "url, title and content summary",
			Permission:    PermissionReadOnly,
			Timeout:       2 * time.Second,
		},
	}
	for _, spec := range specs {
		if err := reg.Register(NewMCPToolAdapter(spec, client)); err != nil {
			return err
		}
	}
	return nil
}

func (m MockMCPClient) CallTool(_ context.Context, call MCPCall) (MCPResult, error) {
	switch strings.TrimSpace(call.Name) {
	case "github.project_analyze":
		return mockGitHubProjectAnalyze(call.Input)
	case "web.fetch":
		return mockWebFetch(call.Input)
	default:
		return MCPResult{}, fmt.Errorf("%w: %s", ErrToolNotFound, call.Name)
	}
}

func mockGitHubProjectAnalyze(input any) (MCPResult, error) {
	rawURL := inputString(input, "url")
	name := repoNameFromURL(rawURL)
	if name == "" {
		name = "unknown-project"
	}
	output := map[string]any{
		"url":              rawURL,
		"project":          name,
		"summary":          fmt.Sprintf("%s mock GitHub project analysis", name),
		"primary_language": "Go",
		"highlights": []string{
			"uses layered backend structure",
			"keeps tool calls behind AgentKit registry",
		},
		"risk_points": []string{
			"mock result only, no live repository inspection",
			"external network integration is not enabled",
		},
	}
	return MCPResult{Output: output, Summary: "mock github analysis for " + name}, nil
}

func mockWebFetch(input any) (MCPResult, error) {
	rawURL := inputString(input, "url")
	host := ""
	if parsed, err := url.Parse(rawURL); err == nil {
		host = parsed.Host
	}
	if host == "" {
		host = "unknown-host"
	}
	output := map[string]any{
		"url":             rawURL,
		"title":           "Mock page from " + host,
		"content_summary": "Mock web fetch result for local AgentKit tool-chain verification.",
	}
	return MCPResult{Output: output, Summary: "mock web fetch for " + host}, nil
}

func inputString(input any, key string) string {
	switch v := input.(type) {
	case map[string]any:
		if raw, ok := v[key]; ok {
			return strings.TrimSpace(fmt.Sprint(raw))
		}
	case map[string]string:
		return strings.TrimSpace(v[key])
	}
	return ""
}

func repoNameFromURL(rawURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}
