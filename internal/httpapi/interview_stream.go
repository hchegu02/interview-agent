package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return nil
}

func writeInterviewSSEComment(w http.ResponseWriter, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	return nil
}
