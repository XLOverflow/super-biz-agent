package retriever

import (
	"SuperBizAgent/internal/ai/bm25"
	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	milvuscli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	defaultTopK     = 5
	denseWeight     = 0.7
	sparseWeight    = 0.3
	hnswEf          = 100
	sparseDropRatio = 0.0
)

// HybridRetriever implements retriever.Retriever using Milvus HybridSearch
// combining dense vector search (weight 0.7) and BM25 sparse search (weight 0.3).
type HybridRetriever struct {
	milvusClient milvuscli.Client
	embedder     embedding.Embedder
	bm25Encoder  *bm25.Encoder
	topK         int
}

// NewHybridRetriever creates a hybrid retriever backed by Milvus.
func NewHybridRetriever(ctx context.Context) (retriever.Retriever, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}
	eb, err := embedder.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	return &HybridRetriever{
		milvusClient: cli,
		embedder:     eb,
		bm25Encoder:  bm25.NewEncoder(),
		topK:         defaultTopK,
	}, nil
}

// Retrieve implements retriever.Retriever.
func (h *HybridRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	topK := h.topK
	for _, o := range opts {
		co := retriever.GetCommonOptions(&retriever.Options{}, o)
		if co.TopK != nil && *co.TopK > 0 {
			topK = *co.TopK
		}
	}

	// 1. Dense embedding for the query
	vecs, err := h.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("hybrid retriever: embed query error: %w", err)
	}
	denseF32 := make([]float32, len(vecs[0]))
	for i, v := range vecs[0] {
		denseF32[i] = float32(v)
	}
	denseVec := entity.FloatVector(denseF32)

	// 2. BM25 sparse vector for the query
	positions, values := h.bm25Encoder.Encode(query)
	sparseVec, err := entity.NewSliceSparseEmbedding(positions, values)
	if err != nil {
		return nil, fmt.Errorf("hybrid retriever: sparse embed error: %w", err)
	}

	// 3. Build ANN sub-requests
	denseSearchParam, err := entity.NewIndexHNSWSearchParam(hnswEf)
	if err != nil {
		return nil, fmt.Errorf("hybrid retriever: dense search param error: %w", err)
	}
	denseReq := milvuscli.NewANNSearchRequest(
		"dense_vector", entity.IP, "",
		[]entity.Vector{denseVec}, denseSearchParam, topK,
	)

	sparseSearchParam, err := entity.NewIndexSparseInvertedSearchParam(sparseDropRatio)
	if err != nil {
		return nil, fmt.Errorf("hybrid retriever: sparse search param error: %w", err)
	}
	sparseReq := milvuscli.NewANNSearchRequest(
		"sparse_vector", entity.IP, "",
		[]entity.Vector{sparseVec}, sparseSearchParam, topK,
	)

	// 4. Weighted reranker: dense=0.7, sparse=0.3
	reranker := milvuscli.NewWeightedReranker([]float64{denseWeight, sparseWeight})

	outputFields := []string{"id", "content", "metadata"}
	results, err := h.milvusClient.HybridSearch(
		ctx,
		common.MilvusCollectionName,
		nil, // no partition filter
		topK,
		outputFields,
		reranker,
		[]*milvuscli.ANNSearchRequest{denseReq, sparseReq},
	)
	if err != nil {
		return nil, fmt.Errorf("hybrid retriever: HybridSearch error: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	return h.parseResults(results[0])
}

func (h *HybridRetriever) parseResults(result milvuscli.SearchResult) ([]*schema.Document, error) {
	docs := make([]*schema.Document, 0, result.ResultCount)
	for i := 0; i < result.ResultCount; i++ {
		// Extract ID
		idVal, err := result.IDs.GetAsString(i)
		if err != nil {
			return nil, fmt.Errorf("hybrid retriever: get id at %d: %w", i, err)
		}

		// Extract content
		content := ""
		for _, col := range result.Fields {
			if col.Name() == "content" {
				v, err := col.GetAsString(i)
				if err == nil {
					content = v
				}
				break
			}
		}

		// Extract metadata
		metaData := map[string]interface{}{}
		for _, col := range result.Fields {
			if col.Name() == "metadata" {
				raw, err := col.GetAsString(i)
				if err == nil {
					_ = json.Unmarshal([]byte(raw), &metaData)
				}
				break
			}
		}

		docs = append(docs, &schema.Document{
			ID:       idVal,
			Content:  content,
			MetaData: metaData,
		})
	}
	return docs, nil
}
