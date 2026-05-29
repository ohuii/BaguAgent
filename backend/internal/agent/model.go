package agent

import "time"

// AgentRun 记录一次 Agent/RAG 问答的执行轨迹。
// 它比 messages 更偏观测和评测，包含意图、工具、召回 chunk 和耗时。
type AgentRun struct {
	ID                  uint64    `gorm:"primaryKey" json:"id"`
	ConversationID      uint64    `gorm:"index;not null" json:"conversation_id"`
	MessageID           uint64    `gorm:"index;not null" json:"message_id"`
	UserQuery           string    `gorm:"type:text;not null" json:"user_query"`
	Intent              string    `gorm:"size:64;not null" json:"intent"`
	ToolsUsed           *string   `gorm:"type:json" json:"tools_used"`
	AgentStepsJSON      *string   `gorm:"type:json" json:"agent_steps_json"`
	RetrievedChunksJSON *string   `gorm:"type:json" json:"retrieved_chunks_json"`
	FinalAnswer         string    `gorm:"type:longtext" json:"final_answer"`
	LatencyMS           int64     `gorm:"not null;default:0" json:"latency_ms"`
	CreatedAt           time.Time `json:"created_at"`
}

// TableName 固定 Agent 运行记录表名。
func (AgentRun) TableName() string {
	return "agent_runs"
}
