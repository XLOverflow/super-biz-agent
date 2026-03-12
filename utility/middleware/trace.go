package middleware

import (
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/google/uuid"
)

const TraceIDKey = "X-Request-ID"

// TraceMiddleware 请求追踪中间件
// 为每个请求生成唯一的 Request ID，写入响应头和上下文，记录请求起止日志（方法、路径、客户端IP、耗时、状态码）。
func TraceMiddleware(r *ghttp.Request) {
	requestID := uuid.New().String()

	// 写入上下文，供下游日志使用
	r.SetCtxVar(TraceIDKey, requestID)
	// 写入响应头
	r.Response.Header().Set(TraceIDKey, requestID)

	start := time.Now()
	clientIP := r.GetClientIp()

	g.Log().Infof(r.Context(), "[%s] --> %s %s from %s",
		requestID, r.Method, r.URL.Path, clientIP)

	r.Middleware.Next()

	latency := time.Since(start)
	status := r.Response.Status

	g.Log().Infof(r.Context(), "[%s] <-- %s %s %d %s",
		requestID, r.Method, r.URL.Path, status, latency)
}
