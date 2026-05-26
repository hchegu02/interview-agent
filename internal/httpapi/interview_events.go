package httpapi

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

const (
	interviewEventSnapshot         = "snapshot"
	interviewEventSessionCreated   = "session.created"
	interviewEventSessionUpdated   = "session.updated"
	interviewEventSessionCompleted = "session.completed"
	interviewEventSessionFailed    = "session.failed"
	interviewEventNodeStart        = "graph.node.start"
	interviewEventNodeEnd          = "graph.node.end"
	interviewEventNodeError        = "graph.node.error"
)

type InterviewEvent struct {
	ID          string           `json:"id,omitempty"`
	Type        string           `json:"type"`
	SessionID   string           `json:"session_id"`
	UserID      string           `json:"user_id,omitempty"`
	Node        string           `json:"node,omitempty"`
	Status      string           `json:"status,omitempty"`
	CurrentNode string           `json:"current_node,omitempty"`
	Question    *domain.Question `json:"question,omitempty"`
	Report      *domain.Report   `json:"report,omitempty"`
	Error       string           `json:"error,omitempty"`
	CreatedAt   time.Time        `json:"created_at,omitempty"`
	UpdatedAt   time.Time        `json:"updated_at,omitempty"`
	At          time.Time        `json:"at"`
}

type InterviewEventPublisher interface {
	Publish(ctx context.Context, event InterviewEvent) InterviewEvent
}

type InterviewEventHub interface {
	InterviewEventPublisher
	Subscribe(ctx context.Context, sessionID string, afterID string) (<-chan InterviewEvent, func(), error)
	Close() error
}

type InterviewEventHubStats struct {
	DroppedEvents uint64
}

type MemoryInterviewEventHub struct {
	mu            sync.RWMutex
	subs          map[string]map[chan InterviewEvent]struct{}
	history       map[string][]InterviewEvent
	buffer        int
	nextID        uint64
	droppedEvents uint64
	closed        bool
}

func NewMemoryInterviewEventHub(buffer int) *MemoryInterviewEventHub {
	if buffer <= 0 {
		buffer = 32
	}
	return &MemoryInterviewEventHub{
		subs:    map[string]map[chan InterviewEvent]struct{}{},
		history: map[string][]InterviewEvent{},
		buffer:  buffer,
	}
}

func (h *MemoryInterviewEventHub) Publish(ctx context.Context, event InterviewEvent) InterviewEvent {
	if h == nil || event.SessionID == "" {
		return event
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return event
		}
	}
	event = h.withMeta(event)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return event
	}
	h.history[event.SessionID] = appendEventHistory(h.history[event.SessionID], event, h.buffer)
	for ch := range h.subs[event.SessionID] {
		select {
		case ch <- event:
		default:
			h.droppedEvents++
		}
	}
	h.mu.Unlock()
	return event
}

func (h *MemoryInterviewEventHub) Subscribe(ctx context.Context, sessionID string, afterID string) (<-chan InterviewEvent, func(), error) {
	buffer := 32
	if h != nil && h.buffer > 0 {
		buffer = h.buffer
	}
	ch := make(chan InterviewEvent, buffer)
	if h == nil || sessionID == "" {
		return ch, func() {}, nil
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ch, func() {}, fmt.Errorf("event hub closed")
	}
	if h.subs[sessionID] == nil {
		h.subs[sessionID] = map[chan InterviewEvent]struct{}{}
	}
	replay := replayAfterID(h.history[sessionID], afterID)
	for _, event := range replay {
		select {
		case ch <- event:
		default:
		}
	}
	h.subs[sessionID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			if subs, ok := h.subs[sessionID]; ok {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(h.subs, sessionID)
				}
			}
			h.mu.Unlock()
		})
	}

	if ctx != nil {
		go func() {
			<-ctx.Done()
			unsubscribe()
		}()
	}

	return ch, unsubscribe, nil
}

func (h *MemoryInterviewEventHub) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	for sessionID, subs := range h.subs {
		for ch := range subs {
			close(ch)
		}
		delete(h.subs, sessionID)
	}
	return nil
}

