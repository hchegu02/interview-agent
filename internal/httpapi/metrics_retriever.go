package httpapi

import (
	"context"
	"time"

	"interview-agent/internal/retriever"
)

type metricsRetriever struct {
	inner    retriever.Retriever
	recorder *metricsRecorder
	source   string
}

func NewMetricsRetriever(inner retriever.Retriever, recorder *metricsRecorder, source string) retriever.Retriever {
	if inner == nil || recorder == nil {
		return inner
	}
	if source == "" {
		source = "unknown"
	}
	return metricsRetriever{inner: inner, recorder: recorder, source: source}
}

func (r metricsRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	started := time.Now()
	results, err := r.inner.Retrieve(ctx, q)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.recorder.recordRAGRetrieve(r.source, status, time.Since(started), len(results))
	return results, err
}
