package llm

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// mockToolCallingModel 在没有真实大模型时，用脚本化方式驱动 ReAct 循环：
// 第一轮先调用绑定的（检索）工具，拿到工具结果后再产出最终回答。
// 这样 mock provider 也能跑通原生 ReAct Agent 的端到端链路，方便本地联调。
type mockToolCallingModel struct {
	tools []*schema.ToolInfo
}

// NewMockToolCallingModel 创建 mock 的 ToolCallingChatModel。
func NewMockToolCallingModel() *mockToolCallingModel {
	return &mockToolCallingModel{}
}

func (m *mockToolCallingModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return &mockToolCallingModel{tools: tools}, nil
}

func (m *mockToolCallingModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	// 已经有工具结果回填，进入回答阶段。
	if hasToolResult(input) || len(m.tools) == 0 {
		return schema.AssistantMessage(m.buildAnswer(input), nil), nil
	}

	// 还没检索过：脚本化地调用第一个工具（约定为检索工具，入参 query）。
	args, _ := json.Marshal(map[string]string{"query": lastUserContent(input)})
	return schema.AssistantMessage("", []schema.ToolCall{
		{
			ID:   "call_mock_1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      m.tools[0].Name,
				Arguments: string(args),
			},
		},
	}), nil
}

func (m *mockToolCallingModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	// tool-call 轮：单块返回即可，让 ReAct 的 checker 命中工具调用。
	if len(msg.ToolCalls) > 0 {
		return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
	}

	// 回答轮：把内容切片模拟逐字流式输出。
	sr, sw := schema.Pipe[*schema.Message](16)
	go func() {
		defer sw.Close()
		runes := []rune(msg.Content)
		const step = 24
		for start := 0; start < len(runes); start += step {
			end := start + step
			if end > len(runes) {
				end = len(runes)
			}
			if sw.Send(&schema.Message{Role: schema.Assistant, Content: string(runes[start:end])}, nil) {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
	}()
	return sr, nil
}

func hasToolResult(input []*schema.Message) bool {
	for _, msg := range input {
		if msg.Role == schema.Tool {
			return true
		}
	}
	return false
}

func lastUserContent(input []*schema.Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role == schema.User {
			return strings.TrimSpace(input[i].Content)
		}
	}
	return ""
}

func lastToolContent(input []*schema.Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role == schema.Tool {
			return strings.TrimSpace(input[i].Content)
		}
	}
	return ""
}

func (m *mockToolCallingModel) buildAnswer(input []*schema.Message) string {
	question := lastUserContent(input)
	knowledge := lastToolContent(input)
	if knowledge == "" {
		knowledge = "知识库中没有检索到足够片段，当前只能给出有限回答。"
	}

	var b strings.Builder
	b.WriteString("## 一句话回答\n\n")
	b.WriteString(firstSentence(knowledge))
	b.WriteString("\n\n## 核心原理\n\n")
	b.WriteString("- 以下结论来自对个人知识库检索片段的归纳（mock 模式）。\n")
	b.WriteString("- 接入真实大模型后，这里会由模型综合多个片段自主组织语言。\n")
	b.WriteString("\n## 面试回答\n\n")
	b.WriteString("针对问题「")
	b.WriteString(question)
	b.WriteString("」，可以这样回答：\n\n")
	b.WriteString(knowledge)
	b.WriteString("\n\n## 常见追问\n\n")
	b.WriteString("1. 这个概念解决了什么问题？\n")
	b.WriteString("2. 核心组件分别承担什么职责？\n")
	b.WriteString("3. 在实际项目或源码中如何体现？\n")
	b.WriteString("\n## 引用来源\n\n")
	b.WriteString("- 见接口返回的 citations 字段。\n")
	return b.String()
}
