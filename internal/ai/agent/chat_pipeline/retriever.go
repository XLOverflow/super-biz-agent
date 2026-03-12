package chat_pipeline

import (
	retriever2 "SecOpsAgent/internal/ai/retriever"
	"context"

	"github.com/cloudwego/eino/components/retriever"
)

func newRetriever(ctx context.Context) (rtr retriever.Retriever, err error) {
	return retriever2.NewHybridRetriever(ctx)
}
