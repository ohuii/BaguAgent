package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	agentmodel "bagu-agent/backend/internal/agent"
	"bagu-agent/backend/internal/conversation"
	"bagu-agent/backend/internal/llm"
	"bagu-agent/backend/internal/message"
	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/tool"
	"bagu-agent/backend/internal/vectorstore"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/flow/agent/react"
)

// Service 编排 RAG 问答主流程。
type Service struct {
	convRepo     *conversation.Repository
	msgRepo      *message.Repository
	runRepo      *agentmodel.Repository
	retriever    *retriever.Service
	llm          llm.Client
	searchTool   *tool.SearchKnowledgeTool
	answerTool   *tool.InterviewAnswerTool
	questionTool *tool.QuestionGenerateTool
	historySize  int

	toolCallingModel model.ToolCallingChatModel
	agentMode        string

	einoGraphOnce     sync.Once
	einoGraphRunnable einoInterviewRunnable
	einoGraphErr      error

	reactAgentOnce sync.Once
	reactAgent     *react.Agent
	reactAgentErr  error
}

// NewService 创建 RAG service。
// toolCallingModel 用于原生 ReAct Agent（agent_mode=react），缺省的确定性 Eino Graph 不依赖它。
func NewService(convRepo *conversation.Repository, msgRepo *message.Repository, runRepo *agentmodel.Repository, retriever *retriever.Service, llm llm.Client, toolCallingModel model.ToolCallingChatModel, agentMode string) *Service {
	if agentMode == "" {
		agentMode = "graph"
	}
	return &Service{
		convRepo:         convRepo,
		msgRepo:          msgRepo,
		runRepo:          runRepo,
		retriever:        retriever,
		llm:              llm,
		searchTool:       tool.NewSearchKnowledgeTool(retriever),
		answerTool:       tool.NewInterviewAnswerTool(llm),
		questionTool:     tool.NewQuestionGenerateTool(llm),
		historySize:      6,
		toolCallingModel: toolCallingModel,
		agentMode:        agentMode,
	}
}

// ChatInput 是 Agent Chat 请求。
type ChatInput struct {
	UserID         uint64 `json:"user_id"`
	ConversationID uint64 `json:"conversation_id"`
	Question       string `json:"question"`
	Category       string `json:"category"`
	TopK           int    `json:"top_k"`
}

// Citation 是答案引用来源。
type Citation struct {
	ChunkUID   string  `json:"chunk_uid"`
	DocumentID int64   `json:"document_id"`
	ChunkID    int64   `json:"chunk_id"`
	TitlePath  string  `json:"title_path"`
	Category   string  `json:"category"`
	Score      float32 `json:"score"`
}

// ChatResult 是 Agent Chat 响应。
// AgentStep 记录一次轻量 ReAct 执行步骤，便于调试和后续迁移到 Eino Graph。
type AgentStep struct {
	Step        int    `json:"step"`
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	ActionInput any    `json:"action_input,omitempty"`
	Observation any    `json:"observation,omitempty"`
	LatencyMS   int64  `json:"latency_ms"`
}

type ChatResult struct {
	ConversationID  uint64                     `json:"conversation_id"`
	Answer          string                     `json:"answer"`
	Citations       []Citation                 `json:"citations"`
	RetrievedChunks []vectorstore.SearchResult `json:"retrieved_chunks"`
	AgentSteps      []AgentStep                `json:"agent_steps"`
}

// QuestionGenerateInput 是生成面试题请求。
type QuestionGenerateInput struct {
	UserID   uint64 `json:"user_id"`
	Topic    string `json:"topic"`
	Category string `json:"category"`
	Count    int    `json:"count"`
	TopK     int    `json:"top_k"`
}

// QuestionGenerateResult 是生成面试题响应。
type QuestionGenerateResult struct {
	Content         string                     `json:"content"`
	RetrievedChunks []vectorstore.SearchResult `json:"retrieved_chunks"`
}

// StreamEvent 是 /agent/chat/stream 返回的 SSE 事件数据。
type StreamEvent struct {
	Type            string                     `json:"type"`
	ConversationID  uint64                     `json:"conversation_id,omitempty"`
	Delta           string                     `json:"delta,omitempty"`
	Answer          string                     `json:"answer,omitempty"`
	Citations       []Citation                 `json:"citations,omitempty"`
	RetrievedChunks []vectorstore.SearchResult `json:"retrieved_chunks,omitempty"`
	AgentSteps      []AgentStep                `json:"agent_steps,omitempty"`
	Error           string                     `json:"error,omitempty"`
}

