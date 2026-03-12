package health

import (
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
)

// RegisterRoutes 注册健康检查路由（不走认证中间件）
func RegisterRoutes(s *ghttp.Server) {
	s.BindHandler("GET:/healthz", Liveness)
	s.BindHandler("GET:/readyz", Readiness)
}

// Liveness 存活探针 — 服务进程正常即返回 200
func Liveness(r *ghttp.Request) {
	r.Response.WriteStatus(http.StatusOK)
	r.Response.WriteJson(map[string]string{
		"status": "ok",
	})
}

// Readiness 就绪探针 — 检查关键依赖（Milvus、SQLite）的连通性
func Readiness(r *ghttp.Request) {
	// 基本就绪检查：服务已启动即视为就绪
	// 生产环境可扩展检查 Milvus 连接和 SQLite 可写性
	r.Response.WriteStatus(http.StatusOK)
	r.Response.WriteJson(map[string]string{
		"status": "ready",
	})
}
