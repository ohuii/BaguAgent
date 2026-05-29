package llm

import (
	"context"
	"fmt"

	"bagu-agent/backend/internal/config"

	"github.com/cloudwego/eino/components/model"
)

// Client 是大模型聊天生成的统一接口。
type Client interface {
	Generate(ctx context.Context, prompt string) (string, error)
	Stream(ctx context.Context, prompt string, onDelta func(delta string) error) error
}

// New 根据配置创建 LLM client。
// provider=mock 或缺少 API Key 时使用本地 mock，方便不接模型也能跑通 RAG 链路。
func New(cfg config.AIConfig) (Client, error) {
	if cfg.Provider == "mock" || chatAPIKey(cfg) == "" || cfg.ChatModel == "" {
		return NewMock(), nil
	}
	if chatBaseURL(cfg) == "" {
		return nil, fmt.Errorf("ai.chat_base_url or ai.base_url is required")
	}
	return NewOpenAICompatible(cfg), nil
}

// NewToolCallingModel 根据配置创建支持 tool-calling 的 Eino ChatModel。
// provider=mock 或缺少 API Key 时返回脚本化 mock，保证原生 ReAct 链路在本地也能跑通。
func NewToolCallingModel(cfg config.AIConfig) (model.ToolCallingChatModel, error) {
	if cfg.Provider == "mock" || chatAPIKey(cfg) == "" || cfg.ChatModel == "" {
		return NewMockToolCallingModel(), nil
	}
	if chatBaseURL(cfg) == "" {
		return nil, fmt.Errorf("ai.chat_base_url or ai.base_url is required")
	}
	return NewOpenAIToolCallingModel(cfg), nil
}

func chatBaseURL(cfg config.AIConfig) string {
	if cfg.ChatBaseURL != "" {
		return cfg.ChatBaseURL
	}
	return cfg.BaseURL
}

func chatAPIKey(cfg config.AIConfig) string {
	if cfg.ChatAPIKey != "" {
		return cfg.ChatAPIKey
	}
	return cfg.APIKey
}
