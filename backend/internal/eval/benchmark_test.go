package eval

import (
	"context"
	"testing"

	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"
)

type fakeSearcher struct {
	byQuery map[string][]string
}

func (f fakeSearcher) Search(_ context.Context, input retriever.SearchInput) ([]vectorstore.SearchResult, error) {
	uids := f.byQuery[input.Query]
	results := make([]vectorstore.SearchResult, 0, len(uids))
	for _, uid := range uids {
		results = append(results, vectorstore.SearchResult{ChunkUID: uid})
	}
	return results, nil
}

func TestBenchmarkAggregatesAcrossK(t *testing.T) {
	searcher := fakeSearcher{byQuery: map[string][]string{
		"qa": {"a1", "x", "y"},  // 命中在第 1 位
		"qb": {"z", "b2", "b1"}, // 第 1 位未命中，前 3 位命中两条
	}}
	cases := []BenchmarkCase{
		{Question: "qa", ExpectedChunkUIDs: []string{"a1"}},
		{Question: "qb", ExpectedChunkUIDs: []string{"b1", "b2"}},
	}

	report, err := Benchmark(context.Background(), searcher, 1, cases, []int{3, 1})
	if err != nil {
		t.Fatalf("Benchmark error: %v", err)
	}
	if report.CaseCount != 2 {
		t.Fatalf("CaseCount = %d, want 2", report.CaseCount)
	}
	if len(report.Ks) != 2 {
		t.Fatalf("len(Ks) = %d, want 2", len(report.Ks))
	}

	// normalizeKs 应升序：k=1 在前，k=3 在后。
	k1 := report.Ks[0]
	if k1.K != 1 {
		t.Fatalf("Ks[0].K = %d, want 1", k1.K)
	}
	assertFloat(t, "k1.HitRate", k1.HitRate, 0.5)
	assertFloat(t, "k1.RecallAtK", k1.RecallAtK, 0.5)
	assertFloat(t, "k1.MRR", k1.MRR, 0.5)

	k3 := report.Ks[1]
	if k3.K != 3 {
		t.Fatalf("Ks[1].K = %d, want 3", k3.K)
	}
	assertFloat(t, "k3.HitRate", k3.HitRate, 1.0)
	assertFloat(t, "k3.RecallAtK", k3.RecallAtK, 1.0)
	assertFloat(t, "k3.MRR", k3.MRR, 0.75)
}

func TestBenchmarkRejectsEmptyCases(t *testing.T) {
	if _, err := Benchmark(context.Background(), fakeSearcher{}, 1, nil, []int{5}); err == nil {
		t.Fatal("expected error for empty cases")
	}
}

func TestBenchmarkRejectsUnlabeledCase(t *testing.T) {
	cases := []BenchmarkCase{{Question: "q", ExpectedChunkUIDs: nil}}
	if _, err := Benchmark(context.Background(), fakeSearcher{}, 1, cases, []int{5}); err == nil {
		t.Fatal("expected error for case without expected_chunk_uids")
	}
}

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
