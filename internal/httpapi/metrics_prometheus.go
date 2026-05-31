package httpapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (m *metricsRecorder) renderPrometheus() string {
	return m.renderPrometheusWithEventHub(EventHubMetrics{})
}

func (m *metricsRecorder) renderPrometheusWithEventHub(eventHub EventHubMetrics) string {
	if m == nil {
		m = newMetricsRecorder()
	}
	m.mu.Lock()
	httpRequests := make(map[httpMetricKey]uint64, len(m.httpRequests))
	for k, v := range m.httpRequests {
		httpRequests[k] = v
	}
	httpDurations := make(map[httpMetricKey]histogramMetric, len(m.httpDurations))
	for k, v := range m.httpDurations {
		httpDurations[k] = v
	}
	parserDocs := make(map[string]uint64, len(m.parserDocs))
	for k, v := range m.parserDocs {
		parserDocs[k] = v
	}
	graphNodes := make(map[graphNodeMetricKey]durationMetric, len(m.graphNodes))
	for k, v := range m.graphNodes {
		graphNodes[k] = v
	}
	graphNodeHistograms := make(map[graphNodeMetricKey]histogramMetric, len(m.graphNodeHistograms))
	for k, v := range m.graphNodeHistograms {
		graphNodeHistograms[k] = v
	}
	llmCalls := make(map[llmCallMetricKey]durationMetric, len(m.llmCalls))
	for k, v := range m.llmCalls {
		llmCalls[k] = v
	}
	llmCallHistograms := make(map[llmCallMetricKey]histogramMetric, len(m.llmCallHistograms))
	for k, v := range m.llmCallHistograms {
		llmCallHistograms[k] = v
	}
	llmTokens := make(map[llmTokenMetricKey]uint64, len(m.llmTokens))
	for k, v := range m.llmTokens {
		llmTokens[k] = v
	}
	ragRetrieves := make(map[ragMetricKey]durationMetric, len(m.ragRetrieves))
	for k, v := range m.ragRetrieves {
		ragRetrieves[k] = v
	}
	ragRetrieveHistograms := make(map[ragMetricKey]histogramMetric, len(m.ragRetrieveHistograms))
	for k, v := range m.ragRetrieveHistograms {
		ragRetrieveHistograms[k] = v
	}
	ragCandidates := make(map[ragCandidateMetricKey]uint64, len(m.ragCandidates))
	for k, v := range m.ragCandidates {
		ragCandidates[k] = v
	}
	ragEmpty := make(map[string]uint64, len(m.ragEmpty))
	for k, v := range m.ragEmpty {
		ragEmpty[k] = v
	}
	ragFallback := make(map[string]uint64, len(m.ragFallback))
	for k, v := range m.ragFallback {
		ragFallback[k] = v
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
	b.WriteString("# HELP interview_http_request_duration_seconds HTTP request duration in seconds.\n")
	b.WriteString("# TYPE interview_http_request_duration_seconds histogram\n")
	for _, k := range keys {
		writeHistogram(&b, "interview_http_request_duration_seconds",
			map[string]string{"method": k.method, "path": k.path, "status": strconv.Itoa(k.status)},
			httpDurations[k])
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
	_ = graphNodes
	b.WriteString("# HELP interview_graph_node_duration_seconds Graph node execution duration histogram in seconds.\n")
	b.WriteString("# TYPE interview_graph_node_duration_seconds histogram\n")
	for _, k := range graphKeys {
		writeHistogram(&b, "interview_graph_node_duration_seconds",
			map[string]string{"node": k.node, "status": k.status},
			graphNodeHistograms[k])
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
	_ = llmCalls
	b.WriteString("# HELP interview_llm_call_duration_seconds LLM call duration histogram in seconds.\n")
	b.WriteString("# TYPE interview_llm_call_duration_seconds histogram\n")
	for _, k := range llmCallKeys {
		writeHistogram(&b, "interview_llm_call_duration_seconds",
			map[string]string{"model": k.model, "err_class": k.errClass},
			llmCallHistograms[k])
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

	writeRAGMetrics(&b, ragRetrieves, ragRetrieveHistograms, ragCandidates, ragEmpty, ragFallback)
	writeEventHubMetrics(&b, eventHub)

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
