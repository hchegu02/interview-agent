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

func (c *metricsGraphCallback) OnNodeStart(ctx context.Context, node string, sess *domain.Session) {
	sessionID := metricsSessionID(sess)
	key := metricsSpanKey(sessionID, node)
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.inflight[key]; ok {
		slog.WarnContext(ctx, "metrics callback: node started while previous in-flight",
			"event", "node_overlap",
			"node", node,
			"session_id", sessionID,
			"previous_started_at", old,
		)
	}
	c.inflight[key] = c.now()
}

func (c *metricsGraphCallback) OnNodeEnd(ctx context.Context, node string, sess *domain.Session) {
	c.complete(ctx, node, sess, nil)
}

func (c *metricsGraphCallback) OnNodeError(ctx context.Context, node string, sess *domain.Session, err error) {
	c.complete(ctx, node, sess, err)
}

func (c *metricsGraphCallback) complete(ctx context.Context, node string, sess *domain.Session, err error) {
	sessionID := metricsSessionID(sess)
	key := metricsSpanKey(sessionID, node)
	c.mu.Lock()
	start, ok := c.inflight[key]
	if ok {
		delete(c.inflight, key)
	} else {
		slog.WarnContext(ctx, "metrics callback: node completed without matching start",
			"event", "node_unmatched_end",
			"node", node,
			"session_id", sessionID,
		)
		start = c.now()
	}
	duration := c.now().Sub(start)
	c.mu.Unlock()

	c.recorder.recordGraphNode(node, classifyGraphNodeErr(err), duration)
}

func metricsSessionID(sess *domain.Session) string {
	if sess == nil {
		return ""
	}
	return sess.ID
}

func metricsSpanKey(sessionID, node string) string {
	return sessionID + "\x00" + node
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
