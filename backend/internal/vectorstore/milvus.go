package vectorstore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"bagu-agent/backend/internal/config"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	fieldID         = "id"
	fieldDocumentID = "document_id"
	fieldChunkID    = "chunk_id"
	fieldUserID     = "user_id"
	fieldTitlePath  = "title_path"
	fieldContent    = "content"
	fieldCategory   = "category"
	fieldEmbedding  = "embedding"
)

// ChunkVector 是写入 Milvus 的 chunk 向量和元信息。
type ChunkVector struct {
	ChunkUID   string
	DocumentID int64
	ChunkID    int64
	UserID     int64
	TitlePath  string
	Content    string
	Category   string
	Embedding  []float32
}

// SearchRequest 是 Milvus 检索请求。
type SearchRequest struct {
	UserID   uint64
	Category string
	Query    []float32
	TopK     int
}

// SearchResult 是 Milvus 检索结果。
type SearchResult struct {
	ChunkUID   string  `json:"chunk_uid"`
	DocumentID int64   `json:"document_id"`
	ChunkID    int64   `json:"chunk_id"`
	TitlePath  string  `json:"title_path"`
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Score      float32 `json:"score"`
}

// Store 是向量库的业务接口。
type Store interface {
	EnsureCollection(ctx context.Context) error
	InsertChunks(ctx context.Context, chunks []ChunkVector) error
	DeleteByDocumentID(ctx context.Context, documentID uint64) error
	Flush(ctx context.Context) error
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
}

// MilvusStore 封装 Milvus collection 初始化、写入和检索。
type MilvusStore struct {
	cfg    config.MilvusConfig
	client client.Client
}

// LazyMilvusStore 延迟创建 Milvus 连接，避免本地只测 MySQL/API 时启动失败。
type LazyMilvusStore struct {
	cfg   config.MilvusConfig
	mu    sync.Mutex
	store *MilvusStore
}

// NewLazyMilvusStore 创建懒加载 Milvus store。
func NewLazyMilvusStore(cfg config.MilvusConfig) *LazyMilvusStore {
	return &LazyMilvusStore{cfg: cfg}
}

// EnsureCollection 确保 collection 已创建并加载。
func (s *LazyMilvusStore) EnsureCollection(ctx context.Context) error {
	store, err := s.get(ctx)
	if err != nil {
		return err
	}
	return store.EnsureCollection(ctx)
}

// InsertChunks 批量写入 chunk 向量。
func (s *LazyMilvusStore) InsertChunks(ctx context.Context, chunks []ChunkVector) error {
	store, err := s.get(ctx)
	if err != nil {
		return err
	}
	return store.InsertChunks(ctx, chunks)
}

// DeleteByDocumentID 按 document_id 删除该文档在 Milvus 中的所有 chunk 向量。
func (s *LazyMilvusStore) DeleteByDocumentID(ctx context.Context, documentID uint64) error {
	store, err := s.get(ctx)
	if err != nil {
		return err
	}
	return store.DeleteByDocumentID(ctx, documentID)
}

// Flush 刷新 collection，使写入的数据可用于后续检索。
func (s *LazyMilvusStore) Flush(ctx context.Context) error {
	store, err := s.get(ctx)
	if err != nil {
		return err
	}
	return store.Flush(ctx)
}

// Search 根据 query embedding 检索 TopK chunk。
func (s *LazyMilvusStore) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	store, err := s.get(ctx)
	if err != nil {
		return nil, err
	}
	return store.Search(ctx, req)
}

func (s *LazyMilvusStore) get(ctx context.Context) (*MilvusStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store != nil {
		return s.store, nil
	}
	store, err := NewMilvusStore(ctx, s.cfg)
	if err != nil {
		return nil, err
	}
	s.store = store
	return s.store, nil
}

// NewMilvusStore 创建 Milvus 客户端。
func NewMilvusStore(ctx context.Context, cfg config.MilvusConfig) (*MilvusStore, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cli, err := client.NewClient(connectCtx, client.Config{Address: cfg.Addr})
	if err != nil {
		return nil, fmt.Errorf("connect milvus: %w", err)
	}
	return &MilvusStore{cfg: cfg, client: cli}, nil
}

