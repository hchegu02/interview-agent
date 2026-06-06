package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/graph"
)

const (
	interviewErrorSessionNotFound = "session_not_found"
	interviewErrorInvalidState    = "invalid_state"
	interviewErrorLeaseConflict   = "lease_conflict"
	interviewErrorInvalidConfig   = "invalid_config"
	interviewErrorInternal        = "internal_error"
	interviewErrorStaleWrite      = "stale_session_write"
)

func writeInterviewError(c *gin.Context, err error) {
	traceID := ""
	if c != nil && c.Request != nil {
		traceID = traceIDFromContext(c.Request.Context())
	}
	switch {
	case err == nil:
		c.Status(http.StatusOK)
	case errors.Is(err, graph.ErrInvalidConfig):
		c.JSON(http.StatusInternalServerError, interviewErrorBody(interviewErrorInvalidConfig, "面试服务暂不可用，请稍后重试", traceID))
	case errors.Is(err, ErrSessionLeaseConflict):
		retryAfterSeconds := int(sessionLeaseRetryAfter.Seconds())
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		body := interviewErrorBody(interviewErrorLeaseConflict, "当前会话正在处理中，请稍后重试", traceID)
		body["retry_after_seconds"] = retryAfterSeconds
		c.JSON(http.StatusConflict, body)
	case errors.Is(err, ErrStaleSessionWrite):
		c.JSON(http.StatusConflict, interviewErrorBody(interviewErrorStaleWrite, "会话状态已被其他请求更新，请重新拉取后再试", traceID))
	case errors.Is(err, ErrSessionNotFound):
		c.JSON(http.StatusBadRequest, interviewErrorBody(interviewErrorSessionNotFound, "会话不存在或无权访问", traceID))
	case errors.Is(err, ErrInvalidSessionState):
		c.JSON(http.StatusBadRequest, interviewErrorBody(interviewErrorInvalidState, "当前会话状态不允许该操作", traceID))
	default:
		c.JSON(http.StatusBadRequest, interviewErrorBody(interviewErrorInternal, "请求无法处理，请检查会话状态后重试", traceID))
	}
}

func interviewErrorBody(code, message, traceID string) gin.H {
	body := gin.H{
		"code":  code,
		"error": message,
	}
	if traceID != "" {
		body["trace_id"] = traceID
	}
	return body
}
