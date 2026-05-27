package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/domain"
)

const interviewSSEHeartbeatInterval = 15 * time.Second

func (s *Server) streamInterview(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	sess, err := s.interview.Get(c.Request.Context(), sessionID)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	if userID := c.Query("user_id"); userID != "" && sess.UserID != userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("session %q not found", sessionID)})
		return
	}

	lastEventID := c.GetHeader("Last-Event-ID")
	if lastEventID == "" {
		lastEventID = c.Query("last_event_id")
	}
	events, unsubscribe, err := s.interview.Subscribe(c.Request.Context(), sessionID, lastEventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer unsubscribe()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	endMetrics := s.metricsRecorder.beginSSEConnection()
	defer endMetrics()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	if err := writeInterviewSSE(c.Writer, buildInterviewEvent(interviewEventSnapshot, sess, "", "")); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(interviewSSEHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if err := writeInterviewSSEComment(c.Writer, "ping"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := writeInterviewSSE(c.Writer, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeInterviewSSE(w http.ResponseWriter, event InterviewEvent) error {
	if event.Type == "" {
		event.Type = interviewEventSnapshot
	}
	publicEvent := buildInterviewStreamEvent(event)
	raw, err := json.Marshal(publicEvent)
	if err != nil {
		return err
	}
	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", publicEvent.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return nil
}

type interviewStreamEvent struct {
	ID        string                  `json:"id,omitempty"`
	Type      string                  `json:"type"`
	SessionID string                  `json:"session_id"`
	UserID    string                  `json:"user_id,omitempty"`
	Mode      string                  `json:"mode,omitempty"`
	Status    string                  `json:"status,omitempty"`
	Phase     string                  `json:"phase,omitempty"`
	Progress  []interviewProgressStep `json:"progress,omitempty"`
	Question  *interviewQuestion      `json:"question,omitempty"`
	Rounds    []interviewRound        `json:"rounds,omitempty"`
	Report    *domain.Report          `json:"report,omitempty"`
	Error     string                  `json:"error,omitempty"`
	CreatedAt time.Time               `json:"created_at,omitempty"`
	UpdatedAt time.Time               `json:"updated_at,omitempty"`
	At        time.Time               `json:"at"`
}

func buildInterviewStreamEvent(event InterviewEvent) interviewStreamEvent {
	eventType := publicInterviewEventType(event.Type)
	out := interviewStreamEvent{
		ID:        event.ID,
		Type:      eventType,
		SessionID: event.SessionID,
		UserID:    event.UserID,
		Mode:      normalizeInterviewMode(event.Mode),
		Status:    event.Status,
		Phase:     event.Phase,
		Progress:  event.Progress,
		Question:  buildInterviewQuestion(event.Question, false),
		Rounds:    event.Rounds,
		Report:    cloneReport(event.Report),
		CreatedAt: event.CreatedAt,
		UpdatedAt: event.UpdatedAt,
		At:        event.At,
	}
	if out.Phase == "" {
		out.Phase = "preparing"
	}
	if event.Type == interviewEventSessionFailed || event.Type == interviewEventNodeError {
		out.Error = "面试流程暂时无法继续，请稍后重试"
	}
	return out
}

func publicInterviewEventType(eventType string) string {
	switch eventType {
	case interviewEventSnapshot:
		return "snapshot"
	case interviewEventSessionCompleted:
		return "interview.completed"
	case interviewEventSessionFailed, interviewEventNodeError:
		return "interview.failed"
	default:
		return "interview.progress"
	}
}

func writeInterviewSSEComment(w http.ResponseWriter, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	return nil
}
