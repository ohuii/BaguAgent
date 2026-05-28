package markdown

import (
	"strings"
	"testing"
)

func TestChunkMarkdownWithHeadingPath(t *testing.T) {
	source := []byte(`# Go 八股

## GMP 模型

### G 是什么

G 表示 Goroutine，是 Go 语言中的轻量级协程。

### M 是什么

M 表示 Machine，对应操作系统线程。

## GC 垃圾回收

### 三色标记法

三色标记法包含白色、灰色和黑色对象。
`)

	chunks, err := ChunkMarkdown(source, "go_interview.md", "Go", DefaultChunkerOptions())
	if err != nil {
		t.Fatalf("ChunkMarkdown() error = %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got empty")
	}

	var found bool
	for _, c := range chunks {
		if strings.Contains(c.ContentWithTitle, "Go 八股 / GMP 模型 / G 是什么") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected title path in content_with_title, chunks = %+v", chunks)
	}
}

func TestSplitMarkdownBlocksKeepsFence(t *testing.T) {
	content := "说明文字。\n\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n\n结论。"
	blocks := splitMarkdownBlocks(content)

	var codeBlock string
	for _, block := range blocks {
		if strings.Contains(block, "func main") {
			codeBlock = block
			break
		}
	}
	if !strings.Contains(codeBlock, "```go") || !strings.Contains(codeBlock, "```") {
		t.Fatalf("expected fenced code block to stay complete, block = %q", codeBlock)
	}
}
