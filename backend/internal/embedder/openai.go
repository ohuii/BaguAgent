package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bagu-agent/backend/internal/config"
)

// OpenAICompatibleClient 调用 OpenAI-compatible embeddings API。
type OpenAICompatibleClient struct {
	cfg        config.AIConfig
	dim        int
	httpClient *http.Client
}

// NewOpenAICompatible 创建 OpenAI-compatible embedding client。
func NewOpenAICompatible(cfg config.AIConfig, dim int) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		cfg: cfg,
		dim: dim,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// EmbedTexts 批量生成 embedding。
func (c *OpenAICompatibleClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	baseURL := strings.TrimRight(c.cfg.BaseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("ai.base_url is required")
	}

	body, err := json.Marshal(embeddingRequest{
		Model: c.cfg.EmbeddingModel,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call embedding api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding api status: %s", resp.Status)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	vectors := make([][]float32, len(result.Data))
	for _, item := range result.Data {
		if len(item.Embedding) != c.dim {
			return nil, fmt.Errorf("embedding dim mismatch: got %d want %d", len(item.Embedding), c.dim)
		}
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding response index out of range: %d", item.Index)
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []embeddingData `json:"data"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}
