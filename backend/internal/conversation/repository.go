package conversation

import (
	"context"

	"gorm.io/gorm"
)

// Repository 封装 conversations 表的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建会话 repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 新建会话。
func (r *Repository) Create(ctx context.Context, conv *Conversation) error {
	return r.db.WithContext(ctx).Create(conv).Error
}

// GetByID 查询单个会话。
func (r *Repository) GetByID(ctx context.Context, id uint64) (*Conversation, error) {
	var conv Conversation
	if err := r.db.WithContext(ctx).First(&conv, id).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

// ListByUserID 查询用户会话列表。
func (r *Repository) ListByUserID(ctx context.Context, userID uint64) ([]Conversation, error) {
	var convs []Conversation
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").
		Find(&convs).Error
	return convs, err
}
