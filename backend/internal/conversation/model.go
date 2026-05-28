package conversation

import "time"

// Conversation 表示一次面试复习对话。
// 后续可以按用户、分类或知识点生成会话标题。
type Conversation struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"index;not null" json:"user_id"`
	Title     string    `gorm:"size:255;not null" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定会话表名。
func (Conversation) TableName() string {
	return "conversations"
}
