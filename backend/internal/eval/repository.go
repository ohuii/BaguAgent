package eval

import (
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 RAG 评测 repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateCase 新增一条人工标注的评测用例。
func (r *Repository) CreateCase(ctx context.Context, item *RAGEvalCase) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// GetCaseByID 按主键查询单条评测用例。
func (r *Repository) GetCaseByID(ctx context.Context, id uint64) (*RAGEvalCase, error) {
	var item RAGEvalCase
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// ListCases 查询评测用例列表，支持按 id 集合和分类筛选。
func (r *Repository) ListCases(ctx context.Context, filter CaseFilter) ([]RAGEvalCase, error) {
	var items []RAGEvalCase
	query := r.db.WithContext(ctx).Model(&RAGEvalCase{})
	if len(filter.IDs) > 0 {
		query = query.Where("id IN ?", filter.IDs)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	err := query.Order("id DESC").Find(&items).Error
	return items, err
}

// CreateResult 保存一次评测用例运行后的指标结果。
func (r *Repository) CreateResult(ctx context.Context, item *RAGEvalResult) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// ListResults 查询评测结果列表，默认最多返回最近 50 条。
func (r *Repository) ListResults(ctx context.Context, filter ResultFilter) ([]RAGEvalResult, error) {
	var items []RAGEvalResult
	query := r.db.WithContext(ctx).Model(&RAGEvalResult{})
	if filter.EvalCaseID > 0 {
		query = query.Where("eval_case_id = ?", filter.EvalCaseID)
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	err := query.Order("id DESC").Limit(filter.Limit).Find(&items).Error
	return items, err
}
