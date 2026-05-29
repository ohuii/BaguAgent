package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bagu-agent/backend/internal/tool"
	"bagu-agent/backend/internal/vectorstore"

	"github.com/cloudwego/eino/compose"
)

const (
	einoNodePlanReAct        = "plan_react"
	einoNodeSearchKnowledge  = "search_knowledge"
	einoNodeAssessRetrieval  = "assess_retrieval"
	einoNodeRetrySearch      = "retry_search"
	einoNodeInterviewAnswer  = "interview_answer"
	einoNodeNoContext        = "no_context_answer"
	minUsefulRetrievalScore  = 0.45
	defaultNoContextCategory = "当前分类"
	einoGraphMaxRunSteps     = 30
)

type einoInterviewRunnable = compose.Runnable[einoInterviewGraphState, einoInterviewGraphState]

// einoInterviewGraphState 是 Eino Graph 在节点之间流转的状态。
// v2 引入 ReAct 雏形：Plan -> Action(Search) -> Observation(Assess) -> Answer。
type einoInterviewGraphState struct {
	UserID         uint64
	Question       string
	Category       string
	TopK           int
	HistorySummary string

	NeedSearch          bool
	PlannedCategory     string
	CategoryWasInferred bool
	CategoryMismatch    bool
	RetrievalSufficient bool
	Retried             bool
	AnswerMode          string
	StreamAnswer        bool

	Chunks       []vectorstore.SearchResult
	AnswerChunks []vectorstore.SearchResult
	Answer       string
	Steps        []AgentStep
}

// runEinoInterviewGraph 使用 Eino Graph 编排一次 RAG 问答。
// 这一版先做轻量 ReAct 骨架，后续可以继续扩展成循环、多工具选择和条件分支。
func (s *Service) runEinoInterviewGraph(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	runnable, err := s.einoInterviewGraph(ctx)
	if err != nil {
		return state, err
	}
	out, err := runnable.Invoke(ctx, state)
	if err != nil {
		return state, fmt.Errorf("invoke eino graph: %w", err)
	}
	return out, nil
}

func (s *Service) einoInterviewGraph(ctx context.Context) (einoInterviewRunnable, error) {
	s.einoGraphOnce.Do(func() {
		s.einoGraphRunnable, s.einoGraphErr = s.compileEinoInterviewGraph(ctx)
	})
	return s.einoGraphRunnable, s.einoGraphErr
}

// compileEinoInterviewGraph 只在首次请求时编译一次 Graph，后续请求复用 Runnable。
func (s *Service) compileEinoInterviewGraph(ctx context.Context) (einoInterviewRunnable, error) {
	graph := compose.NewGraph[einoInterviewGraphState, einoInterviewGraphState]()

	if err := graph.AddLambdaNode(einoNodePlanReAct, compose.InvokableLambda(s.einoPlanReActNode)); err != nil {
		return nil, fmt.Errorf("add plan react node: %w", err)
	}
	if err := graph.AddLambdaNode(einoNodeSearchKnowledge, compose.InvokableLambda(s.einoSearchKnowledgeNode)); err != nil {
		return nil, fmt.Errorf("add search knowledge node: %w", err)
	}
	if err := graph.AddLambdaNode(einoNodeAssessRetrieval, compose.InvokableLambda(s.einoAssessRetrievalNode)); err != nil {
		return nil, fmt.Errorf("add assess retrieval node: %w", err)
	}
	if err := graph.AddLambdaNode(einoNodeRetrySearch, compose.InvokableLambda(s.einoRetrySearchNode)); err != nil {
		return nil, fmt.Errorf("add retry search node: %w", err)
	}
	if err := graph.AddLambdaNode(einoNodeInterviewAnswer, compose.InvokableLambda(s.einoInterviewAnswerNode)); err != nil {
		return nil, fmt.Errorf("add interview answer node: %w", err)
	}
	if err := graph.AddLambdaNode(einoNodeNoContext, compose.InvokableLambda(s.einoNoContextAnswerNode)); err != nil {
		return nil, fmt.Errorf("add no context node: %w", err)
	}

	if err := graph.AddEdge(compose.START, einoNodePlanReAct); err != nil {
		return nil, fmt.Errorf("add start edge: %w", err)
	}
	if err := graph.AddEdge(einoNodePlanReAct, einoNodeSearchKnowledge); err != nil {
		return nil, fmt.Errorf("add search edge: %w", err)
	}
	if err := graph.AddEdge(einoNodeSearchKnowledge, einoNodeAssessRetrieval); err != nil {
		return nil, fmt.Errorf("add assess edge: %w", err)
	}

	// 观察检索结果后分支：足够则回答，分类不匹配且未重试则换分类重试，否则保守兜底。
	assessBranch := compose.NewGraphBranch[einoInterviewGraphState](s.einoAssessBranch, map[string]bool{
		einoNodeInterviewAnswer: true,
		einoNodeRetrySearch:     true,
		einoNodeNoContext:       true,
	})
	if err := graph.AddBranch(einoNodeAssessRetrieval, assessBranch); err != nil {
		return nil, fmt.Errorf("add assess branch: %w", err)
	}

	// 重试检索后回到 assess 重新评估，形成一次有界回环。
	if err := graph.AddEdge(einoNodeRetrySearch, einoNodeAssessRetrieval); err != nil {
		return nil, fmt.Errorf("add retry edge: %w", err)
	}
	if err := graph.AddEdge(einoNodeInterviewAnswer, compose.END); err != nil {
		return nil, fmt.Errorf("add answer end edge: %w", err)
	}
	if err := graph.AddEdge(einoNodeNoContext, compose.END); err != nil {
		return nil, fmt.Errorf("add no context end edge: %w", err)
	}

	runnable, err := graph.Compile(ctx,
		compose.WithGraphName("bagu_interview_react_v3"),
		compose.WithMaxRunSteps(einoGraphMaxRunSteps),
	)
	if err != nil {
		return nil, fmt.Errorf("compile eino graph: %w", err)
	}
	return runnable, nil
}

