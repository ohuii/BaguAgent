package rag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"bagu-agent/backend/internal/tool"
	"bagu-agent/backend/internal/vectorstore"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

const reactAgentMaxStep = 8

// reactSystemPersona 约束模型：先检索个人知识库，再基于检索片段给出结构化面试回答，禁止编造。
const reactSystemPersona = `你是一名资深后端面试官和八股辅导老师。
你只能依据用户个人知识库（Markdown 笔记）中的内容来回答技术面试问题。

工作方式：
1. 收到问题后，先调用 search_knowledge 工具检索个人知识库；不要凭记忆直接作答。
2. 如果首次检索结果不相关或为空，可以换更合适的分类或关键词再检索一次。
3. 如果确实检索不到相关内容，要如实说明知识库里没有，并建议用户补充或重新索引笔记，绝不编造。

当检索到足够内容时，按以下 Markdown 结构输出最终回答：

## 一句话回答
## 核心原理
## 面试回答
## 常见追问
## 常见误区
## 引用来源

“引用来源”里列出你实际使用的片段标题路径。`

// reactInterviewAgent 懒加载并复用一次编译好的原生 ReAct Agent。
func (s *Service) reactInterviewAgent(ctx context.Context) (*react.Agent, error) {
	s.reactAgentOnce.Do(func() {
		if s.toolCallingModel == nil {
			s.reactAgentErr = errors.New("react agent requires a tool calling model")
			return
		}
		searchTool, err := tool.NewEinoSearchKnowledgeTool(s.retriever)
		if err != nil {
			s.reactAgentErr = fmt.Errorf("build search tool: %w", err)
			return
		}
		s.reactAgent, s.reactAgentErr = react.NewAgent(ctx, &react.AgentConfig{
			ToolCallingModel: s.toolCallingModel,
			ToolsConfig: compose.ToolsNodeConfig{
				Tools: []einotool.BaseTool{searchTool},
			},
			MessageModifier: react.NewPersonaModifier(reactSystemPersona),
			MaxStep:         reactAgentMaxStep,
			GraphName:       "bagu_interview_react_agent",
		})
	})
	return s.reactAgent, s.reactAgentErr
}

// reactResult 是一次 ReAct 运行的归一化输出，供 Chat / ChatStream 复用。
type reactResult struct {
	Answer       string
	Chunks       []vectorstore.SearchResult
	AnswerChunks []vectorstore.SearchResult
	Steps        []AgentStep
}

// runReactInterview 以阻塞方式运行原生 ReAct Agent，由模型自主决定何时检索。
func (s *Service) runReactInterview(ctx context.Context, input ChatInput, historySummary string) (reactResult, error) {
	agent, req, err := s.prepareReactRun(ctx, input)
	if err != nil {
		return reactResult{}, err
	}
	ctx = tool.WithReactRequest(ctx, req)

	start := time.Now()
	out, err := agent.Generate(ctx, buildReactMessages(input.Question, historySummary))
	if err != nil {
		return reactResult{}, fmt.Errorf("run react agent: %w", err)
	}
	return s.collectReactResult(req, out.Content, time.Since(start).Milliseconds()), nil
}

// streamReactInterview 以流式方式运行 ReAct Agent，最终回答 token 通过 onDelta 持续输出。
func (s *Service) streamReactInterview(ctx context.Context, input ChatInput, historySummary string, onDelta func(delta string) error) (reactResult, error) {
	agent, req, err := s.prepareReactRun(ctx, input)
	if err != nil {
		return reactResult{}, err
	}
	ctx = tool.WithReactRequest(ctx, req)

	start := time.Now()
	stream, err := agent.Stream(ctx, buildReactMessages(input.Question, historySummary))
	if err != nil {
		return reactResult{}, fmt.Errorf("stream react agent: %w", err)
	}
	defer stream.Close()

	var answer string
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return reactResult{}, fmt.Errorf("recv react stream: %w", recvErr)
		}
		if chunk.Content == "" {
			continue
		}
		answer += chunk.Content
		if onDelta != nil {
			if err := onDelta(chunk.Content); err != nil {
				return reactResult{}, err
			}
		}
	}
	return s.collectReactResult(req, answer, time.Since(start).Milliseconds()), nil
}

func (s *Service) prepareReactRun(ctx context.Context, input ChatInput) (*react.Agent, *tool.ReactRequest, error) {
	agent, err := s.reactInterviewAgent(ctx)
	if err != nil {
		return nil, nil, err
	}
	req := &tool.ReactRequest{UserID: input.UserID, TopK: input.TopK}
	return agent, req, nil
}

func buildReactMessages(question, historySummary string) []*schema.Message {
	msgs := make([]*schema.Message, 0, 2)
	if historySummary != "" {
		msgs = append(msgs, schema.SystemMessage("最近的对话摘要（供参考，必要时仍需检索知识库）：\n"+historySummary))
	}
	msgs = append(msgs, schema.UserMessage(question))
	return msgs
}

// collectReactResult 把工具调用收集到的检索结果整理成 citations 所需的形态，并回放 Agent 步骤。
func (s *Service) collectReactResult(req *tool.ReactRequest, answer string, answerLatencyMS int64) reactResult {
	chunks := req.Chunks()
	answerChunks := selectAnswerChunks(chunks)

	steps := make([]AgentStep, 0, len(req.Searches())+1)
	for _, obs := range req.Searches() {
		steps = append(steps, AgentStep{
			Step:    len(steps) + 1,
			Thought: "模型自主决定调用知识库检索工具。",
			Action:  tool.EinoSearchKnowledgeToolName,
			ActionInput: map[string]any{
				"query":    inputPreview(obs.Query, 120),
				"category": obs.Category,
			},
			Observation: map[string]any{
				"retrieved_count": obs.Count,
				"chunk_uids":      obs.UIDs,
			},
		})
	}
	steps = append(steps, AgentStep{
		Step:        len(steps) + 1,
		Thought:     "基于检索片段生成结构化面试回答。",
		Action:      "ReActAnswer",
		Observation: map[string]any{"answer_chars": len([]rune(answer))},
		LatencyMS:   answerLatencyMS,
	})

	return reactResult{
		Answer:       answer,
		Chunks:       chunks,
		AnswerChunks: answerChunks,
		Steps:        steps,
	}
}
