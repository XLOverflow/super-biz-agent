package chat_pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func BuildChatAgent(ctx context.Context) (adk.Agent, error) {
	chatModel, err := newChatModel(ctx)
	if err != nil {
		return nil, err
	}

	retriever, err := newRetriever(ctx)
	if err != nil {
		return nil, err
	}

	tools, err := buildTools(ctx)
	if err != nil {
		return nil, err
	}

	ragMiddleware := adk.AgentMiddleware{
		BeforeChatModel: func(ctx context.Context, state *adk.ChatModelAgentState) error {
			// 已经注入过文档则跳过，避免每轮 ReAct 迭代重复检索
			for _, msg := range state.Messages {
				if strings.Contains(msg.Content, "==== 文档开始 ====") {
					return nil
				}
			}

			// 取最后一条用户消息作为检索 query
			query := ""
			for i := len(state.Messages) - 1; i >= 0; i-- {
				if state.Messages[i].Role == schema.User {
					query = state.Messages[i].Content
					break
				}
			}
			if query == "" {
				return nil
			}

			docs, err := retriever.Retrieve(ctx, query)
			if err != nil || len(docs) == 0 {
				return nil
			}

			var sb strings.Builder
			for _, doc := range docs {
				sb.WriteString(doc.Content)
				sb.WriteString("\n")
			}

			docMsg := schema.UserMessage(fmt.Sprintf(
				"以下是相关参考文档，请参考后回答用户问题：\n==== 文档开始 ====\n%s==== 文档结束 ====",
				sb.String(),
			))

			// 将文档消息插入到最后一条用户消息之前，而不是修改 system prompt
			lastUserIdx := -1
			for i := len(state.Messages) - 1; i >= 0; i-- {
				if state.Messages[i].Role == schema.User {
					lastUserIdx = i
					break
				}
			}

			newMessages := make([]*schema.Message, 0, len(state.Messages)+1)
			newMessages = append(newMessages, state.Messages[:lastUserIdx]...)
			newMessages = append(newMessages, docMsg)
			newMessages = append(newMessages, state.Messages[lastUserIdx:]...)
			state.Messages = newMessages

			return nil
		},
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "ChatAgent",
		Description: "对话助手，能够根据文档和工具回答用户问题",
		Instruction: buildSystemPrompt(time.Now().Format("2006-01-02")),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
		Middlewares: []adk.AgentMiddleware{ragMiddleware},
	})
}

// BuildAgentInput 将 UserMessage 转换为 AgentInput（对话历史 + 当前问题拼为 messages）
func BuildAgentInput(input *UserMessage, streaming bool) *adk.AgentInput {
	msgs := make([]*schema.Message, 0, len(input.History)+1)
	msgs = append(msgs, input.History...)
	msgs = append(msgs, schema.UserMessage(input.Query))
	return &adk.AgentInput{
		Messages:        msgs,
		EnableStreaming: streaming,
	}
}
