package message

import (
	"context"

	"gorm.io/gorm"
)

// Repository 封装 messages 表的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建消息 repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 新建消息。
func (r *Repository) Create(ctx context.Context, msg *Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// ListByConversationID 查询会话消息。
func (r *Repository) ListByConversationID(ctx context.Context, conversationID uint64) ([]Message, error) {
	var messages []Message
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&messages).Error
	return messages, err
}
