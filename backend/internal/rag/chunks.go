package rag

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tmc/langchaingo/textsplitter"
)

const (
	defaultSize    = 600
	defaultOverlap = 65
)

// Chunk keeps source metadata separate from the text sent to the embedding model.
type Chunk struct {
	HeadingPath   []string
	SectionTitle  string
	Content       string
	EmbeddingText string
	ChunkIndex    int
}

// SplitText splits a section body, measuring size and overlap in Unicode code points.
// A nonpositive size selects the default; zero overlap explicitly disables overlap.
func SplitText(text string, size, overlap int) ([]string, error) {
	if size <= 0 {
		size = defaultSize
	}
	if overlap < 0 || overlap >= size {
		return nil, fmt.Errorf("overlap must satisfy 0 <= overlap < size (%d)", size)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("text is not valid UTF-8")
	}
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(size),
		textsplitter.WithChunkOverlap(overlap),
		textsplitter.WithLenFunc(utf8.RuneCountInString),
	)
	chunks, err := splitter.SplitText(text)
	if err != nil {
		return nil, fmt.Errorf("split text: %w", err)
	}
	return chunks, nil
}

// SplitMarkdown applies SplitText to every nonempty Markdown section.
// size includes the repeated heading prefix in EmbeddingText. Pass (0, 0) for
// the default size without overlap, or use DefaultChunks for both defaults.
func SplitMarkdown(markdown string, size, overlap int) ([]Chunk, error) {
	if size <= 0 {
		size = defaultSize
	}
	if overlap < 0 || overlap >= size {
		return nil, fmt.Errorf("overlap must satisfy 0 <= overlap < size (%d)", size)
	}
	sections, err := ParseMarkdown(markdown)
	if err != nil {
		return nil, err
	}
	chunks := make([]Chunk, 0)
	for _, section := range sections {
		title := strings.Join(section.HeadingPath, " \u2192 ")
		prefix := ""
		if title != "" {
			prefix = "Section: " + title + "\n\n"
		}
		bodySize := size - utf8.RuneCountInString(prefix)
		if bodySize <= 0 {
			return nil, fmt.Errorf("heading path %q leaves no room for content within size %d", title, size)
		}
		// A long heading reduces the body budget. Keep overlap valid and ensure progress.
		bodyOverlap := min(overlap, bodySize-1)
		parts, err := SplitText(section.Content, bodySize, bodyOverlap)
		if err != nil {
			return nil, fmt.Errorf("section %q: %w", title, err)
		}
		for _, part := range parts {
			embeddingText := prefix + part
			if utf8.RuneCountInString(embeddingText) > size {
				return nil, fmt.Errorf("section %q: chunk exceeds size %d", title, size)
			}
			chunks = append(chunks, Chunk{
				HeadingPath:   append([]string(nil), section.HeadingPath...),
				SectionTitle:  title,
				Content:       part,
				EmbeddingText: embeddingText,
				ChunkIndex:    len(chunks),
			})
		}
	}
	return chunks, nil
}

// DefaultChunks uses the default size and overlap, both measured in Unicode code points.
func DefaultChunks(markdown string) ([]Chunk, error) {
	return SplitMarkdown(markdown, defaultSize, defaultOverlap)
}
