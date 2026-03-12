package client

import (
	"SecOpsAgent/utility/common"
	"context"
	"fmt"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func NewMilvusClient(ctx context.Context) (cli.Client, error) {
	// 1. 先连接default数据库
	defaultClient, err := cli.NewClient(ctx, cli.Config{
		Address: "localhost:19530",
		DBName:  "default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to default database: %w", err)
	}
	// 2. 检查secops数据库是否存在，不存在则创建
	databases, err := defaultClient.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	secopsDBExists := false
	for _, db := range databases {
		if db.Name == common.MilvusDBName {
			secopsDBExists = true
			break
		}
	}
	if !secopsDBExists {
		err = defaultClient.CreateDatabase(ctx, common.MilvusDBName)
		if err != nil {
			return nil, fmt.Errorf("failed to create secops database: %w", err)
		}
	}

	// 3. 创建连接到agent数据库的客户端
	secopsClient, err := cli.NewClient(ctx, cli.Config{
		Address: "localhost:19530",
		DBName:  common.MilvusDBName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to secops database: %w", err)
	}
	// 4. 检查biz collection是否存在，不存在则创建
	collections, err := secopsClient.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	kbCollectionExists := false
	for _, collection := range collections {
		if collection.Name == common.MilvusCollectionName {
			kbCollectionExists = true
			break
		}
	}

	if !kbCollectionExists {
		// 创建biz collection的schema
		schema := &entity.Schema{
			CollectionName: common.MilvusCollectionName,
			Description:    "Security knowledge base collection for playbooks and runbooks",
			Fields:         collectionFields,
		}

		err = secopsClient.CreateCollection(ctx, schema, entity.DefaultShardNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to create security_kb collection: %w", err)
		}

		// 为dense vector字段创建HNSW索引（IP距离，适配text-embedding-v4 float32输出）
		denseIndex, err := entity.NewIndexHNSW(entity.IP, 16, 200)
		if err != nil {
			return nil, fmt.Errorf("failed to create dense vector index: %w", err)
		}
		err = secopsClient.CreateIndex(ctx, common.MilvusCollectionName, "dense_vector", denseIndex, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create dense vector index: %w", err)
		}

		// 为sparse vector字段创建SparseInverted索引（BM25关键词检索）
		sparseIndex, err := entity.NewIndexSparseInverted(entity.IP, 0.1)
		if err != nil {
			return nil, fmt.Errorf("failed to create sparse vector index: %w", err)
		}
		err = secopsClient.CreateIndex(ctx, common.MilvusCollectionName, "sparse_vector", sparseIndex, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create sparse vector index: %w", err)
		}
	}

	// 关闭default数据库连接
	defaultClient.Close()

	return secopsClient, nil
}

// collectionFields defines the Milvus collection schema for hybrid search.
// dense_vector: float32 embeddings from text-embedding-v4 (dim=2048, IP metric)
// sparse_vector: BM25 bigram sparse embeddings for keyword search
var collectionFields = []*entity.Field{
	{
		Name:     "id",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "256",
		},
		PrimaryKey: true,
	},
	{
		Name:     "dense_vector",
		DataType: entity.FieldTypeFloatVector,
		TypeParams: map[string]string{
			"dim": "2048",
		},
	},
	{
		Name:     "sparse_vector",
		DataType: entity.FieldTypeSparseVector,
	},
	{
		Name:     "content",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "8192",
		},
	},
	{
		Name:     "metadata",
		DataType: entity.FieldTypeJSON,
	},
}
