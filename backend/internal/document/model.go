package document

import "time"

// 文档索引状态用于描述 Markdown 从上传到向量化入库的生命周期。
const (
	StatusUploaded = "uploaded"
	StatusParsed   = "parsed"
	StatusIndexing = "indexing"
	StatusIndexed  = "indexed"
	StatusFailed   = "failed"
)

// Document 保存用户导入的原始文档元信息。
// Markdown 原文存文件系统或对象存储，数据库只保存路径和索引状态。
type Document struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	UserID       uint64    `gorm:"index;not null" json:"user_id"`
	Name         string    `gorm:"size:255;not null" json:"name"`
	SourceType   string    `gorm:"size:32;not null" json:"source_type"`
	SourcePath   string    `gorm:"size:512;not null" json:"source_path"`
	Category     string    `gorm:"size:64;index" json:"category"`
	Status       string    `gorm:"size:32;index;not null" json:"status"`
	ChunkCount   int       `gorm:"not null;default:0" json:"chunk_count"`
	ErrorMessage string    `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 固定文档表名。
func (Document) TableName() string {
	return "documents"
}
