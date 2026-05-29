package tool

import (
	"context"
	"fmt"

	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"
)

// SearchKnowledgeInput 是知识库检索工具入参。
type SearchKnowledgeInput struct {
	UserID   uint64 `json:"user_id"`
	Query    string `json:"query"`
	Category string `json:"category"`
	TopK     int    `json:"top_k"`
}

// SearchKnowledgeOutput 是知识库检索工具出参。
type SearchKnowledgeOutput struct {
	Chunks []vectorstore.SearchResult `json:"chunks"`
}

// SearchKnowledgeTool 根据用户问题从 Milvus 检索相关知识片段。
type SearchKnowledgeTool struct {
	retriever *retriever.Service
}

// NewSearchKnowledgeTool 创建知识检索工具。
func NewSearchKnowledgeTool(retriever *retriever.Service) *SearchKnowledgeTool {
	return &SearchKnowledgeTool{retriever: retriever}
}

func (t *SearchKnowledgeTool) Name() string {
	return "SearchKnowledgeTool"
}

func (t *SearchKnowledgeTool) Description() string {
	return "根据用户问题从个人八股知识库检索相关 Markdown chunks。"
}

func (t *SearchKnowledgeTool) Call(ctx context.Context, input any) (any, error) {
	in, ok := input.(SearchKnowledgeInput)
	if !ok {
		return nil, fmt.Errorf("invalid SearchKnowledgeTool input")
	}
	chunks, err := t.retriever.Search(ctx, retriever.SearchInput{
		UserID:   in.UserID,
		Query:    in.Query,
		Category: in.Category,
		TopK:     in.TopK,
	})
	if err != nil {
		return nil, err
	}
	return SearchKnowledgeOutput{Chunks: chunks}, nil
}