// einoAssessBranch 根据检索评估结果决定下一步走向。
func (s *Service) einoAssessBranch(_ context.Context, state einoInterviewGraphState) (string, error) {
	if state.RetrievalSufficient {
		return einoNodeInterviewAnswer, nil
	}
	if state.CategoryMismatch && !state.Retried {
		return einoNodeRetrySearch, nil
	}
	return einoNodeNoContext, nil
}

// einoRetrySearchNode 在首轮检索不足且问题与所选分类不一致时，换用推断分类重试一次检索。
func (s *Service) einoRetrySearchNode(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	state.Retried = true
	previousCategory := state.PlannedCategory
	inferredCategory := inferCategoryFromQuestion(state.Question)
	state.PlannedCategory = inferredCategory
	// 已经改用推断分类重试，分类不匹配不再是兜底时要强调的原因。
	state.CategoryMismatch = false

	searchInput := tool.SearchKnowledgeInput{
		UserID:   state.UserID,
		Query:    state.Question,
		Category: inferredCategory,
		TopK:     state.TopK,
	}
	stepStart := time.Now()
	searchOutputAny, err := s.searchTool.Call(ctx, searchInput)
	if err != nil {
		return state, err
	}
	searchOutput := searchOutputAny.(tool.SearchKnowledgeOutput)

	state.Chunks = searchOutput.Chunks
	state.AnswerChunks = selectAnswerChunks(searchOutput.Chunks)
	state.Steps = append(state.Steps, AgentStep{
		Step:    len(state.Steps) + 1,
		Thought: "首轮检索不足且问题与所选分类不一致，换用推断分类重试一次检索。",
		Action:  "RetrySearch",
		ActionInput: map[string]any{
			"previous_category": previousCategory,
			"retry_category":    inferredCategory,
			"query":             inputPreview(state.Question, 120),
			"top_k":             state.TopK,
		},
		Observation: buildSearchObservation(searchOutput.Chunks),
		LatencyMS:   time.Since(stepStart).Milliseconds(),
	})
	return state, nil
}

