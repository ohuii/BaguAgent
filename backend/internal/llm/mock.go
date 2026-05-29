package llm

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// MockClient 用检索片段拼出结构化回答，只用于本地联调。
type MockClient struct{}

// NewMock 创建 mock LLM。
func NewMock() *MockClient {
	return &MockClient{}
}

// Generate 返回一个稳定格式的面试回答，便于前端和接口联调。
func (c *MockClient) Generate(ctx context.Context, prompt string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	question := extractSection(prompt, "用户问题：", "历史对话摘要：")
	firstChunk := extractFirstChunk(prompt)
	if firstChunk == "" {
		firstChunk = "知识库中没有检索到足够片段，当前只能给出有限回答。"
	}
	brief := firstSentence(firstChunk)

	var b strings.Builder
	b.WriteString("## 一句话回答\n\n")
	b.WriteString(brief)
	b.WriteString("\n\n## 核心原理\n\n")
	b.WriteString("- 基于当前知识库检索结果回答问题。\n")
	b.WriteString("- 真实大模型接入后，这里会进一步综合多个片段并组织语言。\n")
	b.WriteString("\n## 面试回答\n\n")
	b.WriteString("针对问题「")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("」，可以这样回答：\n\n")
	b.WriteString(firstChunk)
	b.WriteString("\n\n## 常见追问\n\n")
	b.WriteString("1. 这个概念解决了什么问题？\n")
	b.WriteString("2. 核心组件分别承担什么职责？\n")
	b.WriteString("3. 在实际项目或源码中如何体现？\n")
	b.WriteString("\n## 常见误区\n\n")
	b.WriteString("- 不要脱离知识库片段随意扩展结论。\n")
	b.WriteString("- 注意区分概念定义、运行机制和面试表达。\n")
	b.WriteString("\n## 引用来源\n\n")
	b.WriteString("- 见接口返回的 citations 字段。\n")
	return b.String(), nil
}

// Stream 分段返回 mock 生成结果，模拟真实大模型流式输出。
func (c *MockClient) Stream(ctx context.Context, prompt string, onDelta func(delta string) error) error {
	answer, err := c.Generate(ctx, prompt)
	if err != nil {
		return err
	}

	runes := []rune(answer)
	const step = 24
	for start := 0; start < len(runes); start += step {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := start + step
		if end > len(runes) {
			end = len(runes)
		}
		if err := onDelta(string(runes[start:end])); err != nil {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

func extractSection(s, start, end string) string {
	startIdx := strings.Index(s, start)
	if startIdx < 0 {
		return ""
	}
	startIdx += len(start)
	endIdx := strings.Index(s[startIdx:], end)
	if endIdx < 0 {
		return strings.TrimSpace(s[startIdx:])
	}
	return strings.TrimSpace(s[startIdx : startIdx+endIdx])
}

func extractFirstChunk(prompt string) string {
	re := regexp.MustCompile(`(?s)内容：\n(.+?)(\n\n\[|\n\n请按以下格式输出：)`)
	matches := re.FindStringSubmatch(prompt)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "知识库中没有足够信息。"
	}
	for _, sep := range []string{"。", "\n", ".", "！", "？"} {
		if idx := strings.Index(s, sep); idx >= 0 {
			return strings.TrimSpace(s[:idx+len(sep)])
		}
	}
	if len([]rune(s)) > 120 {
		return string([]rune(s)[:120]) + "..."
	}
	return s
}
