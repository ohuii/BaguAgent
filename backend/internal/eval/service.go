package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"
)

type CaseFilter struct {
	IDs      []uint64
	Category string
}

// ResultFilter 是评测结果列表查询条件。
type ResultFilter struct {
	EvalCaseID uint64
	Limit      int
}

// CreateCaseInput 是新增评测用例的请求参数。
type CreateCaseInput struct {
	Question          string   `json:"question"`
	ExpectedChunkUIDs []string `json:"expected_chunk_uids"`
	Category          string   `json:"category"`
	Difficulty        string   `json:"difficulty"`
}

// RunInput 是一次 RAG 检索评测运行的请求参数。
type RunInput struct {
	UserID   uint64   `json:"user_id"`
	CaseIDs  []uint64 `json:"case_ids"`
	Category string   `json:"category"`
	TopK     int      `json:"top_k"`
}

// CaseDTO 是对外返回的评测用例结构，将 JSON 字符串展开成数组。
type CaseDTO struct {
	ID                uint64   `json:"id"`
	Question          string   `json:"question"`
	ExpectedChunkUIDs []string `json:"expected_chunk_uids"`
	Category          string   `json:"category"`
	Difficulty        string   `json:"difficulty"`
}

// ResultDTO 是单条评测结果的对外结构。
type ResultDTO struct {
	ID                 uint64                     `json:"id,omitempty"`
	EvalCaseID         uint64                     `json:"eval_case_id"`
	Question           string                     `json:"question,omitempty"`
	ExpectedChunkUIDs  []string                   `json:"expected_chunk_uids,omitempty"`
	TopK               int                        `json:"top_k"`
	RetrievedChunkUIDs []string                   `json:"retrieved_chunk_uids"`
	Hit                bool                       `json:"hit"`
	RecallAtK          float64                    `json:"recall_at_k"`
	MRR                float64                    `json:"mrr"`
	RetrievedChunks    []vectorstore.SearchResult `json:"retrieved_chunks,omitempty"`
}

// RunResult 汇总一次评测运行的整体指标和逐题明细。
type RunResult struct {
	CaseCount int         `json:"case_count"`
	TopK      int         `json:"top_k"`
	HitRate   float64     `json:"hit_rate"`
	RecallAtK float64     `json:"recall_at_k"`
	MRR       float64     `json:"mrr"`
	Results   []ResultDTO `json:"results"`
}

// Service 编排评测用例管理、检索执行和指标计算。
type Service struct {
	repo      *Repository
	retriever *retriever.Service
}

// NewService 创建 RAG 评测服务。
func NewService(repo *Repository, retriever *retriever.Service) *Service {
	return &Service{repo: repo, retriever: retriever}
}

// CreateCase 创建一条人工标注的评测用例。
func (s *Service) CreateCase(ctx context.Context, input CreateCaseInput) (*CaseDTO, error) {
	input.Question = strings.TrimSpace(input.Question)
	if input.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	input.ExpectedChunkUIDs = normalizeUIDs(input.ExpectedChunkUIDs)
	if len(input.ExpectedChunkUIDs) == 0 {
		return nil, fmt.Errorf("expected_chunk_uids is required")
	}

	expectedJSON, err := marshalStringSlice(input.ExpectedChunkUIDs)
	if err != nil {
		return nil, err
	}
	item := &RAGEvalCase{
		Question:          input.Question,
		ExpectedChunkUIDs: expectedJSON,
		Category:          strings.TrimSpace(input.Category),
		Difficulty:        strings.TrimSpace(input.Difficulty),
	}
	if err := s.repo.CreateCase(ctx, item); err != nil {
		return nil, fmt.Errorf("create eval case: %w", err)
	}
	return caseToDTO(*item)
}

// ListCases 查询评测用例列表。
func (s *Service) ListCases(ctx context.Context, filter CaseFilter) ([]CaseDTO, error) {
	items, err := s.repo.ListCases(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list eval cases: %w", err)
	}
	out := make([]CaseDTO, 0, len(items))
	for _, item := range items {
		dto, err := caseToDTO(item)
		if err != nil {
			return nil, err
		}
		out = append(out, *dto)
	}
	return out, nil
}

