package httpapi

import (
	"fmt"
	"sort"
	"strings"
)

func writeRAGMetrics(b *strings.Builder, retrieves map[ragMetricKey]durationMetric, histograms map[ragMetricKey]histogramMetric, candidates map[ragCandidateMetricKey]uint64, empty map[string]uint64, fallback map[string]uint64) {
	b.WriteString("# HELP interview_rag_retrieve_total Total RAG retrieval calls.\n")
	b.WriteString("# TYPE interview_rag_retrieve_total counter\n")
	keys := make([]ragMetricKey, 0, len(retrieves))
	for k := range retrieves {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		return keys[i].status < keys[j].status
	})
	for _, k := range keys {
		fmt.Fprintf(b, "interview_rag_retrieve_total{source=%q,status=%q} %d\n", k.source, k.status, retrieves[k].count)
	}
	b.WriteString("# HELP interview_rag_retrieve_duration_seconds RAG retrieval duration in seconds.\n")
	b.WriteString("# TYPE interview_rag_retrieve_duration_seconds histogram\n")
	for _, k := range keys {
		writeHistogram(b, "interview_rag_retrieve_duration_seconds",
			map[string]string{"source": k.source, "status": k.status},
			histograms[k])
	}
	b.WriteString("# HELP interview_rag_candidates_total Total RAG candidates returned by stage.\n")
	b.WriteString("# TYPE interview_rag_candidates_total counter\n")
	candidateKeys := make([]ragCandidateMetricKey, 0, len(candidates))
	for k := range candidates {
		candidateKeys = append(candidateKeys, k)
	}
	sort.Slice(candidateKeys, func(i, j int) bool {
		if candidateKeys[i].source != candidateKeys[j].source {
			return candidateKeys[i].source < candidateKeys[j].source
		}
		return candidateKeys[i].stage < candidateKeys[j].stage
	})
	for _, k := range candidateKeys {
		fmt.Fprintf(b, "interview_rag_candidates_total{source=%q,stage=%q} %d\n", k.source, k.stage, candidates[k])
	}
	for _, source := range sortedStringKeys(empty) {
		fmt.Fprintf(b, "interview_rag_empty_total{source=%q} %d\n", source, empty[source])
	}
	for _, reason := range sortedStringKeys(fallback) {
		fmt.Fprintf(b, "interview_rag_fallback_total{reason=%q} %d\n", reason, fallback[reason])
	}
}

func writeEventHubMetrics(b *strings.Builder, metrics EventHubMetrics) {
	b.WriteString("# HELP interview_event_hub_publish_errors_total Total event hub publish errors.\n")
	b.WriteString("# TYPE interview_event_hub_publish_errors_total counter\n")
	fmt.Fprintf(b, "interview_event_hub_publish_errors_total %d\n", metrics.PublishErrors)
	b.WriteString("# HELP interview_event_hub_dropped_events_total Total event hub dropped events.\n")
	b.WriteString("# TYPE interview_event_hub_dropped_events_total counter\n")
	fmt.Fprintf(b, "interview_event_hub_dropped_events_total %d\n", metrics.DroppedEvents)
	if metrics.LastPublishError != "" {
		b.WriteString("# HELP interview_event_hub_last_publish_error_info Last event hub publish error.\n")
		b.WriteString("# TYPE interview_event_hub_last_publish_error_info gauge\n")
		fmt.Fprintf(b, "interview_event_hub_last_publish_error_info{error=%q} 1\n", metrics.LastPublishError)
	}
}
