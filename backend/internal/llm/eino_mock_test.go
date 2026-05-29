package llm

import (
	"context"
	"strings"
	"testing"

	"bagu-agent/backend/internal/config"

	"github.com/cloudwego/eino/schema"
)

func TestMockToolCallingModelScriptsReActLoop(t *testing.T) {
	ctx := context.Background()
	base := NewMockToolCallingModel()
	m, err := base.WithTools([]*schema.ToolInfo{{Name: "search_knowledge"}})
	if err != nil {
		t.Fatalf("WithTools error: %v", err)
	}

	// 第一轮：只有用户问题，期望模型先发起一次检索工具调用。
	first, err := m.Generate(ctx, []*schema.Message{schema.UserMessage("GMP 模型是什么？")})
	if err != nil {
		t.Fatalf("first Generate error: %v", err)
	}
	if len(first.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(first.ToolCalls))
	}
	if first.ToolCalls[0].Function.Name != "search_knowledge" {
		t.Fatalf("expected search_knowledge tool, got %q", first.ToolCalls[0].Function.Name)
	}
	if !strings.Contains(first.ToolCalls[0].Function.Arguments, "GMP") {
		t.Fatalf("expected tool args to carry the question, got %q", first.ToolCalls[0].Function.Arguments)
	}

	// 第二轮：工具结果已回填，期望模型产出最终回答而不再调用工具。
	second, err := m.Generate(ctx, []*schema.Message{
		schema.UserMessage("GMP 模型是什么？"),
		first,
		schema.ToolMessage("GMP 是 Go 运行时的调度模型，包含 G、M、P 三种角色。", first.ToolCalls[0].ID),
	})
	if err != nil {
		t.Fatalf("second Generate error: %v", err)
	}
	if len(second.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls in answer turn, got %d", len(second.ToolCalls))
	}
	if !strings.Contains(second.Content, "面试回答") {
		t.Fatalf("expected structured answer, got %q", second.Content)
	}
}

func TestNewToolCallingModelFallsBackToMock(t *testing.T) {
	m, err := NewToolCallingModel(config.AIConfig{Provider: "mock"})
	if err != nil {
		t.Fatalf("NewToolCallingModel error: %v", err)
	}
	if _, ok := m.(*mockToolCallingModel); !ok {
		t.Fatalf("expected mock tool calling model, got %T", m)
	}
}
