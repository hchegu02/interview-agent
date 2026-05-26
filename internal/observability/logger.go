// Package observability 集中处理日志、metrics、trace。
//
// 设计要点：
//   - 用 log/slog（Go 1.21+ 标准库），不引第三方
//   - 通过 ContextHandler 自动把 trace_id 注入每条日志
//   - JSON 格式输出，方便 ELK / Loki 采集
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"

	"interview-agent/pkg/traceid"
)

// NewLogger 返回一个把 trace_id 自动加入每条日志的 logger。
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	base := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(&traceHandler{Handler: base})
}

// traceHandler 在每条日志输出前从 ctx 取出 trace_id 并加为 attr。
type traceHandler struct {
	slog.Handler
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := traceid.FromContext(ctx); id != "" {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}
