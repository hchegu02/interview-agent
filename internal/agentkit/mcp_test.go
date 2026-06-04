package agentkit

import (
	"context"
	"errors"
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
