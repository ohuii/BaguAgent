package retriever

import (
	"context"
	"fmt"

	"bagu-agent/backend/internal/embedder"
	"bagu-agent/backend/internal/vectorstore"
)

// Service 封装 query embedding + Milvus TopK 检索。
type Service struct {
	embedder embedder.Client
	milvus   vectorstore.Store
}

// NewService 创建检索服务。
func NewService(embedder embedder.Client, milvus vectorstore.Store) *Service {
	return &Service{embedder: embedder, milvus: milvus}
}

// SearchInput 是语义检索入参。
type SearchInput struct {
	UserID   uint64 `json:"user_id"`
	Query    string `json:"query"`
	Category string `json:"category"`
	TopK     int    `json:"top_k"`
}

// Search 执行语义检索。
func (s *Service) Search(ctx context.Context, input SearchInput) ([]vectorstore.SearchResult, error) {
	if input.UserID == 0 {
		input.UserID = 1
	}
	if input.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	vectors, err := s.embedder.EmbedTexts(ctx, []string{input.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embedding query returned %d vectors", len(vectors))
	}
	return s.milvus.Search(ctx, vectorstore.SearchRequest{
		UserID:   input.UserID,
		Category: input.Category,
		Query:    vectors[0],
		TopK:     input.TopK,
	})
}
