package httpapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func observeHistogram(metric histogramMetric, duration time.Duration) histogramMetric {
	if metric.buckets == nil {
		metric.buckets = make([]uint64, len(durationBuckets))
	}
	metric.count++
	metric.sum += duration
	seconds := duration.Seconds()
	for i, bucket := range durationBuckets {
		if seconds <= bucket {
			metric.buckets[i]++
		}
	}
	return metric
}

func writeHistogram(b *strings.Builder, name string, labels map[string]string, metric histogramMetric) {
	for i, bucket := range durationBuckets {
		labelsWithBucket := cloneLabels(labels)
		labelsWithBucket["le"] = strconv.FormatFloat(bucket, 'g', -1, 64)
		fmt.Fprintf(b, "%s_bucket%s %d\n", name, renderLabels(labelsWithBucket), metric.bucketsAt(i))
	}
	labelsWithBucket := cloneLabels(labels)
	labelsWithBucket["le"] = "+Inf"
	fmt.Fprintf(b, "%s_bucket%s %d\n", name, renderLabels(labelsWithBucket), metric.count)
	fmt.Fprintf(b, "%s_count%s %d\n", name, renderLabels(labels), metric.count)
	fmt.Fprintf(b, "%s_sum%s %.6f\n", name, renderLabels(labels), metric.sum.Seconds())
}

func (m histogramMetric) bucketsAt(i int) uint64 {
	if i < 0 || i >= len(m.buckets) {
		return 0
	}
	return m.buckets[i]
}

func cloneLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	return out
}

func renderLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%q", k, labels[k])
	}
	b.WriteByte('}')
	return b.String()
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
