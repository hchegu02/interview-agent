package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/memory"
)

type userMemoryResponse struct {
	UserID      string             `json:"user_id"`
	Strengths   []string           `json:"strengths,omitempty"`
	Weaknesses  []userWeakness     `json:"weaknesses,omitempty"`
	SkillScores map[string]float64 `json:"skill_scores,omitempty"`
	LastAdvice  []string           `json:"last_advice,omitempty"`
	UpdatedAt   time.Time          `json:"updated_at,omitempty"`
}

type userWeakness struct {
	Topic     string    `json:"topic"`
	Evidence  string    `json:"evidence,omitempty"`
	Severity  int       `json:"severity,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func (s *Server) getUserMemory(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	userID := strings.TrimSpace(c.Param("user_id"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	mem, err := s.interview.GetUserMemory(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, memory.ErrUserMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user memory not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get user memory failed"})
		return
	}
	c.JSON(http.StatusOK, buildUserMemoryResponse(mem))
}

func (s *InterviewService) GetUserMemory(ctx context.Context, userID string) (*memory.UserMemory, error) {
	if s == nil || s.memoryStore == nil {
		return nil, memory.ErrUserMemoryNotFound
	}
	return s.memoryStore.GetUserMemory(ctx, userID)
}

func buildUserMemoryResponse(mem *memory.UserMemory) userMemoryResponse {
	if mem == nil {
		return userMemoryResponse{}
	}
	out := userMemoryResponse{
		UserID:      mem.UserID,
		Strengths:   append([]string(nil), mem.Strengths...),
		SkillScores: map[string]float64{},
		LastAdvice:  append([]string(nil), mem.LastAdvice...),
		UpdatedAt:   mem.UpdatedAt,
	}
	for skill, score := range mem.SkillScores {
		out.SkillScores[skill] = score
	}
	if len(mem.Weaknesses) > 0 {
		out.Weaknesses = make([]userWeakness, 0, len(mem.Weaknesses))
		for _, item := range mem.Weaknesses {
			out.Weaknesses = append(out.Weaknesses, userWeakness{
				Topic:     item.Topic,
				Evidence:  item.Evidence,
				Severity:  item.Severity,
				UpdatedAt: item.UpdatedAt,
			})
		}
	}
	return out
}
