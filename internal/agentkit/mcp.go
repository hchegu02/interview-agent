package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// MCPToolError 是真实工具 client 返回的稳定错误边界。
// 调用方应依赖 Code 做分类，不要解析底层 HTTP 或网络错误文本。
type MCPToolError struct {
	Code    string
	Message string
	Err     error
}

func (e MCPToolError) Error() string {
	if e.Message == "" {
		return "agentkit: mcp tool error: " + e.Code
	}
	return "agentkit: mcp tool error: " + e.Code + ": " + e.Message
}

func (e MCPToolError) Unwrap() error {
	return e.Err
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

// GitHubProjectClient 是显式注入的低风险只读 GitHub 项目 client。
// 它不会被 RegisterDefaultMCPTools 默认启用，也不携带 token 或全局配置。
type GitHubProjectClient struct {
	HTTPClient *http.Client
	BaseURL    string
}

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

func (c GitHubProjectClient) CallTool(ctx context.Context, call MCPCall) (MCPResult, error) {
	if strings.TrimSpace(call.Name) != "github.project_analyze" {
		return MCPResult{}, MCPToolError{
			Code:    "unsupported_tool",
			Message: strings.TrimSpace(call.Name),
			Err:     ErrToolNotFound,
		}
	}
	rawURL := inputString(call.Input, "url")
	owner, repo, canonicalURL, err := parseGitHubRepoURL(rawURL)
	if err != nil {
		return MCPResult{}, err
	}
	endpoint, err := c.repoAPIURL(owner, repo)
	if err != nil {
		return MCPResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return MCPResult{}, MCPToolError{Code: "request_invalid", Message: "build github request", Err: err}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "interview-agent-agentkit")

	httpClient, err := c.httpClient()
	if err != nil {
		return MCPResult{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return MCPResult{}, MCPToolError{Code: "request_failed", Message: "github request failed", Err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return MCPResult{}, MCPToolError{Code: "response_read_failed", Message: "read github response", Err: err}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return MCPResult{}, MCPToolError{
			Code:    "http_status",
			Message: fmt.Sprintf("github returned status %d", resp.StatusCode),
		}
	}

	var api githubProjectAPIResponse
	if err := json.Unmarshal(body, &api); err != nil {
		return MCPResult{}, MCPToolError{Code: "response_invalid", Message: "decode github response", Err: err}
	}
	project := strings.TrimSpace(api.FullName)
	if project == "" {
		project = owner + "/" + repo
	}
	language := strings.TrimSpace(api.Language)
	if language == "" {
		language = "unknown"
	}
	description := strings.TrimSpace(api.Description)
	if description == "" {
		description = "No repository description provided."
	}
	htmlURL := strings.TrimSpace(api.HTMLURL)
	if htmlURL == "" {
		htmlURL = canonicalURL
	}

	output := map[string]any{
		"url":              canonicalURL,
		"project":          project,
		"summary":          description,
		"primary_language": language,
		"default_branch":   strings.TrimSpace(api.DefaultBranch),
		"source":           "github_api",
		"highlights": []string{
			"repository metadata fetched from GitHub API",
			"read-only project analysis stayed behind ToolRegistry",
		},
		"risk_points": []string{
			"analysis uses public repository metadata only",
			"network, API status, or rate limits are returned as tool errors",
		},
		"html_url": htmlURL,
	}
	return MCPResult{Output: output, Summary: "github analysis for " + project}, nil
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

type githubProjectAPIResponse struct {
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	Language      string `json:"language"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
}

func (c GitHubProjectClient) repoAPIURL(owner, repo string) (string, error) {
	baseURL := strings.TrimSpace(c.BaseURL)
	if baseURL == "" {
		return "", MCPToolError{Code: "config_missing", Message: "github api base url is required"}
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if err == nil {
			err = fmt.Errorf("missing scheme or host")
		}
		return "", MCPToolError{Code: "config_invalid", Message: "invalid github api base url", Err: err}
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	basePath := strings.TrimRight(parsed.Path, "/")
	parsed.Path = basePath + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	return parsed.String(), nil
}

func (c GitHubProjectClient) httpClient() (*http.Client, error) {
	if c.HTTPClient == nil {
		return nil, MCPToolError{Code: "config_missing", Message: "github http client is required"}
	}
	return c.HTTPClient, nil
}

func parseGitHubRepoURL(rawURL string) (string, string, string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", "", "", MCPToolError{Code: "invalid_input", Message: "missing github repository url"}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if err == nil {
			err = fmt.Errorf("missing scheme or host")
		}
		return "", "", "", MCPToolError{Code: "invalid_github_url", Message: "invalid github repository url", Err: err}
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", "", "", MCPToolError{Code: "invalid_github_url", Message: "github url must use http or https"}
	}
	if parsed.User != nil {
		return "", "", "", MCPToolError{Code: "invalid_github_url", Message: "github url must not contain userinfo"}
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	if host != "github.com" {
		return "", "", "", MCPToolError{Code: "invalid_github_url", Message: "repository host must be github.com"}
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", "", MCPToolError{Code: "invalid_github_url", Message: "github url must include owner and repo"}
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return "", "", "", MCPToolError{Code: "invalid_github_url", Message: "github url must include owner and repo"}
	}
	return owner, repo, "https://github.com/" + owner + "/" + repo, nil
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
