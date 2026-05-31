package httpapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type metricsRecorder struct {
	mu sync.Mutex

	httpRequests           map[httpMetricKey]uint64
	httpDurations          map[httpMetricKey]histogramMetric
	parserDocs             map[string]uint64
	sseActive              int64
	sseTotal               uint64
	graphNodes             map[graphNodeMetricKey]durationMetric
	graphNodeHistograms    map[graphNodeMetricKey]histogramMetric
	llmCalls               map[llmCallMetricKey]durationMetric
	llmCallHistograms      map[llmCallMetricKey]histogramMetric
	llmTokens              map[llmTokenMetricKey]uint64
	ragRetrieves           map[ragMetricKey]durationMetric
	ragRetrieveHistograms  map[ragMetricKey]histogramMetric
	ragCandidates          map[ragCandidateMetricKey]uint64
	ragEmpty               map[string]uint64
	ragFallback            map[string]uint64
	inflightRequests       map[string]int64
	backpressureRejections map[string]uint64
	breakerState           string
}

type httpMetricKey struct {
	method string
	path   string
	status int
}

type graphNodeMetricKey struct {
	node   string
	status string
}

type llmCallMetricKey struct {
	model    string
	errClass string
}

type llmTokenMetricKey struct {
	model     string
	tokenType string
}

type durationMetric struct {
	count uint64
	sum   time.Duration
}

type histogramMetric struct {
	count   uint64
	sum     time.Duration
	buckets []uint64
}

type ragMetricKey struct {
	status string
	source string
}

type ragCandidateMetricKey struct {
	stage  string
	source string
}

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}

func newMetricsRecorder() *metricsRecorder {
	return &metricsRecorder{
		httpRequests:           map[httpMetricKey]uint64{},
		httpDurations:          map[httpMetricKey]histogramMetric{},
		parserDocs:             map[string]uint64{},
		graphNodes:             map[graphNodeMetricKey]durationMetric{},
		graphNodeHistograms:    map[graphNodeMetricKey]histogramMetric{},
		llmCalls:               map[llmCallMetricKey]durationMetric{},
		llmCallHistograms:      map[llmCallMetricKey]histogramMetric{},
		llmTokens:              map[llmTokenMetricKey]uint64{},
		ragRetrieves:           map[ragMetricKey]durationMetric{},
		ragRetrieveHistograms:  map[ragMetricKey]histogramMetric{},
		ragCandidates:          map[ragCandidateMetricKey]uint64{},
		ragEmpty:               map[string]uint64{},
		ragFallback:            map[string]uint64{},
		inflightRequests:       map[string]int64{},
		backpressureRejections: map[string]uint64{},
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

func (m *metricsRecorder) recordHTTPRequest(method, path string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := httpMetricKey{method: method, path: path, status: status}
	m.httpRequests[key]++
	m.httpDurations[key] = observeHistogram(m.httpDurations[key], duration)
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

func (m *metricsRecorder) recordGraphNode(node, status string, duration time.Duration) {
	if m == nil {
		return
	}
	if node == "" {
		node = "unknown"
	}
	if status == "" {
		status = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := graphNodeMetricKey{node: node, status: status}
	metric := m.graphNodes[key]
	metric.count++
	metric.sum += duration
	m.graphNodes[key] = metric
	m.graphNodeHistograms[key] = observeHistogram(m.graphNodeHistograms[key], duration)
}

func (m *metricsRecorder) recordLLMCall(model, errClass string, duration time.Duration, promptTokens, completionTokens int) {
	if m == nil {
		return
	}
	if model == "" {
		model = "unknown"
	}
	if errClass == "" {
		errClass = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	callKey := llmCallMetricKey{model: model, errClass: errClass}
	callMetric := m.llmCalls[callKey]
	callMetric.count++
	callMetric.sum += duration
	m.llmCalls[callKey] = callMetric
	m.llmCallHistograms[callKey] = observeHistogram(m.llmCallHistograms[callKey], duration)
	if promptTokens > 0 {
		m.llmTokens[llmTokenMetricKey{model: model, tokenType: "prompt"}] += uint64(promptTokens)
	}
	if completionTokens > 0 {
		m.llmTokens[llmTokenMetricKey{model: model, tokenType: "completion"}] += uint64(completionTokens)
	}
}

func (m *metricsRecorder) recordRAGRetrieve(source, status string, duration time.Duration, candidates int) {
	if m == nil {
		return
	}
	if source == "" {
		source = "unknown"
	}
	if status == "" {
		status = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ragMetricKey{source: source, status: status}
	metric := m.ragRetrieves[key]
	metric.count++
	metric.sum += duration
	m.ragRetrieves[key] = metric
	m.ragRetrieveHistograms[key] = observeHistogram(m.ragRetrieveHistograms[key], duration)
	m.ragCandidates[ragCandidateMetricKey{source: source, stage: "final"}] += uint64(max(candidates, 0))
	if candidates == 0 && status == "ok" {
		m.ragEmpty[source]++
	}
	if status != "ok" {
		m.ragFallback[status]++
	}
}

func (m *metricsRecorder) beginInFlight(scope string) func() {
	if m == nil {
		return func() {}
	}
	if scope == "" {
		scope = "unknown"
	}
	m.mu.Lock()
	m.inflightRequests[scope]++
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		if m.inflightRequests[scope] > 0 {
			m.inflightRequests[scope]--
		}
		m.mu.Unlock()
	}
}

func (m *metricsRecorder) recordBackpressureRejection(scope string) {
	if m == nil {
		return
	}
	if scope == "" {
		scope = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backpressureRejections[scope]++
}

func (m *metricsRecorder) setBreakerState(state string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.breakerState = state
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

func (s *Server) metrics(c *gin.Context) {
	if s.breakerState != nil {
		s.metricsRecorder.setBreakerState(s.breakerState())
	}
	var eventHub EventHubMetrics
	if s.eventHubMetrics != nil {
		eventHub = s.eventHubMetrics()
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(s.metricsRecorder.renderPrometheusWithEventHub(eventHub)))
}
