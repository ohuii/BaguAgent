package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"
)

// Searcher 抽象检索能力，便于离线基准跑批复用 *retriever.Service，同时方便单测注入假实现。
type Searcher interface {
	Search(ctx context.Context, input retriever.SearchInput) ([]vectorstore.SearchResult, error)
}

// BenchmarkCase 是一条人工标注的评测用例，对应数据集 JSON 的一项。
type BenchmarkCase struct {
	Question          string   `json:"question"`
	Category          string   `json:"category,omitempty"`
	ExpectedChunkUIDs []string `json:"expected_chunk_uids"`
}

// KMetric 是某个 TopK 档位下，整个数据集的平均检索指标。
type KMetric struct {
	K         int     `json:"k"`
	HitRate   float64 `json:"hit_rate"`
	RecallAtK float64 `json:"recall_at_k"`
	MRR       float64 `json:"mrr"`
}

// BenchmarkReport 汇总一次离线基准跑批的整体结果。
type BenchmarkReport struct {
	CaseCount int       `json:"case_count"`
	Ks        []KMetric `json:"ks"`
}

type metricSum struct {
	hit    int
	recall float64
	mrr    float64
}

// Benchmark 对同一组用例在多个 TopK 档位上评测检索质量。
// 每条用例只检索一次（按最大 K 召回），再对前缀切片计算各档指标，避免重复 embedding 调用。
func Benchmark(ctx context.Context, s Searcher, userID uint64, cases []BenchmarkCase, ks []int) (*BenchmarkReport, error) {
	if s == nil {
		return nil, fmt.Errorf("searcher is required")
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no benchmark cases")
	}
	ks = normalizeKs(ks)
	if len(ks) == 0 {
		ks = []int{5}
	}
	if userID == 0 {
		userID = 1
	}
	maxK := ks[len(ks)-1]

	sums := make(map[int]*metricSum, len(ks))
	for _, k := range ks {
		sums[k] = &metricSum{}
	}

	for _, c := range cases {
		expected := normalizeUIDs(c.ExpectedChunkUIDs)
		if len(expected) == 0 {
			return nil, fmt.Errorf("case %q has no expected_chunk_uids", c.Question)
		}
		chunks, err := s.Search(ctx, retriever.SearchInput{
			UserID:   userID,
			Query:    c.Question,
			Category: c.Category,
			TopK:     maxK,
		})
		if err != nil {
			return nil, fmt.Errorf("search %q: %w", c.Question, err)
		}
		retrieved := chunkUIDs(chunks)
		for _, k := range ks {
			top := retrieved
			if len(top) > k {
				top = top[:k]
			}
			m := calculateMetrics(expected, top)
			acc := sums[k]
			if m.Hit {
				acc.hit++
			}
			acc.recall += m.RecallAtK
			acc.mrr += m.MRR
		}
	}

	n := float64(len(cases))
	report := &BenchmarkReport{CaseCount: len(cases)}
	for _, k := range ks {
		acc := sums[k]
		report.Ks = append(report.Ks, KMetric{
			K:         k,
			HitRate:   float64(acc.hit) / n,
			RecallAtK: acc.recall / n,
			MRR:       acc.mrr / n,
		})
	}
	return report, nil
}

// Markdown 把报告渲染成可直接贴进文档或简历的表格。
func (r *BenchmarkReport) Markdown() string {
	var b strings.Builder
	b.WriteString("## RAG 检索评测报告\n\n")
	fmt.Fprintf(&b, "- 用例数：%d\n\n", r.CaseCount)
	b.WriteString("| TopK | HitRate | Recall@K | MRR |\n")
	b.WriteString("|------|---------|----------|-----|\n")
	for _, k := range r.Ks {
		fmt.Fprintf(&b, "| %d | %.3f | %.3f | %.3f |\n", k.K, k.HitRate, k.RecallAtK, k.MRR)
	}
	return b.String()
}

// normalizeKs 去掉非正数、去重并升序排列。
func normalizeKs(ks []int) []int {
	seen := make(map[int]struct{}, len(ks))
	out := make([]int, 0, len(ks))
	for _, k := range ks {
		if k <= 0 {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
