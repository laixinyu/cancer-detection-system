package observability

// 文件： internal/observability/context.go
// 用途：跨传输链路关联字段的上下文键定义。

import (
	"context"
	"strings"
)

type ctxKey string

const requestIDContextKey ctxKey = "request_id"

func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v := ctx.Value(requestIDContextKey)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
