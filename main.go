package main

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/controller/chat"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/mem"
	"SuperBizAgent/utility/middleware"
	"log"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
)

func main() {
	ctx := gctx.New()
	fileDir, err := g.Cfg().Get(ctx, "file_dir")
	if err != nil {
		panic(err)
	}
	common.FileDir = fileDir.String()

	// Register LLM summarizer for memory compaction
	summarizer, err := chat_pipeline.BuildSummarizer(ctx)
	if err != nil {
		log.Printf("warn: failed to build summarizer, memory compact will fall back to dropping: %v", err)
	} else {
		mem.SetSummarizer(summarizer)
	}

	s := g.Server()
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1())
	})
	s.SetPort(6872)
	s.Run()
}
