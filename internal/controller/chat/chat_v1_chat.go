package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/utility/mem"
	"context"
	"errors"

	"github.com/cloudwego/eino/schema"
)

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	id := req.Id
	msg := req.Question

	agent, err := chat_pipeline.BuildChatAgent(ctx)
	if err != nil {
		return nil, err
	}

	memory, err := mem.GetPersistentMemory(ctx, id)
	if err != nil {
		return nil, err
	}
	history, err := memory.GetHistory(ctx)
	if err != nil {
		return nil, err
	}

	userMessage := &chat_pipeline.UserMessage{
		ID:      id,
		Query:   msg,
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
			return nil, event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		// 只取 assistant 的非流式最终回复，跳过工具调用消息
		if !mv.IsStreaming && mv.Message != nil &&
			mv.Message.Role == schema.Assistant && mv.Message.Content != "" {
			answer = mv.Message.Content
		}
	}

	if answer == "" {
		return nil, errors.New("no response from agent")
	}

	// persist turn; compact runs inside AddTurn if needed (non-fatal)
	_ = memory.AddTurn(ctx, msg, answer)

	return &v1.ChatRes{Answer: answer}, nil
}
