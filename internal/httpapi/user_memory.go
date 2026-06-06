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

const userMemoryOwnerHeader = "X-User-ID"

type UserMemoryOwner struct {
	UserID        string
	Authenticated bool
}

type UserMemoryOwnerResolver func(*http.Request) (UserMemoryOwner, error)

type UserMemoryAuthorizer func(owner UserMemoryOwner, targetUserID string) bool

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

func (s *Server) SetUserMemoryOwnerResolver(resolver UserMemoryOwnerResolver) {
	s.userMemoryOwnerResolver = resolver
}

func (s *Server) SetUserMemoryAuthorizer(authorizer UserMemoryAuthorizer) {
	s.userMemoryAuthorizer = authorizer
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
	owner, err := s.resolveUserMemoryOwner(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "resolve user memory owner failed", "code": "owner_resolve_failed"})
		return
	}
	if !s.authorizeUserMemory(owner, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "code": "user_memory_forbidden"})
		return
	}
	mem, err := s.interview.GetUserMemory(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, memory.ErrUserMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user memory not found", "code": "user_memory_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get user memory failed", "code": "user_memory_read_failed"})
		return
	}
	c.JSON(http.StatusOK, buildUserMemoryResponse(mem))
}

func (s *Server) resolveUserMemoryOwner(r *http.Request) (UserMemoryOwner, error) {
	if s.userMemoryOwnerResolver != nil {
		return s.userMemoryOwnerResolver(r)
	}
	return defaultUserMemoryOwnerResolver(r)
}

func defaultUserMemoryOwnerResolver(r *http.Request) (UserMemoryOwner, error) {
	userID := strings.TrimSpace(r.Header.Get(userMemoryOwnerHeader))
	if userID == "" {
		userID = strings.TrimSpace(r.URL.Query().Get("owner_user_id"))
	}
	if userID == "" {
		return UserMemoryOwner{}, errors.New("user memory owner is required")
	}
	return UserMemoryOwner{UserID: userID, Authenticated: true}, nil
}

func (s *Server) authorizeUserMemory(owner UserMemoryOwner, targetUserID string) bool {
	if s.userMemoryAuthorizer != nil {
		return s.userMemoryAuthorizer(owner, targetUserID)
	}
	return owner.Authenticated && strings.TrimSpace(owner.UserID) == targetUserID
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
