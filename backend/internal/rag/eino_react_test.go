package rag

import (
	"context"
	"testing"

	"bagu-agent/backend/internal/llm"
)

func TestReactInterviewAgentCompiles(t *testing.T) {
	s := &Service{toolCallingModel: llm.NewMockToolCallingModel()}
	agent, err := s.reactInterviewAgent(context.Background())
	if err != nil {
		t.Fatalf("reactInterviewAgent error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected non-nil react agent")
	}
}

func TestReactInterviewAgentRequiresModel(t *testing.T) {
	s := &Service{}
	if _, err := s.reactInterviewAgent(context.Background()); err == nil {
		t.Fatal("expected error when tool calling model is missing")
	}
}