func (h *MemoryInterviewEventHub) Stats() InterviewEventHubStats {
	if h == nil {
		return InterviewEventHubStats{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return InterviewEventHubStats{DroppedEvents: h.droppedEvents}
}

func (h *MemoryInterviewEventHub) withMeta(event InterviewEvent) InterviewEvent {
	event.At = time.Now()
	event.ID = fmt.Sprintf("evt-%d", atomic.AddUint64(&h.nextID, 1))
	return event
}

func appendEventHistory(history []InterviewEvent, event InterviewEvent, limit int) []InterviewEvent {
	history = append(history, event)
	if limit > 0 && len(history) > limit {
		history = history[len(history)-limit:]
	}
	return history
}

func replayAfterID(history []InterviewEvent, afterID string) []InterviewEvent {
	if afterID == "" {
		return nil
	}
	start := 0
	for i := range history {
		if history[i].ID == afterID {
			start = i + 1
			break
		}
	}
	if start >= len(history) {
		return nil
	}
	return append([]InterviewEvent(nil), history[start:]...)
}

func NewInterviewGraphCallback(pub InterviewEventPublisher) graph.Callback {
	if pub == nil {
		return noopInterviewGraphCallback{}
	}
	return interviewGraphCallback{pub: pub}
}

type interviewGraphCallback struct {
	pub InterviewEventPublisher
}

func (c interviewGraphCallback) OnNodeStart(ctx context.Context, node string, sess *domain.Session) {
	c.publish(ctx, interviewEventNodeStart, node, sess, "")
}

func (c interviewGraphCallback) OnNodeEnd(ctx context.Context, node string, sess *domain.Session) {
	c.publish(ctx, interviewEventNodeEnd, node, sess, "")
}

func (c interviewGraphCallback) OnNodeError(ctx context.Context, node string, sess *domain.Session, err error) {
	c.publish(ctx, interviewEventNodeError, node, sess, err.Error())
}

func (c interviewGraphCallback) publish(ctx context.Context, eventType, node string, sess *domain.Session, errMsg string) {
	if c.pub == nil {
		return
	}
	c.pub.Publish(ctx, buildInterviewEvent(eventType, sess, node, errMsg))
}

type noopInterviewGraphCallback struct{}

func (noopInterviewGraphCallback) OnNodeStart(context.Context, string, *domain.Session)        {}
func (noopInterviewGraphCallback) OnNodeEnd(context.Context, string, *domain.Session)          {}
func (noopInterviewGraphCallback) OnNodeError(context.Context, string, *domain.Session, error) {}

func buildInterviewEvent(eventType string, sess *domain.Session, node, errMsg string) InterviewEvent {
	ev := InterviewEvent{
		Type:  eventType,
		Node:  node,
		Error: errMsg,
		At:    time.Now(),
	}
	if sess == nil {
		return ev
	}
	ev.SessionID = sess.ID
	ev.UserID = sess.UserID
	ev.Status = string(sess.Status)
	ev.CurrentNode = sess.CurrentNode
	ev.Question = cloneQuestion(currentQuestion(sess))
	ev.Report = cloneReport(sess.Report)
	ev.CreatedAt = sess.CreatedAt
	ev.UpdatedAt = sess.UpdatedAt
	return ev
}

func cloneQuestion(q *domain.Question) *domain.Question {
	if q == nil {
		return nil
	}
	out := *q
	out.Tags = append([]string(nil), q.Tags...)
	out.ExpectedPoints = append([]string(nil), q.ExpectedPoints...)
	return &out
}

func cloneReport(r *domain.Report) *domain.Report {
	if r == nil {
		return nil
	}
	out := *r
	out.SkillBreakdown = make(map[string]int, len(r.SkillBreakdown))
	for k, v := range r.SkillBreakdown {
		out.SkillBreakdown[k] = v
	}
	out.Highlights = append([]string(nil), r.Highlights...)
	out.Improvements = append([]string(nil), r.Improvements...)
	out.NextSteps = append([]string(nil), r.NextSteps...)
	return &out
}
