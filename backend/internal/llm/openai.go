package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bagu-agent/backend/internal/config"
)

// OpenAICompatibleClient 调用 OpenAI-compatible chat completions API。
type OpenAICompatibleClient struct {
	cfg        config.AIConfig
	httpClient *http.Client
}

// NewOpenAICompatible 创建 OpenAI-compatible LLM client。
func NewOpenAICompatible(cfg config.AIConfig) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Generate 生成回答。
func (c *OpenAICompatibleClient) Generate(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.cfg.ChatModel,
		Messages: []chatMessage{
			{Role: "system", Content: "你是一个资深后端面试官和候选人辅导老师。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	baseURL := strings.TrimRight(chatBaseURL(c.cfg), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+chatAPIKey(c.cfg))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call chat api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("chat api status: %s, body: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("chat api returned empty choices")
	}
	return result.Choices[0].Message.Content, nil
}

// Stream 调用 OpenAI-compatible 流式输出接口。
func (c *OpenAICompatibleClient) Stream(ctx context.Context, prompt string, onDelta func(delta string) error) error {
	body, err := json.Marshal(chatRequest{
		Model: c.cfg.ChatModel,
		Messages: []chatMessage{
			{Role: "system", Content: "你是一个资深后端面试官和候选人辅导老师。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
		Stream:      true,
	})
	if err != nil {
		return fmt.Errorf("marshal chat stream request: %w", err)
	}

	baseURL := strings.TrimRight(chatBaseURL(c.cfg), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new chat stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+chatAPIKey(c.cfg))
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call chat stream api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("chat stream api status: %s, body: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return nil
		}

		var event chatStreamResponse
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return fmt.Errorf("decode chat stream event: %w", err)
		}
		for _, choice := range event.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := onDelta(choice.Delta.Content); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read chat stream: %w", err)
	}
	return nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatStreamResponse struct {
	Choices []chatStreamChoice `json:"choices"`
}

type chatStreamChoice struct {
	Delta        chatMessage `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}
