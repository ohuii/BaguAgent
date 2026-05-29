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

func TestInferCategory(t *testing.T) {
	tests := []struct {
		name      string
		titlePath string
		fallback  string
		want      string
	}{
		{
			name:      "golang",
			titlePath: "一、Golang 八股 / 2. GMP 调度",
			fallback:  "Interview",
			want:      "Go",
		},
		{
			name:      "mysql",
			titlePath: "二、MySQL 八股 / 3. MVCC / undo log",
			fallback:  "Interview",
			want:      "MySQL",
		},
		{
			name:      "redis",
			titlePath: "三、Redis 八股 / IO 多路复用",
			fallback:  "Interview",
			want:      "Redis",
		},
		{
			name:      "fallback",
			titlePath: "面试经验 / 项目介绍",
			fallback:  "General",
			want:      "General",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferCategory(tt.titlePath, tt.fallback); got != tt.want {
				t.Fatalf("InferCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChunkMarkdownFiltersDecorativeBlocks(t *testing.T) {
	source := []byte(`# Go 八股

## GMP 模型

### 元信息

> 重要程度：⭐⭐⭐⭐⭐（5/5）
> 出现频率：⭐⭐⭐⭐⭐（5/5）

#####

### 核心结论

GMP 是 Go runtime 的 goroutine 调度模型。
`)

	chunks, err := ChunkMarkdown(source, "go.md", "Go", DefaultChunkerOptions())
	if err != nil {
		t.Fatalf("ChunkMarkdown() error = %v", err)
	}
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "重要程度") || strings.Contains(chunk.Content, "#####") {
			t.Fatalf("decorative content should be filtered, chunk = %+v", chunk)
		}
	}
}