// einoPlanReActNode 负责做一次轻量计划：是否需要检索、用哪个分类检索。
func (s *Service) einoPlanReActNode(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	_ = ctx
	stepStart := time.Now()

	requestCategory := strings.TrimSpace(state.Category)
	inferredCategory := inferCategoryFromQuestion(state.Question)
	state.NeedSearch = true
	state.PlannedCategory = requestCategory

	if shouldAutoSelectCategory(requestCategory) {
		state.PlannedCategory = inferredCategory
		state.CategoryWasInferred = inferredCategory != ""
	}
	state.CategoryMismatch = requestCategory != "" &&
		!shouldAutoSelectCategory(requestCategory) &&
		inferredCategory != "" &&
		!strings.EqualFold(requestCategory, inferredCategory)

	state.Steps = append(state.Steps, AgentStep{
		Step:    len(state.Steps) + 1,
		Thought: "先判断用户问题是否需要查询个人知识库，并确定本轮检索应该使用的分类。",
		Action:  "ReActPlan",
		ActionInput: map[string]any{
			"question": inputPreview(state.Question, 120),
			"category": requestCategory,
			"top_k":    state.TopK,
		},
		Observation: map[string]any{
			"need_search":           state.NeedSearch,
			"planned_category":      state.PlannedCategory,
			"category_was_inferred": state.CategoryWasInferred,
			"inferred_category":     inferredCategory,
			"category_mismatch":     state.CategoryMismatch,
		},
		LatencyMS: time.Since(stepStart).Milliseconds(),
	})
	return state, nil
}

// einoSearchKnowledgeNode 对应 Graph 的知识库检索节点。
func (s *Service) einoSearchKnowledgeNode(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	searchInput := tool.SearchKnowledgeInput{
		UserID:   state.UserID,
		Query:    state.Question,
		Category: state.PlannedCategory,
		TopK:     state.TopK,
	}

	stepStart := time.Now()
	searchOutputAny, err := s.searchTool.Call(ctx, searchInput)
	if err != nil {
		return state, err
	}
	searchOutput := searchOutputAny.(tool.SearchKnowledgeOutput)

	state.Chunks = searchOutput.Chunks
	state.AnswerChunks = selectAnswerChunks(searchOutput.Chunks)
	state.Steps = append(state.Steps, AgentStep{
		Step:        len(state.Steps) + 1,
		Thought:     "执行 Action：从个人八股知识库检索与问题最相关的 chunk。",
		Action:      s.searchTool.Name(),
		ActionInput: searchInput,
		Observation: buildSearchObservation(searchOutput.Chunks),
		LatencyMS:   time.Since(stepStart).Milliseconds(),
	})
	return state, nil
}

// einoAssessRetrievalNode 负责观察检索结果是否足够支撑回答。
func (s *Service) einoAssessRetrievalNode(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	_ = ctx
	stepStart := time.Now()

	topScore := float32(0)
	if len(state.Chunks) > 0 {
		topScore = state.Chunks[0].Score
	}
	state.RetrievalSufficient = len(state.AnswerChunks) > 0 && topScore >= minUsefulRetrievalScore
	state.AnswerMode = "rag_answer"
	if !state.RetrievalSufficient {
		state.AnswerMode = "no_context_answer"
	}

	state.Steps = append(state.Steps, AgentStep{
		Step:    len(state.Steps) + 1,
		Thought: "观察检索结果质量，判断是否足够支撑一个基于文档的面试回答。",
		Action:  "AssessRetrieval",
		Observation: map[string]any{
			"retrieved_count":      len(state.Chunks),
			"answer_chunk_count":   len(state.AnswerChunks),
			"top_score":            topScore,
			"min_useful_score":     minUsefulRetrievalScore,
			"retrieval_sufficient": state.RetrievalSufficient,
			"answer_mode":          state.AnswerMode,
		},
		LatencyMS: time.Since(stepStart).Milliseconds(),
	})
	return state, nil
}

// einoInterviewAnswerNode 对应 Graph 的最终回答节点，仅在检索足够时由分支进入。
func (s *Service) einoInterviewAnswerNode(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	answerInput := tool.InterviewAnswerInput{
		Question:       state.Question,
		Chunks:         state.AnswerChunks,
		HistorySummary: state.HistorySummary,
	}
	if state.StreamAnswer {
		state.Steps = append(state.Steps, AgentStep{
			Step:        len(state.Steps) + 1,
			Thought:     "检索结果足够，准备使用同一批片段进行流式面试回答。",
			Action:      s.answerTool.Name(),
			ActionInput: summarizeAnswerInput(answerInput),
			Observation: map[string]any{"stream_pending": true},
		})
		return state, nil
	}

	stepStart := time.Now()
	answerOutputAny, err := s.answerTool.Call(ctx, answerInput)
	if err != nil {
		return state, fmt.Errorf("generate answer: %w", err)
	}
	answerOutput := answerOutputAny.(tool.InterviewAnswerOutput)

	state.Answer = answerOutput.Answer
	state.Steps = append(state.Steps, AgentStep{
		Step:        len(state.Steps) + 1,
		Thought:     "检索结果足够，基于这些片段生成结构化面试回答并保留引用来源。",
		Action:      s.answerTool.Name(),
		ActionInput: summarizeAnswerInput(answerInput),
		Observation: map[string]any{"answer_chars": len([]rune(state.Answer))},
		LatencyMS:   time.Since(stepStart).Milliseconds(),
	})
	return state, nil
}