// Chat 执行一次 RAG 问答，并保存会话、消息和运行记录。
func (s *Service) Chat(ctx context.Context, input ChatInput) (*ChatResult, error) {
	start := time.Now()
	if input.UserID == 0 {
		input.UserID = 1
	}
	input.Question = strings.TrimSpace(input.Question)
	if input.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if input.TopK <= 0 {
		input.TopK = 5
	}

	conv, err := s.ensureConversation(ctx, input)
	if err != nil {
		return nil, err
	}

	userMsg := &message.Message{
		ConversationID: conv.ID,
		Role:           message.RoleUser,
		Content:        input.Question,
	}
	if err := s.msgRepo.Create(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}

	historySummary, err := s.historySummary(ctx, conv.ID)
	if err != nil {
		return nil, err
	}

	var chunks, answerChunks []vectorstore.SearchResult
	var answer string
	var steps []AgentStep

	if s.agentMode == "react" {
		// 原生 ReAct：由模型自主决定何时调用检索工具、如何组织回答。
		res, err := s.runReactInterview(ctx, input, historySummary)
		if err != nil {
			return nil, err
		}
		chunks, answerChunks, answer, steps = res.Chunks, res.AnswerChunks, res.Answer, res.Steps
	} else {
		// 默认确定性 Eino Graph v3：计划 -> 检索 -> 评估 -> 分支（回答 / 重试 / 兜底）。
		graphState, err := s.runEinoInterviewGraph(ctx, einoInterviewGraphState{
			UserID:         input.UserID,
			Question:       input.Question,
			Category:       input.Category,
			TopK:           input.TopK,
			HistorySummary: historySummary,
			Steps:          make([]AgentStep, 0, 2),
		})
		if err != nil {
			return nil, err
		}
		chunks, answerChunks, answer, steps = graphState.Chunks, graphState.AnswerChunks, graphState.Answer, graphState.Steps
	}

	citations := buildCitations(answerChunks)
	citationsJSON, err := marshalJSONString(citations)
	if err != nil {
		return nil, err
	}
	assistantMsg := &message.Message{
		ConversationID: conv.ID,
		Role:           message.RoleAssistant,
		Content:        answer,
		CitationsJSON:  citationsJSON,
	}
	if err := s.msgRepo.Create(ctx, assistantMsg); err != nil {
		return nil, fmt.Errorf("save assistant message: %w", err)
	}

	toolsUsedJSON, _ := marshalJSONString(stepActions(steps))
	stepsJSON, _ := marshalJSONString(steps)
	retrievedJSON, _ := marshalJSONString(chunks)
	run := &agentmodel.AgentRun{
		ConversationID:      conv.ID,
		MessageID:           assistantMsg.ID,
		UserQuery:           input.Question,
		Intent:              "interview_qa",
		ToolsUsed:           toolsUsedJSON,
		AgentStepsJSON:      stepsJSON,
		RetrievedChunksJSON: retrievedJSON,
		FinalAnswer:         answer,
		LatencyMS:           time.Since(start).Milliseconds(),
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("save agent run: %w", err)
	}

	return &ChatResult{
		ConversationID:  conv.ID,
		Answer:          answer,
		Citations:       citations,
		RetrievedChunks: chunks,
		AgentSteps:      steps,
	}, nil
}

// GenerateQuestions 根据知识点生成面试题。
func (s *Service) GenerateQuestions(ctx context.Context, input QuestionGenerateInput) (*QuestionGenerateResult, error) {
	if input.UserID == 0 {
		input.UserID = 1
	}
	input.Topic = strings.TrimSpace(input.Topic)
	if input.Topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if input.TopK <= 0 {
		input.TopK = 5
	}

	searchOutputAny, err := s.searchTool.Call(ctx, tool.SearchKnowledgeInput{
		UserID:   input.UserID,
		Query:    input.Topic,
		Category: input.Category,
		TopK:     input.TopK,
	})
	if err != nil {
		return nil, err
	}
	searchOutput := searchOutputAny.(tool.SearchKnowledgeOutput)
	answerChunks := selectAnswerChunks(searchOutput.Chunks)

	questionOutputAny, err := s.questionTool.Call(ctx, tool.QuestionGenerateInput{
		Topic:    input.Topic,
		Category: input.Category,
		Count:    input.Count,
		Chunks:   answerChunks,
	})
	if err != nil {
		return nil, err
	}
	questionOutput := questionOutputAny.(tool.QuestionGenerateOutput)
	return &QuestionGenerateResult{
		Content:         questionOutput.Content,
		RetrievedChunks: searchOutput.Chunks,
	}, nil
}

