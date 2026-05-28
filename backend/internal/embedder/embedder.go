package embedder

import (
	"context"
	"fmt"

	"bagu-agent/backend/internal/config"
)

// Client 是 embedding 模型的统一接口。
// 后续无论接 OpenAI、百炼、火山还是本地模型，业务层都不需要感知差异。
type Client interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

// New 根据配置创建 embedding client。
// provider=mock 或 API Key 为空时使用本地确定性向量，方便开发环境跑通链路。
func New(cfg config.AIConfig, dim int) (Client, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("embedding dim must be positive")
	}
	if cfg.Provider == "mock" || cfg.APIKey == "" || cfg.EmbeddingModel == "" {
		return NewMock(dim), nil
	}
	return NewOpenAICompatible(cfg, dim), nil
}
