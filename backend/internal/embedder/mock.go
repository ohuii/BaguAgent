package embedder

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
)

// MockClient 生成确定性伪向量，只用于本地开发和接口联调。
// 它不能代表真实语义相似度，正式 RAG 需要替换成真实 embedding 模型。
type MockClient struct {
	dim int
}

// NewMock 创建本地 mock embedding client。
func NewMock(dim int) *MockClient {
	return &MockClient{dim: dim}
}

// EmbedTexts 将文本 hash 到固定维度向量，并做 L2 归一化。
func (c *MockClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		vector := make([]float32, c.dim)
		for _, token := range tokenize(text) {
			idx := int(hashString(token) % uint64(c.dim))
			vector[idx] += 1
		}
		normalize(vector)
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func tokenize(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) > 0 {
		return fields
	}

	tokens := make([]string, 0, len([]rune(text)))
	for _, r := range text {
		tokens = append(tokens, string(r))
	}
	return tokens
}

func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func normalize(vector []float32) {
	var sum float64
	for _, v := range vector {
		sum += float64(v * v)
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(sum))
	for i := range vector {
		vector[i] /= norm
	}
}
