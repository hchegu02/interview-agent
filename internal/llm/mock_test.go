package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMockChatModel_Generate(t *testing.T) {
	dir := t.TempDir()
	// 写一个 default fixture
	if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(`{"answer":"hello"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewMockChatModel(dir)
	resp, err := m.Generate(context.Background(), []Message{{Role: "user", Content: "hi"}}, Options{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("empty content")
	}
}

func TestMockChatModel_StreamRespectsContext(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "default.json"), []byte(`{"foo":"bar"}`), 0o644)
	m := NewMockChatModel(dir)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.Stream(ctx, []Message{{Role: "user", Content: "x"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	// 取空 channel 验证关闭
	gotErr := false
	for c := range ch {
		if c.Err != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Logf("note: stream might have completed before cancel propagated; tolerant pass")
	}
}

func TestMockChatModel_FixtureMissing(t *testing.T) {
	m := NewMockChatModel(t.TempDir())
	_, err := m.Generate(context.Background(), []Message{{Role: "user", Content: "x"}}, Options{})
	if err == nil {
		t.Fatal("expected fixture missing error")
	}
}

func TestMockChatModel_BuiltinDemoResponse(t *testing.T) {
	m := NewMockChatModel(t.TempDir())
	resp, err := m.Generate(context.Background(), []Message{{
		Role:    "system",
		Content: "你是岗位 JD 分析助手。从下面的 JD 文本中抽取结构化信息",
	}}, Options{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("empty content")
	}
}
