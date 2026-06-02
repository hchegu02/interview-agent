package observability

import (
	"context"
	"log/slog"
	"sync"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

type tracingGraphCallback struct {
	tracer Tracer

	mu       sync.Mutex
	inflight map[string]SpanEnd
}

func NewTracingGraphCallback(tracer Tracer) graph.Callback {
	if tracer == nil {
		tracer = NoopTracer{}
	}
	return &tracingGraphCallback{
		tracer:   tracer,
		inflight: map[string]SpanEnd{},
	}
}

func (c *tracingGraphCallback) OnNodeStart(ctx context.Context, node string, sess *domain.Session) {
	sessionID := sessionID(sess)
	_, end := c.tracer.Start(ctx, "graph.node", map[string]string{
		"node":       node,
		"session_id": sessionID,
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	// tracing 和 metrics 使用同一类 span key：session_id + node。
	// 这是为了支持多会话并发；只按 node 记录会导致后结束的节点关闭错 span。
	c.inflight[spanKey(sessionID, node)] = end
}

func (c *tracingGraphCallback) OnNodeEnd(ctx context.Context, node string, sess *domain.Session) {
	c.complete(ctx, node, sess, nil)
}

func (c *tracingGraphCallback) OnNodeError(ctx context.Context, node string, sess *domain.Session, err error) {
	c.complete(ctx, node, sess, err)
}

func (c *tracingGraphCallback) complete(ctx context.Context, node string, sess *domain.Session, err error) {
	sessionID := sessionID(sess)
	key := spanKey(sessionID, node)
	c.mu.Lock()
	end := c.inflight[key]
	delete(c.inflight, key)
	c.mu.Unlock()
	if end != nil {
		// end 接收 err，由具体 tracer 决定如何标记 span 状态。
		// 当前默认 NoopTracer 不做外部上报，但调用边界已经固定。
		end(err)
		return
	}
	slog.WarnContext(ctx, "tracing callback: node completed without matching start",
		"event", "node_unmatched_end",
		"node", node,
		"session_id", sessionID,
	)
}

func sessionID(sess *domain.Session) string {
	if sess == nil {
		return ""
	}
	return sess.ID
}

func spanKey(sessionID, node string) string {
	return sessionID + "\x00" + node
}
