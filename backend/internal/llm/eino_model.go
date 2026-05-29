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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// openAIToolCallingModel 在 OpenAI-compatible /chat/completions 接口上实现 Eino 的
// model.ToolCallingChatModel，支持把工具 schema 传给模型并解析模型返回的 tool_calls，
// 是接入原生 ReAct Agent（模型自主决定调用哪个工具）的关键。
type openAIToolCallingModel struct {
	cfg        config.AIConfig
	httpClient *http.Client
	tools      []*schema.ToolInfo
}

// NewOpenAIToolCallingModel 创建 OpenAI-compatible 的 ToolCallingChatModel。
func NewOpenAIToolCallingModel(cfg config.AIConfig) *openAIToolCallingModel {
	return &openAIToolCallingModel{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// WithTools 返回一个绑定了工具的新实例，不修改原实例，可安全并发复用基础 model。
func (m *openAIToolCallingModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	clone := *m
	clone.tools = tools
	return &clone, nil
}

// Generate 阻塞式调用，返回包含 content 或 tool_calls 的 assistant 消息。
func (m *openAIToolCallingModel) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	reqBody, err := m.buildRequest(input, false)
	if err != nil {
		return nil, err
	}
	resp, err := m.do(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result toolChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("chat api returned empty choices")
	}
	choice := result.Choices[0].Message
	return schema.AssistantMessage(choice.Content, toSchemaToolCalls(choice.ToolCalls)), nil
}

// Stream 流式调用，按 SSE 增量推送 content 和 tool_call 分片，由 Eino 负责拼接。
func (m *openAIToolCallingModel) Stream(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reqBody, err := m.buildRequest(input, true)
	if err != nil {
		return nil, err
	}
	resp, err := m.do(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	sr, sw := schema.Pipe[*schema.Message](16)
	go func() {
		defer resp.Body.Close()
		defer sw.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				return
			}
			var event toolChatStreamResponse
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				sw.Send(nil, fmt.Errorf("decode chat stream event: %w", err))
				return
			}
			for _, choice := range event.Choices {
				msg := streamDeltaToMessage(choice.Delta)
				if msg == nil {
					continue
				}
				if sw.Send(msg, nil) {
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			sw.Send(nil, fmt.Errorf("read chat stream: %w", err))
		}
	}()
	return sr, nil
}

func (m *openAIToolCallingModel) buildRequest(input []*schema.Message, stream bool) (toolChatRequest, error) {
	messages := make([]toolChatMessage, 0, len(input))
	for _, msg := range input {
		messages = append(messages, toRequestMessage(msg))
	}
	req := toolChatRequest{
		Model:       m.cfg.ChatModel,
		Messages:    messages,
		Temperature: 0.2,
		Stream:      stream,
	}
	if len(m.tools) > 0 {
		tools, err := toRequestTools(m.tools)
		if err != nil {
			return toolChatRequest{}, err
		}
		req.Tools = tools
		req.ToolChoice = "auto"
	}
	return req, nil
}

func (m *openAIToolCallingModel) do(ctx context.Context, reqBody toolChatRequest) (*http.Response, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}
	baseURL := strings.TrimRight(chatBaseURL(m.cfg), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+chatAPIKey(m.cfg))
	if reqBody.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call chat api: %w", err)
	}
	return resp, nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("chat api status: %s, body: %s", resp.Status, strings.TrimSpace(string(respBody)))
}

func toRequestMessage(msg *schema.Message) toolChatMessage {
	out := toolChatMessage{
		Role:       string(msg.Role),
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}
	for _, tc := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, toolCallPayload{
			ID:   tc.ID,
			Type: "function",
			Function: functionCallPayload{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return out
}

func toRequestTools(tools []*schema.ToolInfo) ([]toolDefinition, error) {
	defs := make([]toolDefinition, 0, len(tools))
	for _, ti := range tools {
		js, err := ti.ToJSONSchema()
		if err != nil {
			return nil, fmt.Errorf("tool %s schema: %w", ti.Name, err)
		}
		defs = append(defs, toolDefinition{
			Type: "function",
			Function: functionDefinition{
				Name:        ti.Name,
				Description: ti.Desc,
				Parameters:  js,
			},
		})
	}
	return defs, nil
}

func toSchemaToolCalls(calls []toolCallPayload) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]schema.ToolCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, schema.ToolCall{
			ID:   c.ID,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		})
	}
	return out
}

func streamDeltaToMessage(delta toolChatStreamDelta) *schema.Message {
	var toolCalls []schema.ToolCall
	for _, c := range delta.ToolCalls {
		idx := c.Index
		toolCalls = append(toolCalls, schema.ToolCall{
			Index: &idx,
			ID:    c.ID,
			Type:  "function",
			Function: schema.FunctionCall{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		})
	}
	if delta.Content == "" && len(toolCalls) == 0 {
		return nil
	}
	return &schema.Message{
		Role:      schema.Assistant,
		Content:   delta.Content,
		ToolCalls: toolCalls,
	}
}

type toolChatRequest struct {
	Model       string            `json:"model"`
	Messages    []toolChatMessage `json:"messages"`
	Temperature float64           `json:"temperature"`
	Stream      bool              `json:"stream,omitempty"`
	Tools       []toolDefinition  `json:"tools,omitempty"`
	ToolChoice  string            `json:"tool_choice,omitempty"`
}

type toolChatMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []toolCallPayload `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type toolDefinition struct {
	Type     string             `json:"type"`
	Function functionDefinition `json:"function"`
}

type functionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type toolCallPayload struct {
	Index    int                 `json:"index,omitempty"`
	ID       string              `json:"id,omitempty"`
	Type     string              `json:"type,omitempty"`
	Function functionCallPayload `json:"function"`
}

type functionCallPayload struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type toolChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string            `json:"content"`
			ToolCalls []toolCallPayload `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

type toolChatStreamResponse struct {
	Choices []struct {
		Delta toolChatStreamDelta `json:"delta"`
	} `json:"choices"`
}

type toolChatStreamDelta struct {
	Content   string `json:"content"`
	ToolCalls []struct {
		Index    int                 `json:"index"`
		ID       string              `json:"id"`
		Function functionCallPayload `json:"function"`
	} `json:"tool_calls"`
}
