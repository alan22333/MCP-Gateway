// Package middleware 提供 Gin 中间件
package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// contextKey 私有类型，防止与其他包的 key 冲突
type contextKey string

// TraceIDKey 在 context.Context 中存取 TraceID 的 key
const TraceIDKey contextKey = "trace_id"

// TraceID 中间件：为每个请求生成唯一的 TraceID
// 1. 优先使用客户端传入的 X-Request-Id header
// 2. 否则生成新的 UUID
// 3. 存入 Gin context (c.Set) 和 request context (context.WithValue)，供 service/proxy 层使用
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Request-Id")
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// 写入 Gin context（handler 层可通过 c.GetString 读取）
		c.Set("trace_id", traceID)

		// 写入 request context（service/proxy 层可通过 ctx.Value 读取）
		ctx := context.WithValue(c.Request.Context(), TraceIDKey, traceID)
		c.Request = c.Request.WithContext(ctx)

		// 响应头也带上，方便客户端日志关联
		c.Header("X-Request-Id", traceID)

		c.Next()
	}
}

// GetTraceID 从 context.Context 中提取 TraceID（供 service/proxy 层使用）
// ctx 为 nil 时返回空字符串（兼容测试场景）
func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}
