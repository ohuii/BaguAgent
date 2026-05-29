package prompt

import (
	"fmt"
	"strings"

	"bagu-agent/backend/internal/vectorstore"
)

// BuildRAGPrompt 构造适合八股面试问答的 RAG Prompt。
func BuildRAGPrompt(question string, chunks []vectorstore.SearchResult, historySummary string) string {
	var b strings.Builder
	b.WriteString("你是一个资深后端面试官和候选人辅导老师。\n")
	b.WriteString("请基于给定知识库片段回答用户问题。\n\n")
	b.WriteString("规则：\n")
	b.WriteString("1. 优先依据知识库片段回答。\n")
	b.WriteString("2. 不要编造片段中没有的信息；如果资料不足，请明确说明“知识库中没有足够信息”。\n")
	b.WriteString("3. 回答要适合程序员面试表达，先简洁后展开。\n")
	b.WriteString("4. 必须给出引用来源，引用应来自实际使用过的片段。\n")
	b.WriteString("5. 如果知识片段之间冲突，请指出冲突并保守回答。\n\n")
	b.WriteString("用户问题：\n")
	b.WriteString(question)
	b.WriteString("\n\n历史对话摘要：\n")
	if historySummary == "" {
		b.WriteString("无")
	} else {
		b.WriteString(historySummary)
	}
	b.WriteString("\n\n知识库片段：\n")
	for i, chunk := range chunks {
		b.WriteString(fmt.Sprintf("[%d]\n", i+1))
		b.WriteString("标题路径：")
		b.WriteString(chunk.TitlePath)
		b.WriteString("\n来源文件：document_id=")
		b.WriteString(fmt.Sprintf("%d", chunk.DocumentID))
		b.WriteString("\n内容：\n")
		b.WriteString(chunk.Content)
		b.WriteString("\n\n")
	}
	b.WriteString("请按以下格式输出：\n\n")
	b.WriteString("## 一句话回答\n\n")
	b.WriteString("## 核心原理\n\n")
	b.WriteString("## 面试回答\n\n")
	b.WriteString("## 常见追问\n\n")
	b.WriteString("## 常见误区\n\n")
	b.WriteString("## 引用来源\n")
	return b.String()
}
