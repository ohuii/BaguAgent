package indexer

import "time"

const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusSucceeded = "succeeded"
	TaskStatusFailed    = "failed"
)

// IndexTask 记录一次文档向量化索引任务的进度。
type IndexTask struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	TaskUID       string     `gorm:"size:64;uniqueIndex;not null" json:"task_id"`
	DocumentID    uint64     `gorm:"index;not null" json:"document_id"`
	Status        string     `gorm:"size:32;index;not null" json:"status"`
	TotalChunks   int        `gorm:"not null;default:0" json:"total_chunks"`
	IndexedChunks int        `gorm:"not null;default:0" json:"indexed_chunks"`
	ErrorMessage  string     `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName 固定索引任务表名。
func (IndexTask) TableName() string {
	return "index_tasks"
}