// Close 关闭 Milvus 连接。
func (s *MilvusStore) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

// EnsureCollection 确保 collection、向量索引和 load 状态已准备好。
func (s *MilvusStore) EnsureCollection(ctx context.Context) error {
	exists, err := s.client.HasCollection(ctx, s.cfg.CollectionName)
	if err != nil {
		return fmt.Errorf("check milvus collection: %w", err)
	}
	if !exists {
		if err := s.createCollection(ctx); err != nil {
			return err
		}
		if err := s.createIndex(ctx); err != nil {
			return err
		}
	}
	if err := s.client.LoadCollection(ctx, s.cfg.CollectionName, false); err != nil {
		return fmt.Errorf("load milvus collection: %w", err)
	}
	return nil
}

// InsertChunks 批量写入 chunk 向量。
func (s *MilvusStore) InsertChunks(ctx context.Context, chunks []ChunkVector) error {
	if len(chunks) == 0 {
		return nil
	}

	ids := make([]string, 0, len(chunks))
	documentIDs := make([]int64, 0, len(chunks))
	chunkIDs := make([]int64, 0, len(chunks))
	userIDs := make([]int64, 0, len(chunks))
	titlePaths := make([]string, 0, len(chunks))
	contents := make([]string, 0, len(chunks))
	categories := make([]string, 0, len(chunks))
	embeddings := make([][]float32, 0, len(chunks))

	for _, chunk := range chunks {
		ids = append(ids, chunk.ChunkUID)
		documentIDs = append(documentIDs, chunk.DocumentID)
		chunkIDs = append(chunkIDs, chunk.ChunkID)
		userIDs = append(userIDs, chunk.UserID)
		titlePaths = append(titlePaths, limitString(chunk.TitlePath, 1024))
		contents = append(contents, limitString(chunk.Content, 8192))
		categories = append(categories, limitString(chunk.Category, 64))
		embeddings = append(embeddings, chunk.Embedding)
	}

	_, err := s.client.Upsert(ctx, s.cfg.CollectionName, "",
		entity.NewColumnVarChar(fieldID, ids),
		entity.NewColumnInt64(fieldDocumentID, documentIDs),
		entity.NewColumnInt64(fieldChunkID, chunkIDs),
		entity.NewColumnInt64(fieldUserID, userIDs),
		entity.NewColumnVarChar(fieldTitlePath, titlePaths),
		entity.NewColumnVarChar(fieldContent, contents),
		entity.NewColumnVarChar(fieldCategory, categories),
		entity.NewColumnFloatVector(fieldEmbedding, s.cfg.EmbeddingDim, embeddings),
	)
	if err != nil {
		return fmt.Errorf("upsert milvus chunks: %w", err)
	}
	return nil
}

// Flush 刷新 collection，使本次写入尽快对搜索可见。
// DeleteByDocumentID 按 document_id 删除该文档在 Milvus 中的所有 chunk 向量。
func (s *MilvusStore) DeleteByDocumentID(ctx context.Context, documentID uint64) error {
	if documentID == 0 {
		return nil
	}
	exists, err := s.client.HasCollection(ctx, s.cfg.CollectionName)
	if err != nil {
		return fmt.Errorf("check milvus collection: %w", err)
	}
	if !exists {
		return nil
	}
	expr := fmt.Sprintf("%s == %d", fieldDocumentID, documentID)
	if err := s.client.Delete(ctx, s.cfg.CollectionName, "", expr); err != nil {
		return fmt.Errorf("delete milvus document vectors: %w", err)
	}
	return s.Flush(ctx)
}

func (s *MilvusStore) Flush(ctx context.Context) error {
	if err := s.client.Flush(ctx, s.cfg.CollectionName, false); err != nil {
		return fmt.Errorf("flush milvus collection: %w", err)
	}
	return nil
}

