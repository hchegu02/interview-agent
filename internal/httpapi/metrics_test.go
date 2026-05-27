package httpapi

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"interview-agent/internal/config"
)

func TestMetricsEndpoint_RecordsHTTPAndParserCounters(t *testing.T) {
	server := NewServer(&config.Config{})
	router := server.Router()

	ping := httptest.NewRecorder()
	router.ServeHTTP(ping, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	if ping.Code != http.StatusOK {
		t.Fatalf("ping status = %d, body=%s", ping.Code, ping.Body.String())
	}

	body, contentType := buildMultipartFile(t, "file", "resume.txt", "张三\nGo 后端工程师")
	parse := httptest.NewRecorder()
	parseReq := httptest.NewRequest(http.MethodPost, "/api/documents/parse-resume", body)
	parseReq.Header.Set("Content-Type", contentType)
	router.ServeHTTP(parse, parseReq)
	if parse.Code != http.StatusOK {
		t.Fatalf("parse status = %d, body=%s", parse.Code, parse.Body.String())
	}

	metrics := httptest.NewRecorder()
	router.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, body=%s", metrics.Code, metrics.Body.String())
	}
	got := metrics.Body.String()
	for _, marker := range []string{
		`interview_http_requests_total{method="GET",path="/api/ping",status="200"} 1`,
		`interview_parser_documents_total{status="ok"} 1`,
		`interview_sse_connections_active 0`,
	} {
		if !strings.Contains(got, marker) {
			t.Fatalf("metrics missing %q\n--- metrics ---\n%s", marker, got)
		}
	}
	if ct := metrics.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("metrics content-type = %q", ct)
	}
}

func TestMetricsEndpoint_RecordsSSEConnections(t *testing.T) {
	hub := NewMemoryInterviewEventHub(8)
	svc := NewInterviewServiceWithStoreAndEvents(fakeInterviewRunner{}, NewMemorySessionStore(), hub)
	server := NewServerWithInterview(&config.Config{}, svc)

	if _, err := svc.Start(context.Background(), startInterviewRequest{
		SessionID:  "s-metrics-stream",
		JDText:     "Go 后端",
		ResumeText: "Go 候选人",
	}); err != nil {
		t.Fatalf("start interview: %v", err)
	}

	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/interview/stream?session_id=s-metrics-stream", nil)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	if first := readSSEBlock(t, bufio.NewReader(resp.Body)); !strings.Contains(first, "event: snapshot") {
		t.Fatalf("first sse block = %q", first)
	}

	got := getMetrics(t, ts.URL)
	for _, marker := range []string{
		`interview_sse_connections_active 1`,
		`interview_sse_connections_total 1`,
	} {
		if !strings.Contains(got, marker) {
			t.Fatalf("metrics missing %q\n--- metrics ---\n%s", marker, got)
		}
	}

	cancel()
	resp.Body.Close()
	eventuallyMetricsContains(t, ts.URL, `interview_sse_connections_active 0`)
}

func getMetrics(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	return string(raw)
}

func eventuallyMetricsContains(t *testing.T, baseURL, marker string) {
	t.Helper()
	var got string
	for range 20 {
		got = getMetrics(t, baseURL)
		if strings.Contains(got, marker) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("metrics missing %q\n--- metrics ---\n%s", marker, got)
}
