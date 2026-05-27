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

func TestWebAppIncludesLiveStatusAndUsableAnswerDock(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	index := httptest.NewRecorder()
	router.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d", index.Code)
	}
	html := index.Body.String()
	for _, marker := range []string{
		`id="setupNotice"`,
		`id="eventTimeline"`,
		`id="resumeFile"`,
		`answer-hint`,
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("index html missing marker %q", marker)
		}
	}

	js := httptest.NewRecorder()
	router.ServeHTTP(js, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if js.Code != http.StatusOK {
		t.Fatalf("js status = %d", js.Code)
	}
	script := js.Body.String()
	for _, marker := range []string{
		"parseResumeFile",
		"/api/documents/parse-resume",
		"pushStreamEvent",
		"renderEventTimeline",
		"state.pendingAnswer",
		"state.lastEventId = \"\"",
	} {
		if !strings.Contains(script, marker) {
			t.Fatalf("app.js missing marker %q", marker)
		}
	}

	css := httptest.NewRecorder()
	router.ServeHTTP(css, httptest.NewRequest(http.MethodGet, "/assets/app.css", nil))
	if css.Code != http.StatusOK {
		t.Fatalf("css status = %d", css.Code)
	}
	styles := css.Body.String()
	for _, marker := range []string{
		".event-timeline",
		".answer-dock",
		"position: sticky",
	} {
		if !strings.Contains(styles, marker) {
			t.Fatalf("app.css missing marker %q", marker)
		}
	}
}
