package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/agent"
)

func (s *Server) agentMessage(c *gin.Context) {
	if s.agent == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "agent service not configured"})
		return
	}
	var req agent.AgentMessage
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := s.agent.HandleMessage(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, agent.ErrEmptyMessage) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
			return
		}
		var agentErr *agent.AgentError
		if errors.As(err, &agentErr) {
			c.JSON(http.StatusBadRequest, gin.H{"code": agentErr.Code, "error": agentErr.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handle agent message failed"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
