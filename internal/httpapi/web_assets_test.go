package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interview-agent/internal/config"
)

func TestRouterServesWebApp(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "InterviewAgent") {
		t.Fatalf("index html missing app marker")
	}
}

func TestRouterServesWebAssets(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "startInterview") {
		t.Fatalf("asset body missing app code")
	}
}
