package retriever

import (
	"context"
	"fmt"
	"strings"

	"bagu-agent/backend/internal/embedder"
	"bagu-agent/backend/internal/markdown"
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
	searchCategory := input.Category
	topK := input.TopK
	if searchCategory != "" {
		// Milvus 中可能存在旧 scalar 字段，先扩大召回，再按 title_path 做可靠过滤。
		searchCategory = ""
		topK = input.TopK * 5
	}
	results, err := s.milvus.Search(ctx, vectorstore.SearchRequest{
		UserID:   input.UserID,
		Category: searchCategory,
		Query:    vectors[0],
		TopK:     topK,
	})
	if err != nil {
		return nil, err
	}
	return filterByCategory(results, input.Category, input.TopK), nil
}

func filterByCategory(results []vectorstore.SearchResult, category string, topK int) []vectorstore.SearchResult {
	if category == "" {
		return limitResults(results, topK)
	}

	filtered := make([]vectorstore.SearchResult, 0, topK)
	for _, result := range results {
		inferred := markdown.InferCategory(result.TitlePath, result.Category)
		if !strings.EqualFold(inferred, category) {
			continue
		}
		result.Category = inferred
		filtered = append(filtered, result)
		if len(filtered) >= topK {
			break
		}
	}
	return filtered
}

func limitResults(results []vectorstore.SearchResult, topK int) []vectorstore.SearchResult {
	if topK <= 0 || len(results) <= topK {
		return results
	}
	return results[:topK]
}
