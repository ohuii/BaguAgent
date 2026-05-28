package chunk

import (
	"context"

	"gorm.io/gorm"
)

// Repository 封装 document_chunks 表的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 chunk repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateBatch 批量保存 chunk，适合文档解析后的整批入库。
func (r *Repository) CreateBatch(ctx context.Context, chunks []*DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(chunks, 100).Error
}

// ListByDocumentID 查询某个文档下的所有 chunk。
func (r *Repository) ListByDocumentID(ctx context.Context, documentID uint64) ([]DocumentChunk, error) {
	var chunks []DocumentChunk
	err := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Order("chunk_index ASC").
		Find(&chunks).Error
	return chunks, err
}

// UpdateMilvusFields 回写 chunk 在 Milvus 中的集合名和主键。
func (r *Repository) UpdateMilvusFields(ctx context.Context, id uint64, collection string, pk string) error {
	return r.db.WithContext(ctx).
		Model(&DocumentChunk{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"milvus_collection": collection,
			"milvus_pk":         pk,
		}).Error
}

// DeleteByDocumentID 删除某个文档下的所有 chunk。
func (r *Repository) DeleteByDocumentID(ctx context.Context, documentID uint64) error {
	return r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Delete(&DocumentChunk{}).Error
}
