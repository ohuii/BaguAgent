package tool

import "context"

// Tool 是 Agent 可调用工具的统一抽象。
// 第一版用强类型 input/output 包装在 any 中，后续接 Eino/ReAct 时可映射成 schema。
type Tool interface {
	Name() string
	Description() string
	Call(ctx context.Context, input any) (any, error)
}
