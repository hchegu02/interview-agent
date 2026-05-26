package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMaxInFlightMiddleware_AllowsBelowLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MaxInFlightMiddleware(2))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: code = %d, want 200", i, w.Code)
		}
	}
}

func TestMaxInFlightMiddleware_RejectsOverLimitWith503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MaxInFlightMiddleware(1))

	hold := make(chan struct{})
	entered := make(chan struct{}, 1)
	r.GET("/hold", func(c *gin.Context) {
		entered <- struct{}{}
		<-hold
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 第一个请求占住槽位
	firstDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/hold", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		firstDone <- w.Code
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter handler")
	}

	// 第二个请求应立即 503
	req := httptest.NewRequest(http.MethodGet, "/hold", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want \"1\"", got)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "busy") {
		t.Fatalf("body.error = %v, want contains \"busy\"", body["error"])
	}
	if v, _ := body["retry_after_seconds"].(float64); int(v) != 1 {
		t.Fatalf("body.retry_after_seconds = %v, want 1", body["retry_after_seconds"])
	}

	close(hold)
	if code := <-firstDone; code != http.StatusOK {
		t.Fatalf("first request code = %d, want 200", code)
	}
}

func TestMaxInFlightMiddleware_LimitZeroIsNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MaxInFlightMiddleware(0))

	// 即便并发 10 个，limit=0 表示 no-op，全部应该 200
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/ok", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200 (limit=0 should be no-op)", code)
		}
	}
}

func TestMaxInFlightMiddleware_ReleasesSlotAfterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MaxInFlightMiddleware(1))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 串行打 5 次：每次结束后 slot 必须释放，下一次仍能拿到
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: code = %d, want 200", i, w.Code)
		}
	}
}

// TestRouter_BackpressureScopedToMutatingEndpoints 验证背压作用域：
// /healthz、/readyz、SSE stream、sessions 读路径在 LLM 路径占满时仍可用。
func TestRouter_BackpressureScopedToMutatingEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// 模仿真实 router 的结构：把 limit=1 挂在 /api/interview/start 上。
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })
	r.GET("/api/interview/stream", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"stream": "ok"}) })
	r.GET("/api/interview/sessions", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"sessions": []string{}}) })

	hold := make(chan struct{})
	entered := make(chan struct{}, 1)
	mutating := r.Group("/api/interview")
	mutating.Use(MaxInFlightMiddleware(1))
	mutating.POST("/start", func(c *gin.Context) {
		entered <- struct{}{}
		<-hold
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 占住 mutating slot
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/interview/start", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first /start did not enter")
	}

	// 不受背压的路径都应 200
	for _, path := range []string{"/healthz", "/readyz", "/api/interview/stream", "/api/interview/sessions"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s code = %d, want 200 (must not be backpressured)", path, w.Code)
		}
	}

	// 新打 /start 仍要 503
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("/start code = %d, want 503 (mutating endpoint must be backpressured)", w.Code)
	}

	close(hold)
}
