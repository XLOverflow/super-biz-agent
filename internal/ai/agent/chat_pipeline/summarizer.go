package chat_pipeline

import (
	"SuperBizAgent/utility/mem"
	"context"

	"github.com/cloudwego/eino/schema"
)

// BuildSummarizer creates a mem.SummarizeFunc that calls the chat model to compress
// a list of messages into a concise summary (≤ 300 characters).
func BuildSummarizer(ctx context.Context) (mem.SummarizeFunc, error) {
	chatModel, err := newChatModel(ctx)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, msgs []*schema.Message) (string, error) {
		prompt := []*schema.Message{
			schema.SystemMessage("你是一个对话历史压缩助手。请将以下对话历史压缩为简洁摘要，不超过300字，保留关键信息、用户意图和重要结论。"),
		}
		prompt = append(prompt, msgs...)
		prompt = append(prompt, schema.UserMessage("请生成以上对话历史的摘要。"))
		result, err := chatModel.Generate(ctx, prompt)
		if err != nil {
			return "", err
		}
		return result.Content, nil
	}, nil
}
