package agentkit

import "context"

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
	result, err := m.client.CallTool(ctx, MCPCall{Name: call.Name, Input: call.Input})
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Output: result.Output, Summary: result.Summary}, nil
}
