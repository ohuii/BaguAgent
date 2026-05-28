package document

import (
	"context"

	"gorm.io/gorm"
)

// Repository 封装 documents 表的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 document repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 新建文档元信息。
func (r *Repository) Create(ctx context.Context, doc *Document) error {
	return r.db.WithContext(ctx).Create(doc).Error
}

// List 按用户和可选分类查询文档列表。
func (r *Repository) List(ctx context.Context, userID uint64, category string) ([]Document, error) {
	var docs []Document
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	err := query.Order("id DESC").Find(&docs).Error
	return docs, err
}

// GetByID 查询单个文档。
func (r *Repository) GetByID(ctx context.Context, id uint64) (*Document, error) {
	var doc Document
	if err := r.db.WithContext(ctx).First(&doc, id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

// UpdateParseResult 更新文档解析结果。
func (r *Repository) UpdateParseResult(ctx context.Context, id uint64, status string, chunkCount int, errorMessage string) error {
	return r.db.WithContext(ctx).
		Model(&Document{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":        status,
			"chunk_count":   chunkCount,
			"error_message": errorMessage,
		}).Error
}

// Delete 删除文档元信息。
func (r *Repository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Document{}, id).Error
}
