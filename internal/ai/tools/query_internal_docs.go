package tools

import (
	"SecOpsAgent/internal/ai/retriever"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type QuerySecurityPlaybookInput struct {
	Query string `json:"query" jsonschema:"description=安全事件关键词或告警名称，用于检索对应的安全处置Playbook和应急响应流程"`
}

func NewQuerySecurityPlaybookTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_security_playbook",
		"检索内部安全Playbook和应急响应知识库。基于 RAG 混合检索（向量语义 + BM25关键词）查找与安全事件匹配的处置流程、应急响应手册、加固方案等。当需要了解某类安全事件的标准处置步骤时使用此工具。",
		func(ctx context.Context, input *QuerySecurityPlaybookInput, opts ...tool.Option) (string, error) {
			rr, err := retriever.NewHybridRetriever(ctx)
			if err != nil {
				return "", fmt.Errorf("failed to create retriever: %w", err)
			}
			resp, err := rr.Retrieve(ctx, input.Query)
			if err != nil {
				return "", fmt.Errorf("failed to retrieve docs: %w", err)
			}
			respBytes, err := json.Marshal(resp)
			if err != nil {
				return "", fmt.Errorf("failed to marshal response: %w", err)
			}
			return string(respBytes), nil
		})
	if err != nil {
		return nil
	}
	return t
}
