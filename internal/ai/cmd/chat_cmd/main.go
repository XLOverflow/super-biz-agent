package main

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/utility/mem"
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()
	id := "111"

	agent, err := chat_pipeline.BuildChatAgent(ctx)
	if err != nil {
		panic(err)
	}

	memory, err := mem.GetPersistentMemory(ctx, id)
	if err != nil {
		panic(err)
	}

	invoke := func(query string) string {
		history, _ := memory.GetHistory(ctx)
		userMessage := &chat_pipeline.UserMessage{
			ID:      id,
			Query:   query,
			History: history,
		}
		iter := agent.Run(ctx, chat_pipeline.BuildAgentInput(userMessage, false))
		var answer string
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				panic(event.Err)
			}
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}
			mv := event.Output.MessageOutput
			if !mv.IsStreaming && mv.Message != nil &&
				mv.Message.Role == schema.Assistant && mv.Message.Content != "" {
				answer = mv.Message.Content
			}
		}
		return answer
	}

	// 第一次对话
	answer := invoke("你好")
	fmt.Println("Q: 你好")
	fmt.Println("A:", answer)
	_ = memory.AddTurn(ctx, "你好", answer)

	// 第二次对话
	answer = invoke("现在是几点")
	fmt.Println("----------------")
	fmt.Println("Q: 现在是几点")
	fmt.Println("A:", answer)
	_ = memory.AddTurn(ctx, "现在是几点", answer)
}
