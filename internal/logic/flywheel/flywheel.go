package flywheel

import (
	"SecOpsAgent/internal/ai/bm25"
	embedder2 "SecOpsAgent/internal/ai/embedder"
	"SecOpsAgent/utility/client"
	"SecOpsAgent/utility/common"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	milvuscli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	// dedupThreshold 向量相似度去重阈值，超过此值判定为重复内容
	dedupThreshold = 0.92
	// maxKBSize 知识库最大文档数硬限制
	maxKBSize = 10000
)

// Record 表示一条待入库的处置记录
type Record struct {
	EventType string // brute_force, port_scan, ddos, malware, etc.
	Severity  string // P0-P3
	Summary   string // Agent 生成的分析摘要
	Actions   string // 实际执行的处置步骤
	Result    string // 处置结果
}

// Service 知识飞轮服务：将 AIOps 处置记录自动索引回 RAG 知识库
type Service struct {
	milvusClient milvuscli.Client
	embedder     embedding.Embedder
	bm25Encoder  *bm25.Encoder
}

// NewService 创建知识飞轮服务实例
func NewService(ctx context.Context) (*Service, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("flywheel: connect milvus: %w", err)
	}
	eb, err := embedder2.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("flywheel: init embedder: %w", err)
	}
	return &Service{
		milvusClient: cli,
		embedder:     eb,
		bm25Encoder:  bm25.NewEncoder(),
	}, nil
}

// Ingest 将处置记录索引到知识库，入库前进行向量相似度去重。
// 返回 (docID, isDuplicate, error)。
func (s *Service) Ingest(ctx context.Context, record *Record) (string, bool, error) {
	content := formatRecord(record)

	// 1. 生成 dense embedding
	vecs, err := s.embedder.EmbedStrings(ctx, []string{content})
	if err != nil {
		return "", false, fmt.Errorf("flywheel: embed error: %w", err)
	}
	denseF32 := toFloat32(vecs[0])

	// 2. 去重检查：在知识库中检索最相似的文档
	isDup, err := s.isDuplicate(ctx, denseF32)
	if err != nil {
		log.Printf("flywheel: dedup check failed (will proceed with insert): %v", err)
		// 去重检查失败不阻塞入库，降级为不去重
	} else if isDup {
		log.Printf("flywheel: duplicate record skipped (event_type=%s)", record.EventType)
		return "", true, nil
	}

	// 3. 生成 sparse embedding (BM25)
	positions, values := s.bm25Encoder.Encode(content)
	sparseEmb, err := entity.NewSliceSparseEmbedding(positions, values)
	if err != nil {
		return "", false, fmt.Errorf("flywheel: sparse embed error: %w", err)
	}

	// 4. 构建文档 ID（基于内容哈希 + 时间戳，避免冲突）
	docID := generateDocID(record)

	// 5. 构建 metadata
	metadata, _ := json.Marshal(map[string]interface{}{
		"source":     "flywheel",
		"event_type": record.EventType,
		"severity":   record.Severity,
		"indexed_at": time.Now().Format(time.RFC3339),
	})

	// 6. 插入 Milvus
	row := map[string]interface{}{
		"id":            docID,
		"dense_vector":  denseF32,
		"sparse_vector": sparseEmb,
		"content":       content,
		"metadata":      metadata,
	}
	if _, err = s.milvusClient.InsertRows(ctx, common.MilvusCollectionName, "", []interface{}{row}); err != nil {
		return "", false, fmt.Errorf("flywheel: insert error: %w", err)
	}
	if err = s.milvusClient.Flush(ctx, common.MilvusCollectionName, false); err != nil {
		return "", false, fmt.Errorf("flywheel: flush error: %w", err)
	}

	log.Printf("flywheel: indexed record %s (event_type=%s, severity=%s)", docID, record.EventType, record.Severity)
	return docID, false, nil
}

// isDuplicate 通过向量相似度检查新记录是否与已有知识重复
func (s *Service) isDuplicate(ctx context.Context, denseVec []float32) (bool, error) {
	// 加载 collection 到内存（如果尚未加载）
	if err := s.milvusClient.LoadCollection(ctx, common.MilvusCollectionName, false); err != nil {
		return false, fmt.Errorf("load collection: %w", err)
	}

	searchParam, err := entity.NewIndexHNSWSearchParam(64)
	if err != nil {
		return false, err
	}

	results, err := s.milvusClient.Search(
		ctx,
		common.MilvusCollectionName,
		nil, // partitions
		"", // expr
		[]string{"id"},
		[]entity.Vector{entity.FloatVector(denseVec)},
		"dense_vector",
		entity.IP,
		1, // topK=1, 只需要最相似的一条
		searchParam,
	)
	if err != nil {
		return false, fmt.Errorf("search: %w", err)
	}
	if len(results) == 0 || results[0].ResultCount == 0 {
		return false, nil // 知识库为空，不可能重复
	}

	// IP (Inner Product) 距离：归一化向量时等价于 cosine similarity
	topScore := results[0].Scores[0]
	return topScore >= dedupThreshold, nil
}

// formatRecord 将处置记录格式化为适合向量化的文本
func formatRecord(r *Record) string {
	return fmt.Sprintf(
		"[安全事件处置记录] 事件类型: %s | 严重等级: %s\n分析摘要: %s\n处置步骤: %s\n处置结果: %s",
		r.EventType, r.Severity, r.Summary, r.Actions, r.Result,
	)
}

// generateDocID 生成基于内容哈希的文档 ID
func generateDocID(r *Record) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d", r.EventType, r.Summary, r.Actions, time.Now().UnixNano())))
	return fmt.Sprintf("fw_%x", hash[:8])
}

// toFloat32 将 float64 切片转换为 float32
func toFloat32(f64 []float64) []float32 {
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}
