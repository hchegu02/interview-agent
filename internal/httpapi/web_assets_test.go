package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interview-agent/internal/config"
)

func TestRouterRedirectsRootToResume(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	if loc := rec.Header().Get("Location"); loc != "/resume" {
		t.Fatalf("location = %q, want /resume", loc)
	}
}

func TestRouterServesReactAppRoutes(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	for _, path := range []string{"/resume", "/jd", "/interview", "/report", "/questions"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("%s content type = %q, want text/html", path, ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `id="root"`) || !strings.Contains(body, "/assets/") {
			t.Fatalf("%s html missing React/Vite markers: %s", path, body)
		}
	}
}

func TestRouterServesViteAssets(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	index := httptest.NewRecorder()
	router.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/resume", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d", index.Code)
	}
	assetPath := firstAssetPath(index.Body.String(), ".js")
	if assetPath == "" {
		t.Fatalf("index html missing JS asset: %s", index.Body.String())
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, assetPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "interview_agent_draft_v1") {
		t.Fatalf("asset body missing React app marker")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("cache-control = %q, want immutable", cc)
	}
}

func firstAssetPath(html, suffix string) string {
	for _, part := range strings.Split(html, `"`) {
		if strings.HasPrefix(part, "/assets/") && strings.HasSuffix(part, suffix) {
			return part
		}
	}
	return ""
}
