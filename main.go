package main

import (
	"SecOpsAgent/internal/ai/agent/chat_pipeline"
	"SecOpsAgent/internal/controller/auth"
	"SecOpsAgent/internal/controller/chat"
	"SecOpsAgent/internal/controller/health"
	"SecOpsAgent/utility/common"
	"SecOpsAgent/utility/mem"
	"SecOpsAgent/utility/middleware"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // 加载 .env（文件不存在时忽略，生产环境可直接注入环境变量）
	ctx := gctx.New()

	fileDir, err := g.Cfg().Get(ctx, "file_dir")
	if err != nil {
		panic(err)
	}
	common.FileDir = fileDir.String()

	// 初始化 JWT 密钥（生产环境应从配置或密钥管理服务加载）
	jwtSecret, _ := g.Cfg().Get(ctx, "jwt_secret")
	secret := jwtSecret.String()
	if secret == "" {
		secret = "secops-default-secret-change-in-production"
	}
	middleware.InitJWT(secret)

	// 初始化限流参数（10 req/s per IP，突发容量20）
	middleware.InitRateLimit(10, 20)

	// 注册 LLM summarizer 用于记忆压缩
	summarizer, err := chat_pipeline.BuildSummarizer(ctx)
	if err != nil {
		log.Printf("warn: failed to build summarizer, memory compact will fall back to dropping: %v", err)
	} else {
		mem.SetSummarizer(summarizer)
	}

	s := g.Server()

	// 健康检查端点（不走认证和限流）
	health.RegisterRoutes(s)

	// 认证端点（不走 JWT，走 CORS + Trace + 响应中间件）
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.TraceMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(auth.NewV1())
	})

	// 业务 API（完整中间件链：CORS → Trace → RateLimit → JWT → Response）
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.TraceMiddleware)
		group.Middleware(middleware.RateLimitMiddleware)
		group.Middleware(middleware.JWTAuthMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1())
	})

	s.SetPort(6872)

	// 优雅关停：GoFrame s.Run() 已内置信号处理和连接排空。
	// 额外注册 goroutine 监听信号，在服务关停前释放数据库连接等资源。
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received signal %v, cleaning up resources...", sig)
		mem.Close()
		log.Println("Resource cleanup completed, server will shutdown gracefully")
	}()

	s.Run()
}