// Search 根据 query embedding 检索 TopK chunk。
func (s *MilvusStore) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	if req.TopK <= 0 {
		req.TopK = 5
	}
	expr := buildFilterExpr(req.UserID, req.Category)
	sp, err := entity.NewIndexHNSWSearchParam(64)
	if err != nil {
		return nil, fmt.Errorf("create search param: %w", err)
	}

	results, err := s.client.Search(ctx,
		s.cfg.CollectionName,
		nil,
		expr,
		[]string{fieldDocumentID, fieldChunkID, fieldTitlePath, fieldContent, fieldCategory},
		[]entity.Vector{entity.FloatVector(req.Query)},
		fieldEmbedding,
		entity.COSINE,
		req.TopK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("search milvus: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	return parseSearchResult(results[0])
}

func (s *MilvusStore) createCollection(ctx context.Context) error {
	schema := entity.NewSchema().
		WithName(s.cfg.CollectionName).
		WithDescription("BaguAgent markdown chunks").
		WithAutoID(false).
		WithField(entity.NewField().WithName(fieldID).WithDataType(entity.FieldTypeVarChar).WithIsPrimaryKey(true).WithMaxLength(128)).
		WithField(entity.NewField().WithName(fieldDocumentID).WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName(fieldChunkID).WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName(fieldUserID).WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName(fieldTitlePath).WithDataType(entity.FieldTypeVarChar).WithMaxLength(1024)).
		WithField(entity.NewField().WithName(fieldContent).WithDataType(entity.FieldTypeVarChar).WithMaxLength(8192)).
		WithField(entity.NewField().WithName(fieldCategory).WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName(fieldEmbedding).WithDataType(entity.FieldTypeFloatVector).WithDim(int64(s.cfg.EmbeddingDim)))

	if err := s.client.CreateCollection(ctx, schema, 2); err != nil {
		return fmt.Errorf("create milvus collection: %w", err)
	}
	return nil
}

func (s *MilvusStore) createIndex(ctx context.Context) error {
	idx, err := entity.NewIndexHNSW(entity.COSINE, 16, 200)
	if err != nil {
		return fmt.Errorf("create hnsw index: %w", err)
	}
	if err := s.client.CreateIndex(ctx, s.cfg.CollectionName, fieldEmbedding, idx, false); err != nil {
		return fmt.Errorf("create milvus index: %w", err)
	}
	return nil
}

func buildFilterExpr(userID uint64, category string) string {
	var parts []string
	if userID > 0 {
		parts = append(parts, fmt.Sprintf("%s == %d", fieldUserID, userID))
	}
	if category != "" {
		parts = append(parts, fmt.Sprintf("%s == \"%s\"", fieldCategory, escapeMilvusString(category)))
	}
	return strings.Join(parts, " && ")
}

func parseSearchResult(result client.SearchResult) ([]SearchResult, error) {
	ids := result.IDs
	documentIDs := result.Fields.GetColumn(fieldDocumentID)
	chunkIDs := result.Fields.GetColumn(fieldChunkID)
	titlePaths := result.Fields.GetColumn(fieldTitlePath)
	contents := result.Fields.GetColumn(fieldContent)
	categories := result.Fields.GetColumn(fieldCategory)

	items := make([]SearchResult, 0, result.ResultCount)
	for i := 0; i < result.ResultCount; i++ {
		chunkUID, _ := ids.GetAsString(i)
		documentID, _ := documentIDs.GetAsInt64(i)
		chunkID, _ := chunkIDs.GetAsInt64(i)
		titlePath, _ := titlePaths.GetAsString(i)
		content, _ := contents.GetAsString(i)
		category, _ := categories.GetAsString(i)
		items = append(items, SearchResult{
			ChunkUID:   chunkUID,
			DocumentID: documentID,
			ChunkID:    chunkID,
			TitlePath:  titlePath,
			Content:    content,
			Category:   category,
			Score:      result.Scores[i],
		})
	}
	return items, nil
}

func limitString(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func escapeMilvusString(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}