// ChatStream 执行一次 RAG 问答，并通过回调持续输出模型增量文本。
func (s *Service) ChatStream(ctx context.Context, input ChatInput, emit func(StreamEvent) error) error {
	start := time.Now()
	if input.UserID == 0 {
		input.UserID = 1
	}
	input.Question = strings.TrimSpace(input.Question)
	if input.Question == "" {
		return fmt.Errorf("question is required")
	}
	if input.TopK <= 0 {
		input.TopK = 5
	}

	conv, err := s.ensureConversation(ctx, input)
	if err != nil {
		return err
	}
	if err := emit(StreamEvent{Type: "meta", ConversationID: conv.ID}); err != nil {
		return err
	}

	userMsg := &message.Message{
		ConversationID: conv.ID,
		Role:           message.RoleUser,
		Content:        input.Question,
	}
	if err := s.msgRepo.Create(ctx, userMsg); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}

	historySummary, err := s.historySummary(ctx, conv.ID)
	if err != nil {
		return err
	}

	var chunks, answerChunks []vectorstore.SearchResult
	var answer string
	var citations []Citation
	var steps []AgentStep

	if s.agentMode == "react" {
		// 原生 ReAct 的检索发生在 Agent 内部，因此先流式输出回答，再补发 retrieved（引用）事件。
		res, err := s.streamReactInterview(ctx, input, historySummary, func(delta string) error {
			return emit(StreamEvent{Type: "delta", ConversationID: conv.ID, Delta: delta})
		})
		if err != nil {
			return err
		}
		chunks, answerChunks, answer, steps = res.Chunks, res.AnswerChunks, res.Answer, res.Steps
		citations = buildCitations(answerChunks)
		if err := emit(StreamEvent{
			Type:            "retrieved",
			ConversationID:  conv.ID,
			Citations:       citations,
			RetrievedChunks: chunks,
			AgentSteps:      steps,
		}); err != nil {
			return err
		}
	} else {
		// 默认确定性 Eino Graph v3：先检索评估，再流式生成回答。
		graphState, err := s.runEinoInterviewGraph(ctx, einoInterviewGraphState{
			UserID:         input.UserID,
			Question:       input.Question,
			Category:       input.Category,
			TopK:           input.TopK,
			HistorySummary: historySummary,
			StreamAnswer:   true,
			Steps:          make([]AgentStep, 0, 4),
		})
		if err != nil {
			return err
		}
		chunks = graphState.Chunks
		answerChunks = graphState.AnswerChunks
		citations = buildCitations(answerChunks)
		steps = graphState.Steps
		if err := emit(StreamEvent{
			Type:            "retrieved",
			ConversationID:  conv.ID,
			Citations:       citations,
			RetrievedChunks: chunks,
			AgentSteps:      steps,
		}); err != nil {
			return err
		}

		answer = graphState.Answer
		if answer != "" {
			if err := emit(StreamEvent{
				Type:           "delta",
				ConversationID: conv.ID,
				Delta:          answer,
			}); err != nil {
				return err
			}
		} else {
			answerInput := tool.InterviewAnswerInput{
				Question:       input.Question,
				Chunks:         answerChunks,
				HistorySummary: historySummary,
			}
			stepStart := time.Now()
			var answerBuilder strings.Builder
			if err := s.answerTool.Stream(ctx, answerInput, func(delta string) error {
				answerBuilder.WriteString(delta)
				return emit(StreamEvent{
					Type:           "delta",
					ConversationID: conv.ID,
					Delta:          delta,
				})
			}); err != nil {
				return err
			}
			answer = answerBuilder.String()
			steps = finishStreamAnswerStep(steps, s.answerTool.Name(), answer, time.Since(stepStart).Milliseconds())
		}
	}
	citationsJSON, err := marshalJSONString(citations)
	if err != nil {
		return err
	}
	assistantMsg := &message.Message{
		ConversationID: conv.ID,
		Role:           message.RoleAssistant,
		Content:        answer,
		CitationsJSON:  citationsJSON,
	}
	if err := s.msgRepo.Create(ctx, assistantMsg); err != nil {
		return fmt.Errorf("save assistant message: %w", err)
	}

	toolsUsedJSON, _ := marshalJSONString(stepActions(steps))
	stepsJSON, _ := marshalJSONString(steps)
	retrievedJSON, _ := marshalJSONString(chunks)
	run := &agentmodel.AgentRun{
		ConversationID:      conv.ID,
		MessageID:           assistantMsg.ID,
		UserQuery:           input.Question,
		Intent:              "interview_qa_stream",
		ToolsUsed:           toolsUsedJSON,
		AgentStepsJSON:      stepsJSON,
		RetrievedChunksJSON: retrievedJSON,
		FinalAnswer:         answer,
		LatencyMS:           time.Since(start).Milliseconds(),
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		return fmt.Errorf("save agent run: %w", err)
	}

	return emit(StreamEvent{
		Type:           "done",
		ConversationID: conv.ID,
		Answer:         answer,
		Citations:      citations,
		AgentSteps:     steps,
	})
}

