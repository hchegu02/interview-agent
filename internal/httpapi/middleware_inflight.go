package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// inflightRetryAfter 与 lease 冲突的 Retry-After 协议保持一致——
// 客户端只需统一处理一种"稍后重试"语义。
const inflightRetryAfter = time.Second

// MaxInFlightMiddleware 是最外层入口背压：
// 用 buffered chan 当信号量，超过 limit 时**非阻塞**直接 503，不排队。
//
// 设计点：
//   - **非阻塞 try-acquire**：避免让客户端 HTTP 连接挂在 server 侧排队，
//     超过容量立刻拒绝，让上游做长重试（client retry / k8s queue / 浏览器重发）。
//   - **复用 lease 冲突的 Retry-After 协议**：header `Retry-After: 1`
//   - body `{"error":"server busy","retry_after_seconds":1}`，
//     客户端逻辑统一。
//   - **limit <= 0 时退化为 no-op**：单实例 dev / 测试不被打扰；
//     生产环境靠 config.validate 强校验 MaxInFlight > 0。
//   - **不影响 service 层错误**：直接在 middleware 内 c.AbortWithStatusJSON，
//     不经过 writeInterviewError，业务代码无感知。
//
// 作用范围由 router.go 控制：start/answer 和 SSE stream 使用不同实例；
// /healthz、/readyz、sessions 读路径不参与，保证健康检查/读路径即使在 LLM
// 路径压满时仍可用。
func MaxInFlightMiddleware(limit int) gin.HandlerFunc {
	if limit <= 0 {
		// 关闭背压：直接透传。
		return func(c *gin.Context) { c.Next() }
	}
	sem := make(chan struct{}, limit)
	return func(c *gin.Context) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			c.Next()
		default:
			retryAfterSeconds := int(inflightRetryAfter.Seconds())
			c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":               "server busy",
				"retry_after_seconds": retryAfterSeconds,
			})
		}
	}
}
