package rag

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Section is a nonempty Markdown body with its heading ancestry, in source order.
type Section struct {
	HeadingPath []string
	Content     string
}

// ParseMarkdown recognizes top-level ATX (#) and Setext (===/---) headings.
// Code blocks, HTML blocks, lists and blockquotes retain their original Markdown;
// headings nested in these containers do not change the document's section path.
// Text before the first heading has an empty HeadingPath. Empty sections are skipped.
func ParseMarkdown(markdown string) ([]Section, error) {
	if !utf8.ValidString(markdown) {
		return nil, fmt.Errorf("markdown is not valid UTF-8")
	}
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("empty markdown")
	}
	source := []byte(markdown)
	document := goldmark.New().Parser().Parse(text.NewReader(source))
	type heading struct {
		level int
		title string
	}
	stack := make([]heading, 0, 6)
	sections := make([]Section, 0)
	bodyStart := 0
	appendSection := func(end int) {
		content := strings.TrimSpace(markdown[bodyStart:end])
		if content == "" {
			return
		}
		path := make([]string, len(stack))
		for i, h := range stack {
			path[i] = h.title
		}
		sections = append(sections, Section{HeadingPath: path, Content: content})
	}
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		h, ok := node.(*ast.Heading)
		if !ok || h.Lines().Len() == 0 {
			continue
		}
		first := h.Lines().At(0)
		start := strings.LastIndexByte(markdown[:first.Start], '\n') + 1
		appendSection(start)
		last := h.Lines().At(h.Lines().Len() - 1)
		end := last.Stop
		if end == 0 || markdown[end-1] != '\n' {
			end = lineEnd(markdown, end)
		}
		// Setext's underline is not included in the heading's text segments.
		if !strings.HasPrefix(strings.TrimLeft(markdown[start:], " "), "#") {
			end = lineEnd(markdown, end)
		}
		for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, heading{level: h.Level, title: strings.TrimSpace(string(h.Text(source)))})
		bodyStart = end
	}
	appendSection(len(markdown))
	return sections, nil
}

func lineEnd(s string, start int) int {
	if i := strings.IndexByte(s[start:], '\n'); i >= 0 {
		return start + i + 1
	}
	return len(s)
}