// Run 执行一轮检索评测，并把每条用例的指标写入 rag_eval_results。
func (s *Service) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	if input.UserID == 0 {
		input.UserID = 1
	}
	if input.TopK <= 0 {
		input.TopK = 5
	}

	cases, err := s.repo.ListCases(ctx, CaseFilter{
		IDs:      input.CaseIDs,
		Category: strings.TrimSpace(input.Category),
	})
	if err != nil {
		return nil, fmt.Errorf("list eval cases: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no eval cases found")
	}

	results := make([]ResultDTO, 0, len(cases))
	var hitCount int
	var recallSum float64
	var mrrSum float64
	for _, evalCase := range cases {
		expected, err := unmarshalStringSlice(evalCase.ExpectedChunkUIDs)
		if err != nil {
			return nil, fmt.Errorf("parse expected chunks for case %d: %w", evalCase.ID, err)
		}
		category := evalCase.Category
		if category == "" {
			category = input.Category
		}
		// 评测第一版只衡量检索质量，不调用 LLM 生成答案，避免指标和生成质量混在一起。
		chunks, err := s.retriever.Search(ctx, retriever.SearchInput{
			UserID:   input.UserID,
			Query:    evalCase.Question,
			Category: category,
			TopK:     input.TopK,
		})
		if err != nil {
			return nil, fmt.Errorf("run eval case %d: %w", evalCase.ID, err)
		}

		retrievedUIDs := chunkUIDs(chunks)
		metrics := calculateMetrics(expected, retrievedUIDs)
		retrievedJSON, err := marshalStringSlice(retrievedUIDs)
		if err != nil {
			return nil, err
		}
		result := &RAGEvalResult{
			EvalCaseID:         evalCase.ID,
			TopK:               input.TopK,
			RetrievedChunkUIDs: retrievedJSON,
			Hit:                metrics.Hit,
			RecallAtK:          metrics.RecallAtK,
			MRR:                metrics.MRR,
		}
		if err := s.repo.CreateResult(ctx, result); err != nil {
			return nil, fmt.Errorf("save eval result for case %d: %w", evalCase.ID, err)
		}

		if metrics.Hit {
			hitCount++
		}
		recallSum += metrics.RecallAtK
		mrrSum += metrics.MRR
		results = append(results, ResultDTO{
			ID:                 result.ID,
			EvalCaseID:         evalCase.ID,
			Question:           evalCase.Question,
			ExpectedChunkUIDs:  expected,
			TopK:               input.TopK,
			RetrievedChunkUIDs: retrievedUIDs,
			Hit:                metrics.Hit,
			RecallAtK:          metrics.RecallAtK,
			MRR:                metrics.MRR,
			RetrievedChunks:    chunks,
		})
	}

	caseCount := len(cases)
	return &RunResult{
		CaseCount: caseCount,
		TopK:      input.TopK,
		HitRate:   float64(hitCount) / float64(caseCount),
		RecallAtK: recallSum / float64(caseCount),
		MRR:       mrrSum / float64(caseCount),
		Results:   results,
	}, nil
}

// ListResults 查询历史评测结果。
func (s *Service) ListResults(ctx context.Context, filter ResultFilter) ([]ResultDTO, error) {
	items, err := s.repo.ListResults(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list eval results: %w", err)
	}
	out := make([]ResultDTO, 0, len(items))
	for _, item := range items {
		retrieved, err := unmarshalStringSlice(item.RetrievedChunkUIDs)
		if err != nil {
			return nil, fmt.Errorf("parse retrieved chunks for result %d: %w", item.ID, err)
		}
		out = append(out, ResultDTO{
			ID:                 item.ID,
			EvalCaseID:         item.EvalCaseID,
			TopK:               item.TopK,
			RetrievedChunkUIDs: retrieved,
			Hit:                item.Hit,
			RecallAtK:          item.RecallAtK,
			MRR:                item.MRR,
		})
	}
	return out, nil
}

type metrics struct {
	Hit       bool
	RecallAtK float64
	MRR       float64
}

// calculateMetrics 根据人工标注的期望 chunk 和实际召回列表计算检索指标。
func calculateMetrics(expected []string, retrieved []string) metrics {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, uid := range normalizeUIDs(expected) {
		expectedSet[uid] = struct{}{}
	}
	if len(expectedSet) == 0 {
		return metrics{}
	}

	seenRelevant := make(map[string]struct{})
	var firstRelevantRank int
	for i, uid := range retrieved {
		if _, ok := expectedSet[uid]; !ok {
			continue
		}
		// MRR 只关心第一条相关结果的位置；Recall@K 则统计 TopK 内命中的去重相关 chunk 数。
		if firstRelevantRank == 0 {
			firstRelevantRank = i + 1
		}
		seenRelevant[uid] = struct{}{}
	}

	out := metrics{
		Hit:       len(seenRelevant) > 0,
		RecallAtK: float64(len(seenRelevant)) / float64(len(expectedSet)),
	}
	if firstRelevantRank > 0 {
		out.MRR = 1 / float64(firstRelevantRank)
	}
	return out
}

// caseToDTO 将数据库模型中的 JSON 字符串转换成 API 友好的数组字段。
func caseToDTO(item RAGEvalCase) (*CaseDTO, error) {
	expected, err := unmarshalStringSlice(item.ExpectedChunkUIDs)
	if err != nil {
		return nil, fmt.Errorf("parse expected chunks for case %d: %w", item.ID, err)
	}
	return &CaseDTO{
		ID:                item.ID,
		Question:          item.Question,
		ExpectedChunkUIDs: expected,
		Category:          item.Category,
		Difficulty:        item.Difficulty,
	}, nil
}

// chunkUIDs 从检索结果中提取 chunk_uid 列表。
func chunkUIDs(chunks []vectorstore.SearchResult) []string {
	uids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.ChunkUID == "" {
			continue
		}
		uids = append(uids, chunk.ChunkUID)
	}
	return uids
}

// normalizeUIDs 清理空字符串并去重，保证指标计算不被重复标注影响。
func normalizeUIDs(uids []string) []string {
	seen := make(map[string]struct{}, len(uids))
	out := make([]string, 0, len(uids))
	for _, uid := range uids {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		out = append(out, uid)
	}
	return out
}

// marshalStringSlice 将字符串数组序列化为 JSON，便于写入 MySQL JSON 字段。
func marshalStringSlice(items []string) (string, error) {
	b, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalStringSlice 将 MySQL JSON 字段反序列化为字符串数组。
func unmarshalStringSlice(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return normalizeUIDs(items), nil
}
