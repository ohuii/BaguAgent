package indexer

import (
	"context"
	"fmt"

	chunkmodel "bagu-agent/backend/internal/chunk"
	"bagu-agent/backend/internal/document"
	"bagu-agent/backend/internal/embedder"
	"bagu-agent/backend/internal/vectorstore"
)

// Service 编排 chunk embedding 和 Milvus 写入。
type Service struct {
	collection string
	docRepo    *document.Repository
	chunkRepo  *chunkmodel.Repository
	embedder   embedder.Client
	milvus     vectorstore.Store
}

// NewService 创建索引服务。
func NewService(collection string, docRepo *document.Repository, chunkRepo *chunkmodel.Repository, embedder embedder.Client, milvus vectorstore.Store) *Service {
	return &Service{
		collection: collection,
		docRepo:    docRepo,
		chunkRepo:  chunkRepo,
		embedder:   embedder,
		milvus:     milvus,
	}
}

// IndexDocument 将某个文档的 MySQL chunks 向量化并写入 Milvus。
func (s *Service) IndexDocument(ctx context.Context, documentID uint64) error {
	doc, err := s.docRepo.GetByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}
	chunks, err := s.chunkRepo.ListByDocumentID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("list chunks: %w", err)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("document has no chunks")
	}

	if err := s.docRepo.UpdateParseResult(ctx, documentID, document.StatusIndexing, len(chunks), ""); err != nil {
		return fmt.Errorf("mark document indexing: %w", err)
	}
	if err := s.milvus.EnsureCollection(ctx); err != nil {
		_ = s.docRepo.UpdateParseResult(ctx, documentID, document.StatusFailed, len(chunks), err.Error())
		return err
	}

	const batchSize = 32
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[start:end]
		texts := make([]string, 0, len(batch))
		for _, chunk := range batch {
			texts = append(texts, chunk.ContentWithTitle)
		}

		vectors, err := s.embedder.EmbedTexts(ctx, texts)
		if err != nil {
			_ = s.docRepo.UpdateParseResult(ctx, documentID, document.StatusFailed, len(chunks), err.Error())
			return fmt.Errorf("embed chunks: %w", err)
		}
		if len(vectors) != len(batch) {
			return fmt.Errorf("embedding count mismatch: got %d want %d", len(vectors), len(batch))
		}

		items := make([]vectorstore.ChunkVector, 0, len(batch))
		for i, chunk := range batch {
			items = append(items, vectorstore.ChunkVector{
				ChunkUID:   chunk.ChunkUID,
				DocumentID: int64(chunk.DocumentID),
				ChunkID:    int64(chunk.ID),
				UserID:     int64(doc.UserID),
				TitlePath:  chunk.TitlePath,
				Content:    chunk.Content,
				Category:   doc.Category,
				Embedding:  vectors[i],
			})
		}

		if err := s.milvus.InsertChunks(ctx, items); err != nil {
			_ = s.docRepo.UpdateParseResult(ctx, documentID, document.StatusFailed, len(chunks), err.Error())
			return err
		}
		for _, chunk := range batch {
			if err := s.chunkRepo.UpdateMilvusFields(ctx, chunk.ID, s.collection, chunk.ChunkUID); err != nil {
				return fmt.Errorf("update chunk milvus fields: %w", err)
			}
		}
	}

	if err := s.docRepo.UpdateParseResult(ctx, documentID, document.StatusIndexed, len(chunks), ""); err != nil {
		return fmt.Errorf("mark document indexed: %w", err)
	}
	return nil
}
