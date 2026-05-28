package document

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	chunkmodel "bagu-agent/backend/internal/chunk"
	"bagu-agent/backend/internal/config"
	"bagu-agent/backend/internal/markdown"
	"bagu-agent/backend/internal/pkg/id"
)

// Service 编排文档上传、Markdown 解析和 chunk 入库流程。
type Service struct {
	storageCfg config.StorageConfig
	docRepo    *Repository
	chunkRepo  *chunkmodel.Repository
}

// NewService 创建文档服务。
func NewService(storageCfg config.StorageConfig, docRepo *Repository, chunkRepo *chunkmodel.Repository) *Service {
	return &Service{
		storageCfg: storageCfg,
		docRepo:    docRepo,
		chunkRepo:  chunkRepo,
	}
}

// UploadMarkdownInput 是 Markdown 上传入参。
type UploadMarkdownInput struct {
	UserID   uint64
	Category string
	File     *multipart.FileHeader
}

// UploadMarkdownResult 是 Markdown 上传和解析结果。
type UploadMarkdownResult struct {
	Document Document                   `json:"document"`
	Chunks   []chunkmodel.DocumentChunk `json:"chunks"`
}

// UploadMarkdown 保存 Markdown 文件，解析标题层级并把 chunk 写入 MySQL。
func (s *Service) UploadMarkdown(ctx context.Context, input UploadMarkdownInput) (*UploadMarkdownResult, error) {
	if input.UserID == 0 {
		input.UserID = 1
	}
	if input.File == nil {
		return nil, fmt.Errorf("file is required")
	}
	if !isMarkdownFile(input.File.Filename) {
		return nil, fmt.Errorf("only markdown files are supported")
	}

	content, err := readUploadedFile(input.File)
	if err != nil {
		return nil, err
	}

	sourcePath, err := s.saveUploadedFile(input.UserID, input.File.Filename, content)
	if err != nil {
		return nil, err
	}

	doc := &Document{
		UserID:     input.UserID,
		Name:       filepath.Base(input.File.Filename),
		SourceType: "markdown",
		SourcePath: sourcePath,
		Category:   strings.TrimSpace(input.Category),
		Status:     StatusUploaded,
	}
	if err := s.docRepo.Create(ctx, doc); err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	parsedChunks, err := markdown.ChunkMarkdown(content, doc.Name, doc.Category, markdown.DefaultChunkerOptions())
	if err != nil {
		_ = s.docRepo.UpdateParseResult(ctx, doc.ID, StatusFailed, 0, err.Error())
		return nil, fmt.Errorf("chunk markdown: %w", err)
	}

	dbChunks := make([]*chunkmodel.DocumentChunk, 0, len(parsedChunks))
	for _, parsed := range parsedChunks {
		dbChunks = append(dbChunks, &chunkmodel.DocumentChunk{
			DocumentID:       doc.ID,
			ChunkUID:         id.NewChunkUID(),
			TitlePath:        parsed.TitlePath,
			HeadingLevel:     parsed.HeadingLevel,
			Content:          parsed.Content,
			ContentWithTitle: parsed.ContentWithTitle,
			ChunkIndex:       parsed.ChunkIndex,
			TokenCount:       parsed.TokenCount,
		})
	}

	if err := s.chunkRepo.CreateBatch(ctx, dbChunks); err != nil {
		_ = s.docRepo.UpdateParseResult(ctx, doc.ID, StatusFailed, 0, err.Error())
		return nil, fmt.Errorf("create chunks: %w", err)
	}

	if err := s.docRepo.UpdateParseResult(ctx, doc.ID, StatusParsed, len(dbChunks), ""); err != nil {
		return nil, fmt.Errorf("update document parse result: %w", err)
	}
	doc.Status = StatusParsed
	doc.ChunkCount = len(dbChunks)

	chunks := make([]chunkmodel.DocumentChunk, 0, len(dbChunks))
	for _, c := range dbChunks {
		chunks = append(chunks, *c)
	}
	return &UploadMarkdownResult{
		Document: *doc,
		Chunks:   chunks,
	}, nil
}

// List 查询文档列表。
func (s *Service) List(ctx context.Context, userID uint64, category string) ([]Document, error) {
	if userID == 0 {
		userID = 1
	}
	return s.docRepo.List(ctx, userID, category)
}

// Get 查询单个文档。
func (s *Service) Get(ctx context.Context, id uint64) (*Document, error) {
	return s.docRepo.GetByID(ctx, id)
}

// ListChunks 查询文档 chunk 列表。
func (s *Service) ListChunks(ctx context.Context, documentID uint64) ([]chunkmodel.DocumentChunk, error) {
	return s.chunkRepo.ListByDocumentID(ctx, documentID)
}

// Delete 删除文档和对应 chunk。Milvus 删除会在第三阶段补上。
func (s *Service) Delete(ctx context.Context, id uint64) error {
	if err := s.chunkRepo.DeleteByDocumentID(ctx, id); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if err := s.docRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func readUploadedFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read upload file: %w", err)
	}
	return content, nil
}

func (s *Service) saveUploadedFile(userID uint64, filename string, content []byte) (string, error) {
	uploadDir := s.storageCfg.UploadDir
	if uploadDir == "" {
		uploadDir = "uploads"
	}

	userDir := filepath.Join(uploadDir, "user_"+strconv.FormatUint(userID, 10))
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	safeName := sanitizeFilename(filename)
	target := filepath.Join(userDir, time.Now().Format("20060102150405")+"_"+safeName)
	if err := os.WriteFile(target, content, 0644); err != nil {
		return "", fmt.Errorf("save upload file: %w", err)
	}
	return target, nil
}

func isMarkdownFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown"
}

func sanitizeFilename(filename string) string {
	name := filepath.Base(filename)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}
