package chunk

import "time"

// DocumentChunk 保存 Markdown 语义切分后的文本块。
// content_with_title 会用于 embedding，title_path 和 milvus_pk 用于引用和检索追踪。
type DocumentChunk struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	DocumentID       uint64    `gorm:"index;not null" json:"document_id"`
	ChunkUID         string    `gorm:"size:64;uniqueIndex;not null" json:"chunk_uid"`
	TitlePath        string    `gorm:"size:1024;index;not null" json:"title_path"`
	HeadingLevel     int       `gorm:"not null" json:"heading_level"`
	Content          string    `gorm:"type:longtext;not null" json:"content"`
	ContentWithTitle string    `gorm:"type:longtext;not null" json:"content_with_title"`
	ChunkIndex       int       `gorm:"not null" json:"chunk_index"`
	TokenCount       int       `gorm:"not null" json:"token_count"`
	MilvusCollection string    `gorm:"size:128" json:"milvus_collection"`
	MilvusPK         string    `gorm:"size:128;index" json:"milvus_pk"`
	CreatedAt        time.Time `json:"created_at"`
}

// TableName 固定 chunk 表名。
func (DocumentChunk) TableName() string {
	return "document_chunks"
}
