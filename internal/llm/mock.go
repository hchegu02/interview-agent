package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MockChatModel 从 testdata/fixtures/ 读取预设 JSON 文件返回。
//
// 设计目标：
//   - 让 CI、单测、本地演示完全脱离真实 LLM
//   - 通过 prompt 第一条 user message 的前 64 字符 hash 选 fixture，
//     让节点测试可以稳定复现
//   - Stream 模式按字符切片模拟流式返回
type MockChatModel struct {
	FixtureDir string // 默认 "testdata/fixtures"
	Latency    time.Duration
}

func NewMockChatModel(dir string) *MockChatModel {
	if dir == "" {
		dir = "testdata/fixtures"
	}
	return &MockChatModel{FixtureDir: dir, Latency: 50 * time.Millisecond}
}

func (m *MockChatModel) Name() string { return "mock" }

func (m *MockChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	if err := sleepCtx(ctx, m.Latency); err != nil {
		return nil, err
	}
	body, err := m.lookup(messages)
	if err != nil {
		return nil, err
	}
	return &Response{Content: body, Model: m.Name(), PromptTokens: len(messages) * 10, CompletionTokens: len(body) / 4}, nil
}

func (m *MockChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	body, err := m.lookup(messages)
	if err != nil {
		return nil, err
	}
	ch := make(chan Chunk, 8)
	go func() {
		defer close(ch)
		// 把 body 切成若干段，模拟 token 流
		runes := []rune(body)
		step := 6
		for i := 0; i < len(runes); i += step {
			j := i + step
			if j > len(runes) {
				j = len(runes)
			}
			select {
			case <-ctx.Done():
				ch <- Chunk{Err: ctx.Err()}
				return
			case ch <- Chunk{Delta: string(runes[i:j])}:
			}
			_ = sleepCtx(ctx, 20*time.Millisecond)
		}
		ch <- Chunk{Done: true}
	}()
	return ch, nil
}

// lookup 根据消息内容选择 fixture。
// 选取规则：取最后一条 user/system 消息前 64 字符的 SHA-like 简化指纹。
// 如果文件不存在则 fallback 到 default.json。
func (m *MockChatModel) lookup(messages []Message) (string, error) {
	fingerprint := fingerprint(messages)
	candidates := []string{fingerprint + ".json", "default.json"}
	for _, name := range candidates {
		path := filepath.Join(m.FixtureDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// fixture 文件本身就是 JSON content。
		// 我们做一次合法性校验，然后把原始 JSON 字符串返回让 Graph 层 unmarshal。
		var parsed interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", fmt.Errorf("fixture %s invalid json: %w", path, err)
		}
		return string(raw), nil
	}
	if body, ok := builtinDemoResponse(messages); ok {
		return body, nil
	}
	return "", fmt.Errorf("%w: tried %v in %s", ErrFixtureMissing, candidates, m.FixtureDir)
}

func builtinDemoResponse(messages []Message) (string, bool) {
	if len(messages) == 0 {
		return "", false
	}
	prompt := messages[len(messages)-1].Content
	switch {
	case strings.Contains(prompt, "岗位 JD 分析助手"):
		return `{"title":"Go 后端工程师","level":"junior","key_skills":["go","redis"],"must_have":["go"],"nice_to_have":[],"years_required":1}`, true
	case strings.Contains(prompt, "简历分析助手"):
		return `{"years":2,"skills":["go","redis"],"projects":[],"highlights":["做过 Go 后端服务"]}`, true
	case strings.Contains(prompt, "从候选题库中挑下一道题"):
		id := firstCandidateID(prompt)
		if id == "" {
			id = "fallback-go-001"
		}
		return fmt.Sprintf(`{"next_question_id":%q,"reasoning":"mock 模式优先验证基础能力"}`, id), true
	case strings.Contains(prompt, "评估候选人对一道技术题的回答"):
		return `{"question_id":"mock","score":72,"strengths":["回答覆盖了核心概念"],"weaknesses":["细节还可以展开"],"suggestion":"补充关键边界和例子"}`, true
	case strings.Contains(prompt, "审视一次评估并判断是否值得追问"):
		return `{"grounded_score":85,"need_refine":false,"issues":[],"summary":"评估基本可靠","has_probe_signal":false,"probe_topic":""}`, true
	case strings.Contains(prompt, "刚结算完一道题"):
		return `{"action":"end","reasoning":"mock 模式完成一轮后生成报告","reflect_topic":""}`, true
	case strings.Contains(prompt, "根据 critic 反馈重做一次评估"):
		return `{"question_id":"mock","score":70,"strengths":["有基本理解"],"weaknesses":["深度不足"],"suggestion":"补充实现细节"}`, true
	case strings.Contains(prompt, "正在追问候选人"):
		return `{"question":"请补充一个具体实现细节。","reason":"验证答案真实性"}`, true
	case strings.Contains(prompt, "刚问完一个追问"):
		return `{"score":70,"strengths":["补充了细节"],"weaknesses":["边界仍少"],"suggestion":"继续补边界","has_more_probe":false,"next_probe_topic":""}`, true
	default:
		return "", false
	}
}

func firstCandidateID(prompt string) string {
	const marker = "- id="
	i := strings.Index(prompt, marker)
	if i < 0 {
		return ""
	}
	rest := prompt[i+len(marker):]
	for j, r := range rest {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return rest[:j]
		}
	}
	return rest
}

func fingerprint(messages []Message) string {
	if len(messages) == 0 {
		return "empty"
	}
	last := messages[len(messages)-1].Content
	last = strings.TrimSpace(last)
	if len(last) > 64 {
		last = last[:64]
	}
	// 简单非加密映射：取所有字符 byte 累加 mod 1e6
	var sum uint32
	for _, b := range []byte(last) {
		sum = sum*31 + uint32(b)
	}
	return fmt.Sprintf("fp_%08x", sum)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
