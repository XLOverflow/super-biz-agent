package chat

import (
	"SecOpsAgent/api/chat/v1"
	"SecOpsAgent/internal/ai/agent/chat_pipeline"
	"SecOpsAgent/utility/mem"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)


func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	id := req.Id
	msg := req.Question

	ctx = context.WithValue(ctx, "client_id", req.Id)
	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}

	agent, err := chat_pipeline.BuildChatAgent(ctx)
	if err != nil {
		client.SendToClient("error", err.Error())
		return nil, err
	}

	memory, memErr := mem.GetPersistentMemory(ctx, id)
	var history []*schema.Message
	if memErr == nil {
		history, _ = memory.GetHistory(ctx)
	}

	userMessage := &chat_pipeline.UserMessage{
		ID:      id,
		Query:   msg,
		History: history,
	}

	iter := agent.Run(ctx, chat_pipeline.BuildAgentInput(userMessage, true))

	var fullResponse strings.Builder
	defer func() {
		completeResponse := fullResponse.String()
		if completeResponse != "" && memory != nil {
			_ = memory.AddTurn(ctx, msg, completeResponse)
		}
	}()

	for {
		event, ok := iter.Next()
		if !ok {
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		if event.Err != nil {
			client.SendToClient("error", event.Err.Error())
			return &v1.ChatStreamRes{}, nil
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		// 只处理 assistant 的流式消息，跳过工具调用结果
		if mv.Role != schema.Assistant || !mv.IsStreaming {
			continue
		}
		for {
			chunk, chunkErr := mv.MessageStream.Recv()
			if errors.Is(chunkErr, io.EOF) {
				break
			}
			if chunkErr != nil {
				client.SendToClient("error", chunkErr.Error())
				return &v1.ChatStreamRes{}, nil
			}
			// content 为空说明是 tool_call 决策阶段，不转发给用户
			if chunk.Content != "" {
				fullResponse.WriteString(chunk.Content)
				client.SendToClient("message", chunk.Content)
			}
		}
	}
}
