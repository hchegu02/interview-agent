package llm

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeModel 是简易 ChatModel：按调用顺序返回预设响应。
// 比 httptest 轻得多，专门测 schema 自纠正循环。
type fakeModel struct {
	responses []string
	idx       atomic.Int32
	lastConv  []Message // 记录最后一次收到的完整消息序列
}

func (f *fakeModel) Name() string { return "fake" }
func (f *fakeModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	f.lastConv = append([]Message(nil), messages...)
	i := f.idx.Add(1) - 1
	if int(i) >= len(f.responses) {
		return nil, errors.New("no more responses")
	}
	return &Response{Content: f.responses[i], PromptTokens: 1, CompletionTokens: 1}, nil
}
func (f *fakeModel) Stream(context.Context, []Message, Options) (<-chan Chunk, error) {
	return nil, errors.New("stream not supported")
}

func TestCallWithSchema_FirstTryOk(t *testing.T) {
	m := &fakeModel{responses: []string{`{"action":"ask_new"}`}}
	resp, err := CallWithSchema(context.Background(), m,
		[]Message{{Role: "user", Content: "pick"}},
		Options{},
		AndValidators(ValidateJSON, func(raw []byte) error {
			return ValidateEnum(raw, "action", "ask_new", "probe", "end")
		}),
		1)
	if err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	if resp.Content != `{"action":"ask_new"}` {
		t.Errorf("content = %q", resp.Content)
	}
	if m.idx.Load() != 1 {
		t.Errorf("should call exactly once; got %d", m.idx.Load())
	}
}

func TestCallWithSchema_SelfCorrect(t *testing.T) {
	m := &fakeModel{
		responses: []string{
			`{"action":"unknown"}`, // 第一次：enum 不在白名单
			`{"action":"probe"}`,   // 第二次：自纠正成功
		},
	}
	resp, err := CallWithSchema(context.Background(), m,
		[]Message{{Role: "user", Content: "pick"}},
		Options{},
		func(raw []byte) error {
			return ValidateEnum(raw, "action", "ask_new", "probe", "end")
		},
		1)
	if err != nil {
		t.Fatalf("should self-correct: %v", err)
	}
	if resp.Content != `{"action":"probe"}` {
		t.Errorf("content = %q", resp.Content)
	}
	if m.idx.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", m.idx.Load())
	}
	// 验证错误回灌：最后一次对话里应该包含 assistant 的坏回答 + user 的纠正提示
	hasBad, hasFix := false, false
	for _, msg := range m.lastConv {
		if msg.Role == "assistant" && strings.Contains(msg.Content, "unknown") {
			hasBad = true
		}
		if msg.Role == "user" && strings.Contains(msg.Content, "schema 校验") {
			hasFix = true
		}
	}
	if !hasBad || !hasFix {
		t.Errorf("self-correct prompt missing: hasBad=%v hasFix=%v conv=%+v", hasBad, hasFix, m.lastConv)
	}
}

func TestCallWithSchema_Exhausted(t *testing.T) {
	m := &fakeModel{
		responses: []string{`{"action":"x"}`, `{"action":"y"}`, `{"action":"z"}`},
	}
	_, err := CallWithSchema(context.Background(), m,
		[]Message{{Role: "user", Content: "pick"}},
		Options{},
		func(raw []byte) error {
			return ValidateEnum(raw, "action", "ask_new")
		},
		1) // 1 次自纠正 → 总共 2 次调用
	if err == nil {
		t.Fatal("expected ErrSchemaInvalid")
	}
	if !errors.Is(err, ErrSchemaInvalid) {
		t.Errorf("want ErrSchemaInvalid, got %v", err)
	}
	if m.idx.Load() != 2 {
		t.Errorf("expected 2 attempts (1 try + 1 fix), got %d", m.idx.Load())
	}
}

func TestCallWithSchema_PropagatesNetworkError(t *testing.T) {
	m := &errModel{err: errors.New("network down")}
	_, err := CallWithSchema(context.Background(), m,
		[]Message{{Role: "user", Content: "x"}},
		Options{}, ValidateJSON, 3)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Errorf("network error should bubble up, got %v", err)
	}
}

type errModel struct{ err error }

func (e *errModel) Name() string { return "err" }
func (e *errModel) Generate(context.Context, []Message, Options) (*Response, error) {
	return nil, e.err
}
func (e *errModel) Stream(context.Context, []Message, Options) (<-chan Chunk, error) {
	return nil, e.err
}

func TestValidateJSON(t *testing.T) {
	if err := ValidateJSON([]byte(`{"a":1}`)); err != nil {
		t.Errorf("valid JSON should pass: %v", err)
	}
	if err := ValidateJSON([]byte(`not json`)); err == nil {
		t.Error("invalid JSON should fail")
	}
}

func TestValidateFields(t *testing.T) {
	if err := ValidateFields([]byte(`{"a":1,"b":"x"}`), "a", "b"); err != nil {
		t.Errorf("all fields present should pass: %v", err)
	}
	if err := ValidateFields([]byte(`{"a":1}`), "a", "b"); err == nil {
		t.Error("missing field should fail")
	}
	if err := ValidateFields([]byte(`{"a":null}`), "a"); err == nil {
		t.Error("null field should fail")
	}
}

func TestValidateEnum(t *testing.T) {
	if err := ValidateEnum([]byte(`{"action":"probe"}`), "action", "ask_new", "probe"); err != nil {
		t.Errorf("matching enum should pass: %v", err)
	}
	if err := ValidateEnum([]byte(`{"action":"bad"}`), "action", "ask_new"); err == nil {
		t.Error("wrong enum should fail")
	}
	if err := ValidateEnum([]byte(`{"action":123}`), "action", "ask_new"); err == nil {
		t.Error("non-string enum should fail")
	}
}
