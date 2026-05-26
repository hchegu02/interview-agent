package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// makeServer 构造一个可控制响应的 OpenAI-compatible 测试服务器。
// handler 接收第 N 次请求（从 0 起），返回 status code + body。
func makeServer(t *testing.T, handler func(attempt int, w http.ResponseWriter, r *http.Request)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1) - 1
		handler(int(n), w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func okBody(content string, prompt, completion int) string {
	return fmt.Sprintf(`{"choices":[{"message":{"content":%q},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d},
		"model":"test-model"}`,
		content, prompt, completion, prompt+completion)
}

func newClient(srv *httptest.Server) *RealChatModel {
	c := NewRealChatModel(srv.URL, "test-key", "test-model", 5*time.Second)
	c.BaseDelay = 1 * time.Millisecond // 测试加速
	c.MaxDelay = 5 * time.Millisecond
	return c
}

func TestReal_Success(t *testing.T) {
	srv, count := makeServer(t, func(_ int, w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okBody(`{"hello":"world"}`, 10, 5))
	})
	c := newClient(srv)

	resp, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Content != `{"hello":"world"}` {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.PromptTokens != 10 || resp.CompletionTokens != 5 {
		t.Errorf("usage = %+v", resp)
	}
	if count.Load() != 1 {
		t.Errorf("expected 1 call, got %d", count.Load())
	}
}

func TestReal_RetryOn5xx(t *testing.T) {
	srv, count := makeServer(t, func(attempt int, w http.ResponseWriter, _ *http.Request) {
		if attempt < 2 {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"error":"upstream busy"}`)
			return
		}
		_, _ = io.WriteString(w, okBody(`{"ok":true}`, 1, 1))
	})
	c := newClient(srv)

	resp, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if err != nil {
		t.Fatalf("should recover after 503: %v", err)
	}
	if resp.Content != `{"ok":true}` {
		t.Errorf("content = %q", resp.Content)
	}
	if count.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", count.Load())
	}
}

func TestReal_RetryOn429(t *testing.T) {
	srv, count := makeServer(t, func(attempt int, w http.ResponseWriter, _ *http.Request) {
		if attempt == 0 {
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `rate limited`)
			return
		}
		_, _ = io.WriteString(w, okBody(`{}`, 1, 1))
	})
	c := newClient(srv)

	_, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if err != nil {
		t.Fatalf("should recover after 429: %v", err)
	}
	if count.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", count.Load())
	}
}

func TestReal_PermanentOn4xx(t *testing.T) {
	srv, count := makeServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `unauthorized`)
	})
	c := newClient(srv)

	_, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPermanent) {
		t.Errorf("want ErrPermanent, got %v", err)
	}
	if count.Load() != 1 {
		t.Errorf("4xx must not retry; got %d attempts", count.Load())
	}
}

func TestReal_ExhaustRetries(t *testing.T) {
	srv, count := makeServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(502)
	})
	c := newClient(srv)
	c.MaxRetries = 3

	_, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if count.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", count.Load())
	}
	if !strings.Contains(err.Error(), "retry exhausted") {
		t.Errorf("error should mention exhaustion: %v", err)
	}
}

func TestReal_RespectsCtxCancel(t *testing.T) {
	srv, _ := makeServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		// 故意慢，让 ctx 先取消
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, okBody(`{}`, 1, 1))
	})
	c := newClient(srv)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := c.Generate(ctx, []Message{{Role: "user", Content: "hi"}}, Options{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestReal_EmptyChoices(t *testing.T) {
	srv, _ := makeServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[],"usage":{},"model":"x"}`)
	})
	c := newClient(srv)

	_, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("want ErrEmptyResponse, got %v", err)
	}
}

func TestReal_ApiErrorField(t *testing.T) {
	srv, _ := makeServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		// 200 但 body 是结构化错误（DashScope 的常见返回方式）
		_, _ = io.WriteString(w, `{"error":{"message":"invalid model","code":"NotFound","type":"InvalidParameter"}}`)
	})
	c := newClient(srv)

	_, err := c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, Options{})
	if !errors.Is(err, ErrPermanent) {
		t.Errorf("want ErrPermanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid model") {
		t.Errorf("error should bubble up message: %v", err)
	}
}

func TestReal_JSONModeRequestShape(t *testing.T) {
	var bodyCaptured string
	srv, _ := makeServer(t, func(_ int, w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodyCaptured = string(b)
		_, _ = io.WriteString(w, okBody(`{}`, 1, 1))
	})
	c := newClient(srv)

	_, _ = c.Generate(context.Background(),
		[]Message{{Role: "user", Content: "hi"}},
		Options{ResponseFormat: "json_object"})

	if !strings.Contains(bodyCaptured, `"response_format":{"type":"json_object"}`) {
		t.Errorf("response_format not propagated; body=%s", bodyCaptured)
	}
}

func TestClassifyHTTPStatus(t *testing.T) {
	cases := []struct {
		status   int
		wantNil  bool
		wantKind error
	}{
		{200, true, nil},
		{299, true, nil},
		{408, false, ErrTransient},
		{429, false, ErrTransient},
		{500, false, ErrTransient},
		{503, false, ErrTransient},
		{599, false, ErrTransient},
		{400, false, ErrPermanent},
		{401, false, ErrPermanent},
		{403, false, ErrPermanent},
		{404, false, ErrPermanent},
	}
	for _, c := range cases {
		err := classifyHTTPStatus(c.status, "")
		if c.wantNil {
			if err != nil {
				t.Errorf("%d: want nil, got %v", c.status, err)
			}
			continue
		}
		if !errors.Is(err, c.wantKind) {
			t.Errorf("%d: want %v, got %v", c.status, c.wantKind, err)
		}
	}
}
