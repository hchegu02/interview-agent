package httpapi

import (
	"github.com/gin-gonic/gin"

	"interview-agent/pkg/traceid"
)

// TraceIDMiddleware 在请求入口处生成或承接 X-Trace-Id。
//
// 设计：客户端可以传 X-Trace-Id 让分布式追踪贯穿网关/前端/后端；
// 没传就服务端生成。所有日志、Redis key、LLM header 都会带这个 id。
func TraceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Trace-Id")
		if id == "" {
			id = traceid.New()
		}
		c.Writer.Header().Set("X-Trace-Id", id)
		ctx := traceid.Inject(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
