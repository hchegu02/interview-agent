package embedding

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

func embedBody(vectors [][]float32) string {
	var sb strings.Builder
	sb.WriteString(`{"data":[`)
	for i, v := range vectors {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"embedding":[`))
		for j, x := range v {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("%v", x))
		}
		sb.WriteString(fmt.Sprintf(`],"index":%d}`, i))
	}
	sb.WriteString(`],"model":"test","usage":{"prompt_tokens":1,"total_tokens":1}}`)
	return sb.String()
}

func makeEmbSrv(t *testing.T, h func(int, http.ResponseWriter, *http.Request)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var c atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := c.Add(1) - 1
		h(int(n), w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &c
}

func newEmbedder(srv *httptest.Server, dim int) *RealEmbedder {
	e := NewRealEmbedder(srv.URL, "test-key", "text-embedding-v4", dim, 5*time.Second)
	e.BaseDelay = 1 * time.Millisecond
	return e
}

func TestEmbed_Success(t *testing.T) {
	srv, count := makeEmbSrv(t, func(_ int, w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth = %q", got)
		}
		_, _ = io.WriteString(w, embedBody([][]float32{
			{0.1, 0.2, 0.3, 0.4},
			{0.5, 0.6, 0.7, 0.8},
		}))
	})
	e := newEmbedder(srv, 4)

	out, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d vectors", len(out))
	}
	if len(out[0]) != 4 || out[0][0] != 0.1 {
		t.Errorf("vector 0 = %v", out[0])
	}
	if count.Load() != 1 {
		t.Errorf("expected 1 call, got %d", count.Load())
	}
}

func TestEmbed_RetryOn5xx(t *testing.T) {
	srv, count := makeEmbSrv(t, func(attempt int, w http.ResponseWriter, _ *http.Request) {
		if attempt < 1 {
			w.WriteHeader(502)
			return
		}
		_, _ = io.WriteString(w, embedBody([][]float32{{0.1, 0.2}}))
	})
	e := newEmbedder(srv, 2)

	out, err := e.Embed(context.Background(), []string{"hi"})
	if err != nil {
		t.Fatalf("should recover: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d", len(out))
	}
	if count.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", count.Load())
	}
}

func TestEmbed_PermanentOn4xx(t *testing.T) {
	srv, count := makeEmbSrv(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `unauthorized`)
	})
	e := newEmbedder(srv, 4)

	_, err := e.Embed(context.Background(), []string{"x"})
	if !errors.Is(err, errPermanent) {
		t.Errorf("want errPermanent, got %v", err)
	}
	if count.Load() != 1 {
		t.Errorf("4xx must not retry; got %d", count.Load())
	}
}

func TestEmbed_DimMismatch(t *testing.T) {
	// 服务器返回 3 维向量但 embedder 配置成 4 维 → 报错
	srv, _ := makeEmbSrv(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, embedBody([][]float32{{0.1, 0.2, 0.3}}))
	})
	e := newEmbedder(srv, 4)

	_, err := e.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected dim mismatch error")
	}
	if !strings.Contains(err.Error(), "dim") {
		t.Errorf("error should mention dim: %v", err)
	}
}

func TestEmbed_EmptyInput(t *testing.T) {
	// 空输入应直接返回，不发请求
	srv, count := makeEmbSrv(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		t.Error("should not be called")
	})
	e := newEmbedder(srv, 4)

	out, err := e.Embed(context.Background(), nil)
	if err != nil || out != nil {
		t.Errorf("empty input: out=%v err=%v", out, err)
	}
	if count.Load() != 0 {
		t.Errorf("expected 0 calls, got %d", count.Load())
	}
}

func TestEmbed_OutOfOrderResponse(t *testing.T) {
	// 服务器按乱序返回（index=1 在前），客户端要按 index 排回正确顺序
	srv, _ := makeEmbSrv(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[
			{"embedding":[9.0,9.0],"index":1},
			{"embedding":[1.0,1.0],"index":0}
		],"model":"x","usage":{}}`)
	})
	e := newEmbedder(srv, 2)

	out, err := e.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if out[0][0] != 1.0 || out[1][0] != 9.0 {
		t.Errorf("ordering broken: %v", out)
	}
}
