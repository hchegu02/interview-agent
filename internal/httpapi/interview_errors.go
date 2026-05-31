package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/graph"
)

func writeInterviewError(c *gin.Context, err error) {
	switch {
	case err == nil:
		c.Status(http.StatusOK)
	case errors.Is(err, graph.ErrInvalidConfig):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "面试服务暂不可用，请稍后重试"})
	case errors.Is(err, ErrSessionLeaseConflict):
		retryAfterSeconds := int(sessionLeaseRetryAfter.Seconds())
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		c.JSON(http.StatusConflict, gin.H{
			"error":               "当前会话正在处理中，请稍后重试",
			"retry_after_seconds": retryAfterSeconds,
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求无法处理，请检查会话状态后重试"})
	}
}
