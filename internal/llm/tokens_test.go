package llm

import (
	"sync"
	"testing"
)

func TestTokenTracker_Add(t *testing.T) {
	tt := &TokenTracker{}
	tt.Add(&Response{PromptTokens: 10, CompletionTokens: 5})
	tt.Add(&Response{PromptTokens: 3, CompletionTokens: 2})

	if tt.PromptTokens() != 13 {
		t.Errorf("prompt = %d, want 13", tt.PromptTokens())
	}
	if tt.CompletionTokens() != 7 {
		t.Errorf("completion = %d, want 7", tt.CompletionTokens())
	}
	if tt.TotalTokens() != 20 {
		t.Errorf("total = %d, want 20", tt.TotalTokens())
	}
	if tt.Calls() != 2 {
		t.Errorf("calls = %d, want 2", tt.Calls())
	}
}

func TestTokenTracker_NilSafe(t *testing.T) {
	tt := &TokenTracker{}
	tt.Add(nil) // 不应 panic
	if tt.Calls() != 0 {
		t.Errorf("nil add shouldn't count; got %d", tt.Calls())
	}
}

func TestTokenTracker_Concurrent(t *testing.T) {
	// 验证并发 Add 不丢失（atomic 正确性）。
	// -race 会捕获潜在数据竞争。
	tt := &TokenTracker{}
	const goroutines = 50
	const perG = 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				tt.Add(&Response{PromptTokens: 1, CompletionTokens: 1})
			}
		}()
	}
	wg.Wait()

	want := int64(goroutines * perG)
	if tt.Calls() != want {
		t.Errorf("calls = %d, want %d", tt.Calls(), want)
	}
	if tt.PromptTokens() != want {
		t.Errorf("prompt = %d, want %d", tt.PromptTokens(), want)
	}
}

func TestTokenTracker_Reset(t *testing.T) {
	tt := &TokenTracker{}
	tt.Add(&Response{PromptTokens: 10, CompletionTokens: 5})
	tt.Reset()
	if tt.TotalTokens() != 0 || tt.Calls() != 0 {
		t.Errorf("Reset didn't clear: total=%d calls=%d", tt.TotalTokens(), tt.Calls())
	}
}
