package markdown

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const titleSeparator = " / "

// Chunk 是 Markdown 解析后得到的业务 chunk，不直接绑定数据库模型。
type Chunk struct {
	TitlePath        string
	HeadingLevel     int
	Content          string
	ContentWithTitle string
	ChunkIndex       int
	TokenCount       int
	SourceFile       string
	Category         string
}

// ChunkerOptions 控制 Markdown 语义切分粒度。
type ChunkerOptions struct {
	MinTokens     int
	TargetTokens  int
	MaxTokens     int
	OverlapTokens int
}

// DefaultChunkerOptions 返回适合八股文档的默认 chunk 参数。
func DefaultChunkerOptions() ChunkerOptions {
	return ChunkerOptions{
		MinTokens:     80,
		TargetTokens:  500,
		MaxTokens:     800,
		OverlapTokens: 80,
	}
}

type headingInfo struct {
	Level        int
	Title        string
	Start        int
	ContentStart int
	ContentEnd   int
	TitlePath    string
	ParentPath   string
}

type headingStackItem struct {
	Level int
	Title string
}

// ChunkMarkdown 按 Markdown 标题层级生成语义 chunk。
// embedding 时应使用 ContentWithTitle，从而让标题路径参与召回。
func ChunkMarkdown(source []byte, sourceFile string, category string, opts ChunkerOptions) ([]Chunk, error) {
	if opts.MaxTokens <= 0 {
		opts = DefaultChunkerOptions()
	}

	headings, err := parseHeadings(source, sourceFile)
	if err != nil {
		return nil, err
	}

	var chunks []Chunk
	for _, h := range headings {
		content := cleanSectionContent(string(source[h.ContentStart:h.ContentEnd]))
		if content == "" {
			continue
		}

		parts := splitSectionContent(content, opts)
		for _, part := range parts {
			tokenCount := EstimateTokens(part)
			chunks = append(chunks, Chunk{
				TitlePath:        h.TitlePath,
				HeadingLevel:     h.Level,
				Content:          part,
				ContentWithTitle: h.TitlePath + "\n" + part,
				TokenCount:       tokenCount,
				SourceFile:       sourceFile,
				Category:         category,
			})
		}
	}

	chunks = mergeShortChunks(chunks, opts)
	for i := range chunks {
		chunks[i].ChunkIndex = i
	}
	return chunks, nil
}

func parseHeadings(source []byte, sourceFile string) ([]headingInfo, error) {
	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(source))

	var headings []headingInfo
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != ast.KindHeading {
			return ast.WalkContinue, nil
		}

		h := n.(*ast.Heading)
		lines := h.Lines()
		if lines.Len() == 0 {
			return ast.WalkContinue, nil
		}

		first := lines.At(0)
		last := lines.At(lines.Len() - 1)
		title := strings.TrimSpace(string(h.Text(source)))
		if title == "" {
			title = "未命名标题"
		}

		headings = append(headings, headingInfo{
			Level:        h.Level,
			Title:        title,
			Start:        first.Start,
			ContentStart: last.Stop,
		})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk markdown ast: %w", err)
	}

	if len(headings) == 0 {
		title := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
		if title == "" || title == "." {
			title = "Markdown 文档"
		}
		return []headingInfo{{
			Level:        1,
			Title:        title,
			Start:        0,
			ContentStart: 0,
			ContentEnd:   len(source),
			TitlePath:    title,
		}}, nil
	}

	for i := range headings {
		if i+1 < len(headings) {
			headings[i].ContentEnd = headings[i+1].Start
		} else {
			headings[i].ContentEnd = len(source)
		}
	}
	fillTitlePaths(headings)
	return headings, nil
}

func fillTitlePaths(headings []headingInfo) {
	var stack []headingStackItem
	for i := range headings {
		for len(stack) > 0 && stack[len(stack)-1].Level >= headings[i].Level {
			stack = stack[:len(stack)-1]
		}

		parentTitles := make([]string, 0, len(stack))
		for _, item := range stack {
			parentTitles = append(parentTitles, item.Title)
		}
		headings[i].ParentPath = strings.Join(parentTitles, titleSeparator)

		currentTitles := append(parentTitles, headings[i].Title)
		headings[i].TitlePath = strings.Join(currentTitles, titleSeparator)

		stack = append(stack, headingStackItem{
			Level: headings[i].Level,
			Title: headings[i].Title,
		})
	}
}

