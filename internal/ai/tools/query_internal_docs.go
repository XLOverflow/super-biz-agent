package tools

import (
	"SuperBizAgent/internal/ai/retriever"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type QueryInternalDocsInput struct {
	Query string `json:"query" jsonschema:"description=The query string to search in internal documentation for relevant information and processing steps"`
}

func NewQueryInternalDocsTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_internal_docs",
		"Use this tool to search internal documentation and knowledge base for relevant information. It performs RAG (Retrieval-Augmented Generation) to find similar documents and extract processing steps. This is useful when you need to understand internal procedures, best practices, or step-by-step guides stored in the company's documentation.",
		func(ctx context.Context, input *QueryInternalDocsInput, opts ...tool.Option) (string, error) {
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
