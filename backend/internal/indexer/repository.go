package indexer

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Repository 封装 index_tasks 表的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建索引任务 repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 新建索引任务。
func (r *Repository) Create(ctx context.Context, task *IndexTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByTaskUID 查询索引任务。
func (r *Repository) GetByTaskUID(ctx context.Context, taskUID string) (*IndexTask, error) {
	var task IndexTask
	if err := r.db.WithContext(ctx).Where("task_uid = ?", taskUID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// MarkRunning 标记任务开始运行。
func (r *Repository) MarkRunning(ctx context.Context, taskUID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&IndexTask{}).
		Where("task_uid = ?", taskUID).
		Updates(map[string]any{
			"status":     TaskStatusRunning,
			"started_at": now,
		}).Error
}

// UpdateProgress 更新任务进度。
func (r *Repository) UpdateProgress(ctx context.Context, taskUID string, indexed int) error {
	return r.db.WithContext(ctx).
		Model(&IndexTask{}).
		Where("task_uid = ?", taskUID).
		Update("indexed_chunks", indexed).Error
}

// MarkSucceeded 标记任务成功完成。
func (r *Repository) MarkSucceeded(ctx context.Context, taskUID string, total int) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&IndexTask{}).
		Where("task_uid = ?", taskUID).
		Updates(map[string]any{
			"status":         TaskStatusSucceeded,
			"indexed_chunks": total,
			"finished_at":    now,
			"error_message":  "",
		}).Error
}

// MarkFailed 标记任务失败。
func (r *Repository) MarkFailed(ctx context.Context, taskUID string, indexed int, message string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&IndexTask{}).
		Where("task_uid = ?", taskUID).
		Updates(map[string]any{
			"status":         TaskStatusFailed,
			"indexed_chunks": indexed,
			"finished_at":    now,
			"error_message":  message,
		}).Error
}
