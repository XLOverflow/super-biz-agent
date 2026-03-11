package indexer

import (
	"SuperBizAgent/internal/ai/bm25"
	embedder2 "SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	milvuscli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// HybridIndexer stores documents with both dense float embeddings and BM25 sparse vectors.
type HybridIndexer struct {
	milvusClient milvuscli.Client
	embedder     embedding.Embedder
	bm25Encoder  *bm25.Encoder
}

// NewHybridIndexer creates a hybrid indexer that writes both dense and BM25 sparse vectors.
func NewHybridIndexer(ctx context.Context) (indexer.Indexer, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}
	eb, err := embedder2.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	return &HybridIndexer{
		milvusClient: cli,
		embedder:     eb,
		bm25Encoder:  bm25.NewEncoder(),
	}, nil
}

// Store implements indexer.Indexer.
func (h *HybridIndexer) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}

	vectors, err := h.embedder.EmbedStrings(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("hybrid indexer: embed error: %w", err)
	}
	if len(vectors) != len(docs) {
		return nil, fmt.Errorf("hybrid indexer: embedding count mismatch: need %d got %d", len(docs), len(vectors))
	}

	rows := make([]interface{}, len(docs))
	ids := make([]string, len(docs))
	for i, doc := range docs {
		id := doc.ID
		if id == "" {
			id = fmt.Sprintf("doc_%d", i)
		}
		ids[i] = id

		// Dense vector: convert float64 → float32
		denseF32 := make([]float32, len(vectors[i]))
		for j, v := range vectors[i] {
			denseF32[j] = float32(v)
		}

		// Sparse vector: BM25 bigram encoding
		positions, values := h.bm25Encoder.Encode(doc.Content)
		sparseEmb, err := entity.NewSliceSparseEmbedding(positions, values)
		if err != nil {
			return nil, fmt.Errorf("hybrid indexer: sparse embedding error for doc %s: %w", id, err)
		}

		// Metadata: marshal doc.MetaData to JSON bytes
		metaBytes, err := json.Marshal(doc.MetaData)
		if err != nil {
			metaBytes = []byte("{}")
		}

		rows[i] = map[string]interface{}{
			"id":            id,
			"dense_vector":  denseF32,
			"sparse_vector": sparseEmb,
			"content":       doc.Content,
			"metadata":      metaBytes,
		}
	}

	if _, err = h.milvusClient.InsertRows(ctx, common.MilvusCollectionName, "", rows); err != nil {
		return nil, fmt.Errorf("hybrid indexer: insert rows error: %w", err)
	}
	if err = h.milvusClient.Flush(ctx, common.MilvusCollectionName, false); err != nil {
		return nil, fmt.Errorf("hybrid indexer: flush error: %w", err)
	}
	return ids, nil
}
