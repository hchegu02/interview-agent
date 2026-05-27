package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type metricsRecorder struct {
	mu sync.Mutex

	httpRequests map[httpMetricKey]uint64
	parserDocs   map[string]uint64
	sseActive    int64
	sseTotal     uint64
}

type httpMetricKey struct {
	method string
	path   string
	status int
}

func newMetricsRecorder() *metricsRecorder {
	return &metricsRecorder{
		httpRequests: map[httpMetricKey]uint64{},
		parserDocs:   map[string]uint64{},
	}
}

func (m *metricsRecorder) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		m.recordHTTPRequest(c.Request.Method, path, c.Writer.Status(), time.Since(started))
	}
}

func (m *metricsRecorder) recordHTTPRequest(method, path string, status int, _ time.Duration) {
	if m == nil {
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpRequests[httpMetricKey{method: method, path: path, status: status}]++
}

func (m *metricsRecorder) recordParserDocument(status string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parserDocs[status]++
}

func (m *metricsRecorder) beginSSEConnection() func() {
	if m == nil {
		return func() {}
	}
	m.mu.Lock()
	m.sseActive++
	m.sseTotal++
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		if m.sseActive > 0 {
			m.sseActive--
		}
		m.mu.Unlock()
	}
}

func (m *metricsRecorder) renderPrometheus() string {
	if m == nil {
		m = newMetricsRecorder()
	}
	m.mu.Lock()
	httpRequests := make(map[httpMetricKey]uint64, len(m.httpRequests))
	for k, v := range m.httpRequests {
		httpRequests[k] = v
	}
	parserDocs := make(map[string]uint64, len(m.parserDocs))
	for k, v := range m.parserDocs {
		parserDocs[k] = v
	}
	sseActive := m.sseActive
	sseTotal := m.sseTotal
	m.mu.Unlock()

	var b strings.Builder
	b.WriteString("# HELP interview_http_requests_total Total HTTP requests by method, normalized path and status.\n")
	b.WriteString("# TYPE interview_http_requests_total counter\n")
	keys := make([]httpMetricKey, 0, len(httpRequests))
	for k := range httpRequests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	for _, k := range keys {
		fmt.Fprintf(&b, "interview_http_requests_total{method=%q,path=%q,status=%q} %d\n",
			k.method, k.path, strconv.Itoa(k.status), httpRequests[k])
	}

	b.WriteString("# HELP interview_parser_documents_total Total parsed resume documents by status.\n")
	b.WriteString("# TYPE interview_parser_documents_total counter\n")
	parserStatuses := make([]string, 0, len(parserDocs))
	for status := range parserDocs {
		parserStatuses = append(parserStatuses, status)
	}
	sort.Strings(parserStatuses)
	for _, status := range parserStatuses {
		fmt.Fprintf(&b, "interview_parser_documents_total{status=%q} %d\n", status, parserDocs[status])
	}

	b.WriteString("# HELP interview_sse_connections_active Active SSE stream connections.\n")
	b.WriteString("# TYPE interview_sse_connections_active gauge\n")
	fmt.Fprintf(&b, "interview_sse_connections_active %d\n", sseActive)
	b.WriteString("# HELP interview_sse_connections_total Total accepted SSE stream connections.\n")
	b.WriteString("# TYPE interview_sse_connections_total counter\n")
	fmt.Fprintf(&b, "interview_sse_connections_total %d\n", sseTotal)
	return b.String()
}

func (s *Server) metrics(c *gin.Context) {
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(s.metricsRecorder.renderPrometheus()))
}
