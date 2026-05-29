package tool

import (
	"context"
	"fmt"
	"strings"

	"bagu-agent/backend/internal/llm"
	"bagu-agent/backend/internal/vectorstore"
)

// QuestionGenerateInput 是面试题生成工具入参。
type QuestionGenerateInput struct {
	Topic    string                     `json:"topic"`
	Category string                     `json:"category"`
	Count    int                        `json:"count"`
	Chunks   []vectorstore.SearchResult `json:"chunks"`
}

// QuestionGenerateOutput 是面试题生成工具出参。
type QuestionGenerateOutput struct {
	Content string `json:"content"`
}

// QuestionGenerateTool 根据知识片段生成面试题、参考答案和考察点。
type QuestionGenerateTool struct {
	llm llm.Client
}

// NewQuestionGenerateTool 创建面试题生成工具。
func NewQuestionGenerateTool(llm llm.Client) *QuestionGenerateTool {
	return &QuestionGenerateTool{llm: llm}
}

func (t *QuestionGenerateTool) Name() string {
	return "QuestionGenerateTool"
}

func (t *QuestionGenerateTool) Description() string {
	return "根据某个知识点或章节生成面试题、参考答案和考察点。"
}

func (t *QuestionGenerateTool) Call(ctx context.Context, input any) (any, error) {
	in, ok := input.(QuestionGenerateInput)
	if !ok {
		return nil, fmt.Errorf("invalid QuestionGenerateTool input")
	}
	if in.Count <= 0 {
		in.Count = 5
	}

	prompt := buildQuestionGeneratePrompt(in)
	content, err := t.llm.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return QuestionGenerateOutput{Content: content}, nil
}

func buildQuestionGeneratePrompt(in QuestionGenerateInput) string {
	var b strings.Builder
	b.WriteString("你是一个资深后端面试官。请基于给定知识库片段生成程序员面试题。\n\n")
	b.WriteString("要求：\n")
	b.WriteString("1. 题目要贴近真实面试。\n")
	b.WriteString("2. 每道题给出参考答案和考察点。\n")
	b.WriteString("3. 不要编造知识库中没有的概念。\n\n")
	b.WriteString(fmt.Sprintf("主题：%s\n分类：%s\n题目数量：%d\n\n", in.Topic, in.Category, in.Count))
	b.WriteString("知识库片段：\n")
	for i, chunk := range in.Chunks {
		b.WriteString(fmt.Sprintf("[%d] %s\n%s\n\n", i+1, chunk.TitlePath, chunk.Content))
	}
	b.WriteString("请按以下格式输出：\n\n")
	b.WriteString("## 题目 1\n\n### 参考答案\n\n### 考察点\n")
	return b.String()
}
