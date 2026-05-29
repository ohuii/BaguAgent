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
		MinTokens:     160,
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
			if isDecorativeContent(part) {
				continue
			}
			tokenCount := EstimateTokens(part)
			chunks = append(chunks, Chunk{
				TitlePath:        h.TitlePath,
				HeadingLevel:     h.Level,
				Content:          part,
				ContentWithTitle: h.TitlePath + "\n" + part,
				TokenCount:       tokenCount,
				SourceFile:       sourceFile,
				Category:         InferCategory(h.TitlePath, category),
			})
		}
	}

	chunks = mergeShortChunks(chunks, opts)
	for i := range chunks {
		chunks[i].ChunkIndex = i
	}
	return chunks, nil
}

// InferCategory 根据标题路径推断 chunk 分类。
// 汇总类 Markdown 通常在一级标题里包含 Go、MySQL、Redis 等主题，优先用标题路径识别。
func InferCategory(titlePath string, fallback string) string {
	normalized := strings.ToLower(titlePath)
	rules := []struct {
		category string
		keywords []string
	}{
		{category: "Go", keywords: []string{"golang", "go 八股", "go语言", "go 语言", "goroutine", "gmp"}},
		{category: "MySQL", keywords: []string{"mysql", "innodb", "mvcc", "事务", "索引"}},
		{category: "Redis", keywords: []string{"redis", "缓存", "rdb", "aof"}},
		{category: "OS", keywords: []string{"操作系统", "进程", "线程", "内存管理", "死锁"}},
		{category: "Network", keywords: []string{"计算机网络", "网络", "tcp", "udp", "http", "https"}},
		{category: "Linux", keywords: []string{"linux", "shell"}},
		{category: "Docker", keywords: []string{"docker", "容器", "kubernetes", "k8s"}},
	}

	for _, rule := range rules {
		for _, keyword := range rule.keywords {
			if strings.Contains(normalized, strings.ToLower(keyword)) {
				return rule.category
			}
		}
	}
	return strings.TrimSpace(fallback)
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
	content = strings.ReplaceAll(content, "\r\n", "\n")
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if isNoiseLine(trimmed) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
		if isDecorativeContent(current.Content) {
			continue
		}
		if current.TokenCount >= opts.MinTokens || i+1 >= len(chunks) {
			merged = append(merged, current)
			continue
		}

		group := []Chunk{current}
		totalTokens := current.TokenCount
		parent := parentPath(current.TitlePath)
		for j := i + 1; j < len(chunks); j++ {
			next := chunks[j]
			if isDecorativeContent(next.Content) {
				i = j
				continue
			}
			if parentPath(next.TitlePath) != parent || totalTokens+next.TokenCount > opts.MaxTokens {
				break
			}
			group = append(group, next)
			totalTokens += next.TokenCount
			i = j
			if totalTokens >= opts.TargetTokens {
				break
			}
		}

		if len(group) > 1 {
			merged = append(merged, mergeChunkGroup(group))
		} else {
			merged = append(merged, current)
		}
	}
	return merged
}

func mergeChunkGroup(group []Chunk) Chunk {
	parent := parentPath(group[0].TitlePath)
	if parent == "" {
		parent = group[0].TitlePath
	}
	headingLevel := group[0].HeadingLevel
	var b strings.Builder
	for i, chunk := range group {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("### ")
		b.WriteString(chunk.TitlePath)
		b.WriteString("\n")
		b.WriteString(chunk.Content)
		headingLevel = min(headingLevel, chunk.HeadingLevel)
	}

	content := strings.TrimSpace(b.String())
	return Chunk{
		TitlePath:        parent,
		HeadingLevel:     headingLevel,
		Content:          content,
		ContentWithTitle: parent + "\n" + content,
		TokenCount:       EstimateTokens(content),
		SourceFile:       group[0].SourceFile,
		Category:         group[0].Category,
	}
}

func parentPath(path string) string {
	idx := strings.LastIndex(path, titleSeparator)
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

func isNoiseLine(line string) bool {
	if line == "" {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if strings.Trim(trimmed, "#") == "" {
		return true
	}
	if strings.Contains(trimmed, "重要程度") || strings.Contains(trimmed, "出现频率") {
		return true
	}
	return false
}

func isDecorativeContent(content string) bool {
	cleaned := cleanDecorativeContent(content)
	return EstimateTokens(cleaned) < 8
}

func cleanDecorativeContent(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isNoiseLine(trimmed) {
			continue
		}
		lines = append(lines, trimmed)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
