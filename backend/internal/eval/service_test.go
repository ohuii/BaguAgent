package eval

import "testing"

// TestCalculateMetrics 覆盖 RAG 检索评测最核心的三个指标。
func TestCalculateMetrics(t *testing.T) {
	tests := []struct {
		name          string
		expected      []string
		retrieved     []string
		wantHit       bool
		wantRecallAtK float64
		wantMRR       float64
	}{
		{
			name:          "hit at first rank",
			expected:      []string{"c1"},
			retrieved:     []string{"c1", "c2"},
			wantHit:       true,
			wantRecallAtK: 1,
			wantMRR:       1,
		},
		{
			name:          "hit at second rank",
			expected:      []string{"c1", "c3"},
			retrieved:     []string{"c2", "c3", "c4"},
			wantHit:       true,
			wantRecallAtK: 0.5,
			wantMRR:       0.5,
		},
		{
			name:          "deduplicate expected and retrieved matches",
			expected:      []string{"c1", "c1", "c2"},
			retrieved:     []string{"c1", "c1", "c3"},
			wantHit:       true,
			wantRecallAtK: 0.5,
			wantMRR:       1,
		},
		{
			name:          "miss",
			expected:      []string{"c1"},
			retrieved:     []string{"c2", "c3"},
			wantHit:       false,
			wantRecallAtK: 0,
			wantMRR:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMetrics(tt.expected, tt.retrieved)
			if got.Hit != tt.wantHit {
				t.Fatalf("Hit = %v, want %v", got.Hit, tt.wantHit)
			}
			if got.RecallAtK != tt.wantRecallAtK {
				t.Fatalf("RecallAtK = %v, want %v", got.RecallAtK, tt.wantRecallAtK)
			}
			if got.MRR != tt.wantMRR {
				t.Fatalf("MRR = %v, want %v", got.MRR, tt.wantMRR)
			}
		})
	}
}
