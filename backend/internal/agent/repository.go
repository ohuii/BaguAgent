package agent

import (
	"context"

	"gorm.io/gorm"
)

// Repository 封装 agent_runs 表的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 AgentRun repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 保存一次 Agent/RAG 执行记录。
func (r *Repository) Create(ctx context.Context, run *AgentRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}
