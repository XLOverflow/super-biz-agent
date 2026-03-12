package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"golang.org/x/time/rate"
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	limiters   sync.Map // map[string]*ipLimiter
	rateLimit  rate.Limit = 10 // 默认 10 req/s
	rateBurst  int        = 20 // 突发容量
	cleanupRun sync.Once
)

// InitRateLimit 初始化限流参数
func InitRateLimit(qps float64, burst int) {
	rateLimit = rate.Limit(qps)
	rateBurst = burst
}

func getLimiter(ip string) *rate.Limiter {
	v, ok := limiters.Load(ip)
	if ok {
		entry := v.(*ipLimiter)
		entry.lastSeen = time.Now()
		return entry.limiter
	}
	limiter := rate.NewLimiter(rateLimit, rateBurst)
	limiters.Store(ip, &ipLimiter{limiter: limiter, lastSeen: time.Now()})
	return limiter
}

// startCleanup 定期清理长时间未访问的 IP 限流器，防止内存泄漏
func startCleanup() {
	cleanupRun.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				threshold := time.Now().Add(-10 * time.Minute)
				limiters.Range(func(key, value interface{}) bool {
					entry := value.(*ipLimiter)
					if entry.lastSeen.Before(threshold) {
						limiters.Delete(key)
					}
					return true
				})
			}
		}()
	})
}

// RateLimitMiddleware 基于客户端 IP 的令牌桶限流中间件
// 每个 IP 独立限流，超过阈值返回 429 Too Many Requests。
func RateLimitMiddleware(r *ghttp.Request) {
	startCleanup()

	clientIP := r.GetClientIp()
	limiter := getLimiter(clientIP)

	if !limiter.Allow() {
		g.Log().Warningf(r.Context(), "Rate limit exceeded for IP: %s, path: %s", clientIP, r.URL.Path)
		r.Response.Header().Set("Retry-After", "1")
		r.Response.WriteStatus(http.StatusTooManyRequests)
		r.Response.WriteJsonExit(Response{
			Code:    42900,
			Message: "请求频率超限，请稍后重试",
		})
		return
	}

	r.Middleware.Next()
}
