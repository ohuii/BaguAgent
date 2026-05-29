package rag

import (
	"context"
	"testing"
)

func TestEinoAssessBranch(t *testing.T) {
	s := &Service{}
	cases := []struct {
		name  string
		state einoInterviewGraphState
		want  string
	}{
		{
			name:  "sufficient goes to answer",
			state: einoInterviewGraphState{RetrievalSufficient: true},
			want:  einoNodeInterviewAnswer,
		},
		{
			name:  "mismatch and not retried goes to retry",
			state: einoInterviewGraphState{RetrievalSufficient: false, CategoryMismatch: true, Retried: false},
			want:  einoNodeRetrySearch,
		},
		{
			name:  "mismatch but already retried goes to no context",
			state: einoInterviewGraphState{RetrievalSufficient: false, CategoryMismatch: true, Retried: true},
			want:  einoNodeNoContext,
		},
		{
			name:  "insufficient without mismatch goes to no context",
			state: einoInterviewGraphState{RetrievalSufficient: false, CategoryMismatch: false},
			want:  einoNodeNoContext,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.einoAssessBranch(context.Background(), tc.state)
			if err != nil {
				t.Fatalf("einoAssessBranch returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("einoAssessBranch = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompileEinoInterviewGraph(t *testing.T) {
	s := &Service{}
	runnable, err := s.compileEinoInterviewGraph(context.Background())
	if err != nil {
		t.Fatalf("compileEinoInterviewGraph returned error: %v", err)
	}
	if runnable == nil {
		t.Fatal("compileEinoInterviewGraph returned nil runnable")
	}
}
