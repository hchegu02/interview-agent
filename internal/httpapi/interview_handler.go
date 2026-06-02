package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/domain"
)

type startInterviewRequest struct {
	SessionID          string                     `json:"session_id"`
	UserID             string                     `json:"user_id"`
	Mode               string                     `json:"mode"`
	JDText             string                     `json:"jd_text" binding:"required"`
	ResumeText         string                     `json:"resume_text" binding:"required"`
	QuestionBankFilter *domain.QuestionBankFilter `json:"question_bank_filter,omitempty"`
}

// answerInterviewRequest 只接收“当前等待输入”的答案。
// 答案具体写到主问题还是追问，由 InterviewService 根据 Session.CurrentNode 判断。
type answerInterviewRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	UserID    string `json:"user_id"`
	Answer    string `json:"answer"`
}

func (s *Server) startInterview(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	var req startInterviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// handler 不直接创建 Session。它只做 HTTP 协议层校验，
	// 会话初始化、Graph 首轮推进、持久化和事件发布都交给 InterviewService。
	sess, err := s.interview.Start(c.Request.Context(), req)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, buildInterviewResponse(sess))
}

func (s *Server) answerInterview(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	var req answerInterviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Answer 入口同样不碰底层 store，避免绕过 lease、snapshot 和单调 UpdatedAt 规则。
	sess, err := s.interview.Answer(c.Request.Context(), req)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, buildInterviewResponse(sess))
}

func (s *Server) listInterviewSessions(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		limit = n
	}
	// handler 先做“必须是正整数”的协议校验，再交给统一 limit 规则截断上限。
	limit = normalizeSessionListLimit(limit)
	sessions, err := s.interview.ListByUser(c.Request.Context(), userID, limit)
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	out := make([]interviewResponse, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, buildInterviewResponse(sess))
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

func (s *Server) getInterviewSession(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	sess, err := s.interview.GetForUser(c.Request.Context(), sessionID, c.Query("user_id"))
	if err != nil {
		writeInterviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, buildInterviewResponse(sess))
}

func (s *Server) deleteInterviewSession(c *gin.Context) {
	if s.interview == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "interview service not configured"})
		return
	}
	sessionID := c.Param("session_id")
	userID := c.Query("user_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	if err := s.interview.DeleteForUser(c.Request.Context(), sessionID, userID); err != nil {
		writeInterviewError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
