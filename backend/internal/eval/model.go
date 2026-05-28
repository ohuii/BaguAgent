package eval

import "time"

// RAGEvalCase 是一条人工标注的检索评测用例。
// expected_chunk_uids 用 JSON 数组保存应该命中的 chunk_uid。
type RAGEvalCase struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	Question          string    `gorm:"type:text;not null" json:"question"`
	ExpectedChunkUIDs string    `gorm:"type:json;not null" json:"expected_chunk_uids"`
	Category          string    `gorm:"size:64;index" json:"category"`
	Difficulty        string    `gorm:"size:32" json:"difficulty"`
	CreatedAt         time.Time `json:"created_at"`
}

// TableName 固定 RAG 评测用例表名。
func (RAGEvalCase) TableName() string {
	return "rag_eval_cases"
}

// RAGEvalResult 保存一次评测运行的指标结果。
// 第一版先计算 Recall@K、MRR、Hit，后续可接 LLM-as-Judge 分数。
type RAGEvalResult struct {
	ID                 uint64    `gorm:"primaryKey" json:"id"`
	EvalCaseID         uint64    `gorm:"index;not null" json:"eval_case_id"`
	TopK               int       `gorm:"not null" json:"top_k"`
	RetrievedChunkUIDs string    `gorm:"type:json;not null" json:"retrieved_chunk_uids"`
	Hit                bool      `gorm:"not null" json:"hit"`
	RecallAtK          float64   `gorm:"not null" json:"recall_at_k"`
	MRR                float64   `gorm:"not null" json:"mrr"`
	Answer             string    `gorm:"type:longtext" json:"answer"`
	FaithfulnessScore  float64   `json:"faithfulness_score"`
	RelevanceScore     float64   `json:"relevance_score"`
	CitationScore      float64   `json:"citation_score"`
	CreatedAt          time.Time `json:"created_at"`
}

// TableName 固定 RAG 评测结果表名。
func (RAGEvalResult) TableName() string {
	return "rag_eval_results"
}
