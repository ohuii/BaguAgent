package indexer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	chunkmodel "bagu-agent/backend/internal/chunk"
	"bagu-agent/backend/internal/document"
	"bagu-agent/backend/internal/embedder"
	"bagu-agent/backend/internal/markdown"
	"bagu-agent/backend/internal/vectorstore"

	"go.uber.org/zap"
)

// Service 编排 chunk embedding 和 Milvus 写入。
type Service struct {
	collection string
	docRepo    *document.Repository
	chunkRepo  *chunkmodel.Repository
	embedder   embedder.Client
	milvus     vectorstore.Store
	taskRepo   *Repository
	log        *zap.Logger
}

// NewService 创建索引服务。
func NewService(collection string, docRepo *document.Repository, chunkRepo *chunkmodel.Repository, embedder embedder.Client, milvus vectorstore.Store, taskRepo *Repository, log *zap.Logger) *Service {
	return &Service{
		collection: collection,
		docRepo:    docRepo,
		chunkRepo:  chunkRepo,
		embedder:   embedder,
		milvus:     milvus,
		taskRepo:   taskRepo,
		log:        log,
	}
}

// IndexDocument 将某个文档的 MySQL chunks 向量化并写入 Milvus。
func (s *Service) IndexDocument(ctx context.Context, documentID uint64) error {
	return s.indexDocument(ctx, documentID, nil)
}

// StartIndexDocument 创建异步索引任务并立即返回 task_id。
func (s *Service) StartIndexDocument(ctx context.Context, documentID uint64) (*IndexTask, error) {
	if s.taskRepo == nil {
		return nil, fmt.Errorf("index task repository is not configured")
	}
	if _, err := s.docRepo.GetByID(ctx, documentID); err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	chunks, err := s.chunkRepo.ListByDocumentID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("list chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("document has no chunks")
	}

	task := &IndexTask{
		TaskUID:     newTaskUID(),
		DocumentID:  documentID,
		Status:      TaskStatusPending,
		TotalChunks: len(chunks),
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("create index task: %w", err)
	}

	go s.runIndexTask(task.TaskUID, documentID)
	return task, nil
}

// GetTask 查询索引任务进度。
func (s *Service) GetTask(ctx context.Context, taskUID string) (*IndexTask, error) {
	if s.taskRepo == nil {
		return nil, fmt.Errorf("index task repository is not configured")
	}
	return s.taskRepo.GetByTaskUID(ctx, taskUID)
}

func (s *Service) runIndexTask(taskUID string, documentID uint64) {
	ctx := context.Background()
	if err := s.taskRepo.MarkRunning(ctx, taskUID); err != nil {
		s.log.Error("mark index task running failed", zap.String("task_id", taskUID), zap.Error(err))
		return
	}

	indexed := 0
	err := s.indexDocument(ctx, documentID, func(done int, total int) error {
		indexed = done
		return s.taskRepo.UpdateProgress(ctx, taskUID, done)
	})
	if err != nil {
		_ = s.taskRepo.MarkFailed(ctx, taskUID, indexed, err.Error())
		s.log.Error("index task failed",
			zap.String("task_id", taskUID),
			zap.Uint64("document_id", documentID),
			zap.Error(err),
		)
		return
	}
	_ = s.taskRepo.MarkSucceeded(ctx, taskUID, indexed)
}

func (s *Service) indexDocument(ctx context.Context, documentID uint64, onProgress func(done int, total int) error) error {
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
	s.log.Info("index document started",
		zap.Uint64("document_id", documentID),
		zap.Int("chunk_count", len(chunks)),
	)

	if err := s.docRepo.UpdateParseResult(ctx, documentID, document.StatusIndexing, len(chunks), ""); err != nil {
		return fmt.Errorf("mark document indexing: %w", err)
	}
	if err := s.milvus.EnsureCollection(ctx); err != nil {
		_ = s.docRepo.UpdateParseResult(ctx, documentID, document.StatusFailed, len(chunks), err.Error())
		return err
	}
	s.log.Info("milvus collection ready", zap.Uint64("document_id", documentID))

	// text-embedding-v4 的 OpenAI 兼容接口单次最多 10 条文本。
	const batchSize = 10
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		s.log.Info("index chunk batch",
			zap.Uint64("document_id", documentID),
			zap.Int("start", start),
			zap.Int("end", end),
			zap.Int("total", len(chunks)),
		)

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
			// 每次索引都按 title_path 重新推断 chunk 分类，修正旧数据或汇总文档误标。
			category := markdown.InferCategory(chunk.TitlePath, doc.Category)
			if category != chunk.Category {
				if err := s.chunkRepo.UpdateCategory(ctx, chunk.ID, category); err != nil {
					return fmt.Errorf("update chunk category: %w", err)
				}
			}
			items = append(items, vectorstore.ChunkVector{
				ChunkUID:   chunk.ChunkUID,
				DocumentID: int64(chunk.DocumentID),
				ChunkID:    int64(chunk.ID),
				UserID:     int64(doc.UserID),
				TitlePath:  chunk.TitlePath,
				Content:    chunk.Content,
				Category:   category,
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
		if onProgress != nil {
			if err := onProgress(end, len(chunks)); err != nil {
				return fmt.Errorf("update index progress: %w", err)
			}
		}
	}

	if err := s.milvus.Flush(ctx); err != nil {
		_ = s.docRepo.UpdateParseResult(ctx, documentID, document.StatusFailed, len(chunks), err.Error())
		return err
	}
	s.log.Info("index document finished", zap.Uint64("document_id", documentID))

	if err := s.docRepo.UpdateParseResult(ctx, documentID, document.StatusIndexed, len(chunks), ""); err != nil {
		return fmt.Errorf("mark document indexed: %w", err)
	}
	return nil
}

func newTaskUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "task_unknown"
	}
	return "task_" + hex.EncodeToString(b[:])
}
