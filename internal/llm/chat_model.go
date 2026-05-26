// Package llm 抽象 LLM 调用。
//
// 接口设计目标：
//   - 上层（Graph 节点）只依赖 ChatModel 接口，不关心是 Mock 还是 Real
//   - Mock 模式从 testdata/fixtures 读固定 JSON，让本地开发 / CI / 演示
//     都不需要真实 API key
//   - Real 模式走 OpenAI-compatible API（兼容阿里通义、DeepSeek 等）
//   - JSON Schema 约束 + 解析重试 + 超时控制在阶段 2 由 graph 层调用
package llm

import (
	"context"
	"errors"
)

// Message 是一次 chat 输入单元，遵循 OpenAI chat 协议。
type Message struct {
	Role    string `json:"role"`    // system | user | assistant
	Content string `json:"content"`
}

// Options 是单次调用可覆盖的参数。
type Options struct {
	Model       string  // 不填走默认
	MaxTokens   int
	Temperature float64
	// ResponseFormat = "json_object" 时强制返回合法 JSON
	ResponseFormat string
}

// Response 是一次非流式响应。
type Response struct {
	Content      string
	Model        string
	PromptTokens int
	CompletionTokens int
}

// Chunk 是一次流式响应的增量片段。
type Chunk struct {
	Delta string
	Done  bool
	Err   error
}

// ChatModel 是 LLM 客户端的抽象接口。
// 实现：MockChatModel（testdata 驱动）、RealChatModel（HTTP 调用）。
type ChatModel interface {
	Generate(ctx context.Context, messages []Message, opts Options) (*Response, error)
	Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error)
	// Name 返回当前模型标识，用于 metrics / 降级链路 label。
	Name() string
}

// ErrFixtureMissing 当 Mock 找不到 fixture 时返回，方便测试断言。
var ErrFixtureMissing = errors.New("llm mock: fixture missing")
