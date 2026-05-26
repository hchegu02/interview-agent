// Package traceid 提供全链路追踪 ID。
//
// 设计：trace_id 通过 context 传播，并自动加入到 slog 输出。
// 业务代码不需要关心 trace_id，只需在 HTTP 中间件入口生成 / 承接，
// 之后所有 slog 日志、Redis key、LLM 请求 header 都会自动带上。
//
// 面试要点：一个 trace_id 能串起一次面试经过的所有 5 个节点的日志。
package traceid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey struct{}

// New 生成一个 16 字节 hex（32 字符）的 trace id。
func New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Inject 把 id 放入 context。
func Inject(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext 取出 id，未设置时返回空串。
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
