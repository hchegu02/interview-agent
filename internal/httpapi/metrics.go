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

	httpRequests           map[httpMetricKey]uint64
	parserDocs             map[string]uint64
	sseActive              int64
	sseTotal               uint64
	graphNodes             map[graphNodeMetricKey]durationMetric
	llmCalls               map[llmCallMetricKey]durationMetric
	llmTokens              map[llmTokenMetricKey]uint64
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

func newMetricsRecorder() *metricsRecorder {
	return &metricsRecorder{
		httpRequests:           map[httpMetricKey]uint64{},
		parserDocs:             map[string]uint64{},
		graphNodes:             map[graphNodeMetricKey]durationMetric{},
		llmCalls:               map[llmCallMetricKey]durationMetric{},
		llmTokens:              map[llmTokenMetricKey]uint64{},
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
	if promptTokens > 0 {
		m.llmTokens[llmTokenMetricKey{model: model, tokenType: "prompt"}] += uint64(promptTokens)
	}
	if completionTokens > 0 {
		m.llmTokens[llmTokenMetricKey{model: model, tokenType: "completion"}] += uint64(completionTokens)
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
	graphNodes := make(map[graphNodeMetricKey]durationMetric, len(m.graphNodes))
	for k, v := range m.graphNodes {
		graphNodes[k] = v
	}
	llmCalls := make(map[llmCallMetricKey]durationMetric, len(m.llmCalls))
	for k, v := range m.llmCalls {
		llmCalls[k] = v
	}
	llmTokens := make(map[llmTokenMetricKey]uint64, len(m.llmTokens))
	for k, v := range m.llmTokens {
		llmTokens[k] = v
	}
	inflightRequests := make(map[string]int64, len(m.inflightRequests))
	for k, v := range m.inflightRequests {
		inflightRequests[k] = v
	}
	backpressureRejections := make(map[string]uint64, len(m.backpressureRejections))
	for k, v := range m.backpressureRejections {
		backpressureRejections[k] = v
	}
	sseActive := m.sseActive
	sseTotal := m.sseTotal
	breakerState := m.breakerState
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

	b.WriteString("# HELP interview_graph_node_total Total graph node executions by node and status.\n")
	b.WriteString("# TYPE interview_graph_node_total counter\n")
	graphKeys := make([]graphNodeMetricKey, 0, len(graphNodes))
	for k := range graphNodes {
		graphKeys = append(graphKeys, k)
	}
	sort.Slice(graphKeys, func(i, j int) bool {
		if graphKeys[i].node != graphKeys[j].node {
			return graphKeys[i].node < graphKeys[j].node
		}
		return graphKeys[i].status < graphKeys[j].status
	})
	for _, k := range graphKeys {
		fmt.Fprintf(&b, "interview_graph_node_total{node=%q,status=%q} %d\n", k.node, k.status, graphNodes[k].count)
	}
	b.WriteString("# HELP interview_graph_node_duration_seconds Graph node execution duration in seconds.\n")
	b.WriteString("# TYPE interview_graph_node_duration_seconds summary\n")
	for _, k := range graphKeys {
		metric := graphNodes[k]
		fmt.Fprintf(&b, "interview_graph_node_duration_seconds_count{node=%q,status=%q} %d\n", k.node, k.status, metric.count)
		fmt.Fprintf(&b, "interview_graph_node_duration_seconds_sum{node=%q,status=%q} %.6f\n", k.node, k.status, metric.sum.Seconds())
	}

	b.WriteString("# HELP interview_llm_calls_total Total LLM calls by model and error class.\n")
	b.WriteString("# TYPE interview_llm_calls_total counter\n")
	llmCallKeys := make([]llmCallMetricKey, 0, len(llmCalls))
	for k := range llmCalls {
		llmCallKeys = append(llmCallKeys, k)
	}
	sort.Slice(llmCallKeys, func(i, j int) bool {
		if llmCallKeys[i].model != llmCallKeys[j].model {
			return llmCallKeys[i].model < llmCallKeys[j].model
		}
		return llmCallKeys[i].errClass < llmCallKeys[j].errClass
	})
	for _, k := range llmCallKeys {
		fmt.Fprintf(&b, "interview_llm_calls_total{model=%q,err_class=%q} %d\n", k.model, k.errClass, llmCalls[k].count)
	}
	b.WriteString("# HELP interview_llm_call_duration_seconds LLM call duration in seconds.\n")
	b.WriteString("# TYPE interview_llm_call_duration_seconds summary\n")
	for _, k := range llmCallKeys {
		metric := llmCalls[k]
		fmt.Fprintf(&b, "interview_llm_call_duration_seconds_count{model=%q,err_class=%q} %d\n", k.model, k.errClass, metric.count)
		fmt.Fprintf(&b, "interview_llm_call_duration_seconds_sum{model=%q,err_class=%q} %.6f\n", k.model, k.errClass, metric.sum.Seconds())
	}
	b.WriteString("# HELP interview_llm_tokens_total Total LLM tokens by model and token type.\n")
	b.WriteString("# TYPE interview_llm_tokens_total counter\n")
	tokenKeys := make([]llmTokenMetricKey, 0, len(llmTokens))
	for k := range llmTokens {
		tokenKeys = append(tokenKeys, k)
	}
	sort.Slice(tokenKeys, func(i, j int) bool {
		if tokenKeys[i].model != tokenKeys[j].model {
			return tokenKeys[i].model < tokenKeys[j].model
		}
		return tokenKeys[i].tokenType < tokenKeys[j].tokenType
	})
	for _, k := range tokenKeys {
		fmt.Fprintf(&b, "interview_llm_tokens_total{model=%q,type=%q} %d\n", k.model, k.tokenType, llmTokens[k])
	}

	b.WriteString("# HELP interview_inflight_requests Active requests inside backpressure-protected scopes.\n")
	b.WriteString("# TYPE interview_inflight_requests gauge\n")
	inflightScopes := sortedStringKeys(inflightRequests)
	for _, scope := range inflightScopes {
		fmt.Fprintf(&b, "interview_inflight_requests{scope=%q} %d\n", scope, inflightRequests[scope])
	}
	b.WriteString("# HELP interview_backpressure_rejections_total Total requests rejected by backpressure.\n")
	b.WriteString("# TYPE interview_backpressure_rejections_total counter\n")
	rejectionScopes := sortedStringKeys(backpressureRejections)
	for _, scope := range rejectionScopes {
		fmt.Fprintf(&b, "interview_backpressure_rejections_total{scope=%q} %d\n", scope, backpressureRejections[scope])
	}
	if breakerState != "" {
		b.WriteString("# HELP interview_llm_breaker_state LLM breaker state gauge, 1 for current state.\n")
		b.WriteString("# TYPE interview_llm_breaker_state gauge\n")
		for _, state := range []string{"closed", "open", "half_open"} {
			value := 0
			if state == breakerState {
				value = 1
			}
			fmt.Fprintf(&b, "interview_llm_breaker_state{state=%q} %d\n", state, value)
		}
	}
	return b.String()
}

func (s *Server) metrics(c *gin.Context) {
	if s.breakerState != nil {
		s.metricsRecorder.setBreakerState(s.breakerState())
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(s.metricsRecorder.renderPrometheus()))
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
