package tool

import (
	"context"
	"fmt"

	"bagu-agent/backend/internal/llm"
	"bagu-agent/backend/internal/prompt"
	"bagu-agent/backend/internal/vectorstore"
)

// InterviewAnswerInput 是面试回答工具入参。
type InterviewAnswerInput struct {
	Question       string                     `json:"question"`
	Chunks         []vectorstore.SearchResult `json:"chunks"`
	HistorySummary string                     `json:"history_summary"`
}

// InterviewAnswerOutput 是面试回答工具出参。
type InterviewAnswerOutput struct {
	Answer string `json:"answer"`
}

// InterviewAnswerTool 根据检索片段生成结构化面试回答。
type InterviewAnswerTool struct {
	llm llm.Client
}

// NewInterviewAnswerTool 创建面试回答工具。
func NewInterviewAnswerTool(llm llm.Client) *InterviewAnswerTool {
	return &InterviewAnswerTool{llm: llm}
}

func (t *InterviewAnswerTool) Name() string {
	return "InterviewAnswerTool"
}

func (t *InterviewAnswerTool) Description() string {
	return "基于检索到的 chunks 生成结构化、面试化回答。"
}

func (t *InterviewAnswerTool) Call(ctx context.Context, input any) (any, error) {
	in, ok := input.(InterviewAnswerInput)
	if !ok {
		return nil, fmt.Errorf("invalid InterviewAnswerTool input")
	}
	ragPrompt := prompt.BuildRAGPrompt(in.Question, in.Chunks, in.HistorySummary)
	answer, err := t.llm.Generate(ctx, ragPrompt)
	if err != nil {
		return nil, err
	}
	return InterviewAnswerOutput{Answer: answer}, nil
}

// Stream 使用同一套 RAG prompt 进行流式回答，避免 service 层重复拼 prompt。
func (t *InterviewAnswerTool) Stream(ctx context.Context, input InterviewAnswerInput, onDelta func(delta string) error) error {
	ragPrompt := prompt.BuildRAGPrompt(input.Question, input.Chunks, input.HistorySummary)
	if err := t.llm.Stream(ctx, ragPrompt, onDelta); err != nil {
		return fmt.Errorf("stream answer: %w", err)
	}
	return nil
}