// einoNoContextAnswerNode 在检索不足时给出保守回答，避免模型脱离个人文档自由发挥。
func (s *Service) einoNoContextAnswerNode(ctx context.Context, state einoInterviewGraphState) (einoInterviewGraphState, error) {
	_ = ctx
	stepStart := time.Now()

	category := strings.TrimSpace(state.PlannedCategory)
	if category == "" {
		category = defaultNoContextCategory
	}
	state.AnswerChunks = nil
	state.Answer = buildNoContextAnswer(state.Question, category, state.CategoryMismatch)
	state.Steps = append(state.Steps, AgentStep{
		Step:    len(state.Steps) + 1,
		Thought: "检索结果不足，不能假装引用了个人文档，因此返回保守提示。",
		Action:  "NoContextAnswer",
		ActionInput: map[string]any{
			"question":         inputPreview(state.Question, 120),
			"planned_category": state.PlannedCategory,
		},
		Observation: map[string]any{
			"answer_chars":           len([]rune(state.Answer)),
			"retrieval_sufficient":   state.RetrievalSufficient,
			"category_mismatch":      state.CategoryMismatch,
			"fallback_answer_reason": "no_reliable_context",
		},
		LatencyMS: time.Since(stepStart).Milliseconds(),
	})
	return state, nil
}

func shouldAutoSelectCategory(category string) bool {
	normalized := strings.ToLower(strings.TrimSpace(category))
	return normalized == "" || normalized == "auto" || normalized == "all" || normalized == "全部"
}

func inferCategoryFromQuestion(question string) string {
	q := strings.ToLower(question)
	switch {
	case containsAny(q, "mysql", "innodb", "binlog", "redo log", "undo log", "mvcc", "索引", "事务", "隔离级别"):
		return "MySQL"
	case containsAny(q, "redis", "缓存", "rdb", "aof", "哨兵", "集群", "跳表", "过期删除"):
		return "Redis"
	case containsAny(q, "go", "golang", "goroutine", "gmp", "channel", "slice", "map", "gin", "gorm", "context"):
		return "Go"
	case containsAny(q, "java", "jvm", "spring", "springboot", "mybatis"):
		return "Java"
	default:
		return ""
	}
}

func containsAny(s string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(s, keyword) {
			return true
		}
	}
	return false
}

func inputPreview(s string, max int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

func buildNoContextAnswer(question string, category string, categoryMismatch bool) string {
	var b strings.Builder
	b.WriteString("## 一句话回答\n\n")
	b.WriteString("当前知识库没有检索到足够相关的内容，所以我不能把这个问题包装成基于个人文档的 RAG 回答。\n\n")
	b.WriteString("## 检索情况\n\n")
	b.WriteString("- 问题：")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n")
	b.WriteString("- 检索分类：")
	b.WriteString(category)
	b.WriteString("\n")
	if categoryMismatch {
		b.WriteString("- 可能原因：问题内容和你手动选择的分类不一致，可以切换分类或使用 Auto 分类后再试。\n")
	} else {
		b.WriteString("- 可能原因：对应文档还没有上传、没有完成索引，或者问题表述和笔记内容差异较大。\n")
	}
	b.WriteString("\n## 建议\n\n")
	b.WriteString("1. 确认相关 Markdown 已上传并完成索引。\n")
	b.WriteString("2. 如果你在固定分类下提问，可以尝试切换到更匹配的分类。\n")
	b.WriteString("3. 如果确认文档里有答案，可以换一个更接近笔记标题的问法。\n\n")
	b.WriteString("## 引用来源\n\n")
	b.WriteString("无。因为本轮没有足够可靠的检索片段可引用。\n")
	return b.String()
}