func selectAnswerChunks(chunks []vectorstore.SearchResult) []vectorstore.SearchResult {
	selected := make([]vectorstore.SearchResult, 0, len(chunks))
	for _, chunk := range chunks {
		if len([]rune(strings.TrimSpace(chunk.Content))) < 80 {
			continue
		}
		selected = append(selected, chunk)
	}
	if len(selected) == 0 {
		return chunks
	}
	return selected
}

func buildSearchObservation(chunks []vectorstore.SearchResult) map[string]any {
	return map[string]any{
		"retrieved_count": len(chunks),
		"chunk_uids":      searchResultUIDs(chunks),
	}
}

func summarizeAnswerInput(input tool.InterviewAnswerInput) map[string]any {
	return map[string]any{
		"question":        input.Question,
		"chunk_count":     len(input.Chunks),
		"chunk_uids":      searchResultUIDs(input.Chunks),
		"has_history":     strings.TrimSpace(input.HistorySummary) != "",
		"history_chars":   len([]rune(input.HistorySummary)),
		"knowledge_chars": searchResultContentChars(input.Chunks),
	}
}

func searchResultUIDs(chunks []vectorstore.SearchResult) []string {
	uids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.ChunkUID == "" {
			continue
		}
		uids = append(uids, chunk.ChunkUID)
	}
	return uids
}

func searchResultContentChars(chunks []vectorstore.SearchResult) int {
	var total int
	for _, chunk := range chunks {
		total += len([]rune(chunk.Content))
	}
	return total
}

func stepActions(steps []AgentStep) []string {
	seen := make(map[string]bool)
	actions := make([]string, 0, len(steps))
	for _, step := range steps {
		action := strings.TrimSpace(step.Action)
		if action == "" || seen[action] {
			continue
		}
		seen[action] = true
		actions = append(actions, action)
	}
	return actions
}

func finishStreamAnswerStep(steps []AgentStep, action string, answer string, latencyMS int64) []AgentStep {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Action != action {
			continue
		}
		steps[i].Observation = map[string]any{
			"stream":       true,
			"answer_chars": len([]rune(answer)),
		}
		steps[i].LatencyMS = latencyMS
		return steps
	}
	return append(steps, AgentStep{
		Step:        len(steps) + 1,
		Thought:     "基于检索片段完成流式面试回答。",
		Action:      action,
		Observation: map[string]any{"stream": true, "answer_chars": len([]rune(answer))},
		LatencyMS:   latencyMS,
	})
}

func (s *Service) ensureConversation(ctx context.Context, input ChatInput) (*conversation.Conversation, error) {
	if input.ConversationID > 0 {
		return s.convRepo.GetByID(ctx, input.ConversationID)
	}

	title := input.Question
	if len([]rune(title)) > 30 {
		title = string([]rune(title)[:30]) + "..."
	}
	conv := &conversation.Conversation{
		UserID: input.UserID,
		Title:  title,
	}
	if err := s.convRepo.Create(ctx, conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

func (s *Service) historySummary(ctx context.Context, conversationID uint64) (string, error) {
	messages, err := s.msgRepo.ListByConversationID(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("list history messages: %w", err)
	}
	if len(messages) == 0 {
		return "", nil
	}
	if len(messages) > s.historySize {
		messages = messages[len(messages)-s.historySize:]
	}

	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(trimRunes(msg.Content, 200))
		b.WriteString("\n")
	}
	return b.String(), nil
}

func buildCitations(chunks []vectorstore.SearchResult) []Citation {
	citations := make([]Citation, 0, len(chunks))
	seen := make(map[string]bool)
	for _, chunk := range chunks {
		if seen[chunk.ChunkUID] {
			continue
		}
		seen[chunk.ChunkUID] = true
		citations = append(citations, Citation{
			ChunkUID:   chunk.ChunkUID,
			DocumentID: chunk.DocumentID,
			ChunkID:    chunk.ChunkID,
			TitlePath:  chunk.TitlePath,
			Category:   chunk.Category,
			Score:      chunk.Score,
		})
	}
	return citations
}

func marshalJSONString(v any) (*string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}

func trimRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
