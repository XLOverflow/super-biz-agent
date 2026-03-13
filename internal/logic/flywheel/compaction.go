package flywheel

import (
	"SecOpsAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	// compactBatchSize 每次压缩处理的同类文档数量上限
	compactBatchSize = 20
	// ttlDays 处置记录的最大保留天数，超过此期限的记录将被淘汰
	ttlDays = 180
)

// CompactByEventType 将同类事件的多条处置记录压缩为一条"最佳实践"。
// summarizer 是 LLM 摘要函数，负责将多条记录总结为一条。
// 返回被删除的旧文档数量。
func (s *Service) CompactByEventType(ctx context.Context, eventType string, summarizer func(ctx context.Context, texts []string) (string, error)) (int, error) {
	if err := s.milvusClient.LoadCollection(ctx, common.MilvusCollectionName, false); err != nil {
		return 0, fmt.Errorf("flywheel compact: load collection: %w", err)
	}

	// 1. 查询该事件类型下来自飞轮的所有文档
	expr := fmt.Sprintf(`metadata["source"] == "flywheel" && metadata["event_type"] == "%s"`, eventType)
	results, err := s.milvusClient.Query(
		ctx,
		common.MilvusCollectionName,
		nil,
		expr,
		[]string{"id", "content", "metadata"},
	)
	if err != nil {
		return 0, fmt.Errorf("flywheel compact: query: %w", err)
	}
	if len(results) == 0 {
		return 0, nil
	}

	// 提取 ID 和 content
	var docIDs []string
	var contents []string
	idCol := results.GetColumn("id")
	contentCol := results.GetColumn("content")
	for i := 0; i < idCol.Len(); i++ {
		id, _ := idCol.GetAsString(i)
		content, _ := contentCol.GetAsString(i)
		docIDs = append(docIDs, id)
		contents = append(contents, content)
	}

	// 不足2条无需压缩
	if len(docIDs) < 2 {
		return 0, nil
	}

	// 限制单次处理量
	if len(contents) > compactBatchSize {
		contents = contents[:compactBatchSize]
		docIDs = docIDs[:compactBatchSize]
	}

	// 2. 调用 LLM 将多条记录总结为一条最佳实践
	summary, err := summarizer(ctx, contents)
	if err != nil {
		return 0, fmt.Errorf("flywheel compact: summarize: %w", err)
	}

	// 3. 删除旧文档
	deleteExpr := fmt.Sprintf(`id in [%s]`, quoteIDs(docIDs))
	if err := s.milvusClient.Delete(ctx, common.MilvusCollectionName, "", deleteExpr); err != nil {
		return 0, fmt.Errorf("flywheel compact: delete old docs: %w", err)
	}

	// 4. 将压缩后的最佳实践作为新文档入库
	bestPractice := &Record{
		EventType: eventType,
		Severity:  "N/A",
		Summary:   fmt.Sprintf("[最佳实践-基于%d次实战处置] %s", len(docIDs), summary),
		Actions:   "（已从多次处置记录中聚合）",
		Result:    "聚合记录",
	}
	if _, _, err := s.Ingest(ctx, bestPractice); err != nil {
		return 0, fmt.Errorf("flywheel compact: ingest best practice: %w", err)
	}

	log.Printf("flywheel compact: merged %d records into 1 best practice (event_type=%s)", len(docIDs), eventType)
	return len(docIDs), nil
}

// PurgeTTL 删除超过保留期限的飞轮文档。
// 返回被删除的文档数量。
func (s *Service) PurgeTTL(ctx context.Context) (int, error) {
	if err := s.milvusClient.LoadCollection(ctx, common.MilvusCollectionName, false); err != nil {
		return 0, fmt.Errorf("flywheel purge: load collection: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -ttlDays).Format(time.RFC3339)
	expr := fmt.Sprintf(`metadata["source"] == "flywheel" && metadata["indexed_at"] < "%s"`, cutoff)

	// 查询过期文档 ID
	results, err := s.milvusClient.Query(ctx, common.MilvusCollectionName, nil, expr, []string{"id"})
	if err != nil {
		return 0, fmt.Errorf("flywheel purge: query expired: %w", err)
	}
	if len(results) == 0 {
		return 0, nil
	}

	idCol := results.GetColumn("id")
	var expiredIDs []string
	for i := 0; i < idCol.Len(); i++ {
		id, _ := idCol.GetAsString(i)
		expiredIDs = append(expiredIDs, id)
	}

	if len(expiredIDs) == 0 {
		return 0, nil
	}

	deleteExpr := fmt.Sprintf(`id in [%s]`, quoteIDs(expiredIDs))
	if err := s.milvusClient.Delete(ctx, common.MilvusCollectionName, "", deleteExpr); err != nil {
		return 0, fmt.Errorf("flywheel purge: delete: %w", err)
	}

	log.Printf("flywheel purge: removed %d expired documents (older than %d days)", len(expiredIDs), ttlDays)
	return len(expiredIDs), nil
}

// quoteIDs 将字符串 ID 列表转为 Milvus expr 格式: "id1","id2","id3"
func quoteIDs(ids []string) string {
	result := ""
	for i, id := range ids {
		if i > 0 {
			result += ","
		}
		// 转义内部双引号
		escaped, _ := json.Marshal(id)
		result += string(escaped)
	}
	return result
}