func cleanSectionContent(content string) string {
	return strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
}

func splitSectionContent(content string, opts ChunkerOptions) []string {
	if EstimateTokens(content) <= opts.MaxTokens {
		return []string{content}
	}

	blocks := splitMarkdownBlocks(content)
	var chunks []string
	var current []string
	currentTokens := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.TrimSpace(strings.Join(current, "")))
		current = overlapBlocks(current, opts.OverlapTokens)
		currentTokens = 0
		for _, block := range current {
			currentTokens += EstimateTokens(block)
		}
	}

	for _, block := range blocks {
		blockTokens := EstimateTokens(block)
		if currentTokens > 0 && currentTokens+blockTokens > opts.MaxTokens {
			flush()
		}
		current = append(current, block)
		currentTokens += blockTokens
	}

	if len(current) > 0 {
		chunks = append(chunks, strings.TrimSpace(strings.Join(current, "")))
	}
	return chunks
}

// splitMarkdownBlocks 按空行切分 Markdown block，并把 fenced code block 作为原子内容保留。
func splitMarkdownBlocks(content string) []string {
	lines := strings.SplitAfter(content, "\n")
	var blocks []string
	var current strings.Builder
	inFence := false
	fenceMarker := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFenceLine(trimmed) {
			marker := trimmed[:3]
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			current.WriteString(line)
			continue
		}

		current.WriteString(line)
		if !inFence && trimmed == "" {
			block := current.String()
			if strings.TrimSpace(block) != "" {
				blocks = append(blocks, block)
			}
			current.Reset()
		}
	}

	if strings.TrimSpace(current.String()) != "" {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func overlapBlocks(blocks []string, overlapTokens int) []string {
	if overlapTokens <= 0 || len(blocks) == 0 {
		return nil
	}

	var selected []string
	total := 0
	for i := len(blocks) - 1; i >= 0; i-- {
		selected = append([]string{blocks[i]}, selected...)
		total += EstimateTokens(blocks[i])
		if total >= overlapTokens {
			break
		}
	}
	return selected
}

func mergeShortChunks(chunks []Chunk, opts ChunkerOptions) []Chunk {
	if opts.MinTokens <= 0 || len(chunks) < 2 {
		return chunks
	}

	merged := make([]Chunk, 0, len(chunks))
	for i := 0; i < len(chunks); i++ {
		current := chunks[i]
		if current.TokenCount >= opts.MinTokens || i+1 >= len(chunks) {
			merged = append(merged, current)
			continue
		}

		next := chunks[i+1]
		if sameParent(current.TitlePath, next.TitlePath) && current.TokenCount+next.TokenCount <= opts.MaxTokens {
			parent := parentPath(current.TitlePath)
			if parent == "" {
				parent = current.TitlePath
			}
			content := "### " + current.TitlePath + "\n" + current.Content + "\n\n" +
				"### " + next.TitlePath + "\n" + next.Content
			merged = append(merged, Chunk{
				TitlePath:        parent,
				HeadingLevel:     min(current.HeadingLevel, next.HeadingLevel),
				Content:          content,
				ContentWithTitle: parent + "\n" + content,
				TokenCount:       EstimateTokens(content),
				SourceFile:       current.SourceFile,
				Category:         current.Category,
			})
			i++
			continue
		}

		merged = append(merged, current)
	}
	return merged
}

func sameParent(a, b string) bool {
	return parentPath(a) == parentPath(b)
}

func parentPath(path string) string {
	idx := strings.LastIndex(path, titleSeparator)
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

// EstimateTokens 是轻量 token 估算：中文字符按 1 token，连续英文按约 4 字符 1 token。
// 第一版避免引入具体模型 tokenizer，后续可按 embedding 模型替换。
func EstimateTokens(s string) int {
	cjk := 0
	ascii := 0
	other := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		switch {
		case r <= unicode.MaxASCII:
			ascii++
		case unicode.Is(unicode.Han, r):
			cjk++
		default:
			other++
		}
	}

	tokens := cjk + other + (ascii+3)/4
	if tokens == 0 && strings.TrimSpace(s) != "" {
		return 1
	}
	return tokens
}
