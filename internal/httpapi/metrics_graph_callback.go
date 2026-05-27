package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

type metricsGraphCallback struct {
	recorder *metricsRecorder
	now      func() time.Time

	mu       sync.Mutex
	inflight map[string]time.Time
}

func NewMetricsGraphCallback(recorder *metricsRecorder) graph.Callback {
	return &metricsGraphCallback{
		recorder: recorder,
		now:      time.Now,
		inflight: map[string]time.Time{},
	}
}

func (c *metricsGraphCallback) OnNodeStart(ctx context.Context, node string, _ *domain.Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.inflight[node]; ok {
		slog.WarnContext(ctx, "metrics callback: node started while previous in-flight",
			"event", "node_overlap",
			"node", node,
			"previous_started_at", old,
		)
	}
	c.inflight[node] = c.now()
}

func (c *metricsGraphCallback) OnNodeEnd(ctx context.Context, node string, _ *domain.Session) {
	c.complete(ctx, node, nil)
}

func (c *metricsGraphCallback) OnNodeError(ctx context.Context, node string, _ *domain.Session, err error) {
	c.complete(ctx, node, err)
}

func (c *metricsGraphCallback) complete(ctx context.Context, node string, err error) {
	c.mu.Lock()
	start, ok := c.inflight[node]
	if ok {
		delete(c.inflight, node)
	} else {
		slog.WarnContext(ctx, "metrics callback: node completed without matching start",
			"event", "node_unmatched_end",
			"node", node,
		)
		start = c.now()
	}
	duration := c.now().Sub(start)
	c.mu.Unlock()

	c.recorder.recordGraphNode(node, classifyGraphNodeErr(err), duration)
}

func classifyGraphNodeErr(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, graph.ErrSuspended):
		return "suspended"
	case errors.Is(err, graph.ErrPermanent):
		return "permanent"
	default:
		return llm.ClassifyChatErr(err)
	}
}
