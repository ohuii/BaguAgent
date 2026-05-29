package tool

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"

	einotool "github.com/cloudwego/eino/components/tool"
	einotoolutils "github.com/cloudwego/eino/components/tool/utils"
)

// EinoSearchKnowledgeToolName 是 ReAct Agent 中知识库检索工具的名字。
// 这是模型在 tool-calling 时看到的工具名，需要保持稳定。
const EinoSearchKnowledgeToolName = "search_knowledge"

// einoSearchArgs 是模型生成的检索参数，schema 由 struct tag 推断后交给大模型。
// 注意：user_id / top_k 不暴露给模型，由服务端从请求上下文注入，避免模型乱填越权。
type einoSearchArgs struct {
	Query    string `json:"query" jsonschema:"required,description=要在用户个人八股知识库中检索的问题或关键词"`
	Category string `json:"category,omitempty" jsonschema:"description=可选的知识分类，例如 Go / MySQL / Redis / Java；不确定时留空表示全部分类"`
}

// reactRequestCtxKey 用于在 ctx 中携带单次 ReAct 请求的上下文。
type reactRequestCtxKey struct{}

// ReactRequest 保存一次 ReAct 运行的请求级状态。
// 工具调用发生在 Agent 内部，服务层拿不到检索细节，因此用一个收集器记录每次检索结果，
// 运行结束后服务层据此构建 citations 和 retrieved_chunks。
type ReactRequest struct {
	UserID uint64
	TopK   int

	mu       sync.Mutex
	chunks   []vectorstore.SearchResult
	searches []ReactSearchObservation
}

// ReactSearchObservation 记录一次检索调用的观察结果，便于回放 Agent 步骤。
type ReactSearchObservation struct {
	Query    string
	Category string
	Count    int
	UIDs     []string
}

func (r *ReactRequest) record(query, category string, chunks []vectorstore.SearchResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chunks = append(r.chunks, chunks...)
	uids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c.ChunkUID != "" {
			uids = append(uids, c.ChunkUID)
		}
	}
	r.searches = append(r.searches, ReactSearchObservation{
		Query:    query,
		Category: category,
		Count:    len(chunks),
		UIDs:     uids,
	})
}

// Chunks 返回本次 ReAct 运行累计检索到的去重 chunk（按首次出现顺序）。
func (r *ReactRequest) Chunks() []vectorstore.SearchResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[string]bool, len(r.chunks))
	out := make([]vectorstore.SearchResult, 0, len(r.chunks))
	for _, c := range r.chunks {
		if c.ChunkUID != "" {
			if seen[c.ChunkUID] {
				continue
			}
			seen[c.ChunkUID] = true
		}
		out = append(out, c)
	}
	return out
}

// Searches 返回本次运行发生过的检索调用观察序列。
func (r *ReactRequest) Searches() []ReactSearchObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ReactSearchObservation, len(r.searches))
	copy(out, r.searches)
	return out
}

// WithReactRequest 把请求级上下文放入 ctx，供工具读取 user_id/top_k 并回填检索结果。
func WithReactRequest(ctx context.Context, req *ReactRequest) context.Context {
	return context.WithValue(ctx, reactRequestCtxKey{}, req)
}

func reactRequestFromCtx(ctx context.Context) *ReactRequest {
	req, _ := ctx.Value(reactRequestCtxKey{}).(*ReactRequest)
	return req
}

// NewEinoSearchKnowledgeTool 创建可被 Eino ReAct Agent 调用的原生检索工具。
// 工具的 JSON schema 从 einoSearchArgs 推断，模型据此自主决定何时检索、用什么 query/category。
func NewEinoSearchKnowledgeTool(retr *retriever.Service) (einotool.InvokableTool, error) {
	desc := "从用户的个人八股知识库（Markdown 笔记）检索与问题最相关的片段。" +
		"回答任何技术面试问题前都应先调用本工具，不要凭空作答。" +
		"如果检索结果为空或不相关，请如实说明知识库里没有，不要编造。"

	run := func(ctx context.Context, args einoSearchArgs) (string, error) {
		req := reactRequestFromCtx(ctx)
		userID := uint64(1)
		topK := 5
		if req != nil {
			if req.UserID > 0 {
				userID = req.UserID
			}
			if req.TopK > 0 {
				topK = req.TopK
			}
		}

		chunks, err := retr.Search(ctx, retriever.SearchInput{
			UserID:   userID,
			Query:    strings.TrimSpace(args.Query),
			Category: strings.TrimSpace(args.Category),
			TopK:     topK,
		})
		if err != nil {
			return "", err
		}
		if req != nil {
			req.record(args.Query, args.Category, chunks)
		}
		return formatChunksForModel(chunks), nil
	}

	return einotoolutils.InferTool(EinoSearchKnowledgeToolName, desc, run)
}

// formatChunksForModel 把检索结果渲染成模型易读的文本，供其组织最终回答。
func formatChunksForModel(chunks []vectorstore.SearchResult) string {
	if len(chunks) == 0 {
		return "知识库中没有检索到相关片段。请明确告知用户当前没有可引用的笔记内容，不要编造答案。"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("检索到 %d 个相关片段：\n\n", len(chunks)))
	for i, c := range chunks {
		b.WriteString(fmt.Sprintf("[%d] 标题路径：%s（分类：%s，相关度：%.3f）\n", i+1, c.TitlePath, c.Category, c.Score))
		b.WriteString(strings.TrimSpace(c.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
