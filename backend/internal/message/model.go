package message

import "time"

// 消息角色沿用常见 ChatML 风格，方便后续接入不同 LLM。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Message 保存会话中的用户问题和助手回答。
// CitationsJSON 保存答案引用的 chunk 来源，便于前端展示和质量评测。
type Message struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	ConversationID uint64    `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:32;not null" json:"role"`
	Content        string    `gorm:"type:longtext;not null" json:"content"`
	CitationsJSON  *string   `gorm:"type:json" json:"citations_json"`
	CreatedAt      time.Time `json:"created_at"`
}

// TableName 固定消息表名。
func (Message) TableName() string {
	return "messages"
}
