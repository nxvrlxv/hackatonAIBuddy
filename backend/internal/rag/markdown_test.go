package rag

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Section
	}{
		{"hierarchy and preamble", "Intro\n# Root\nBody\n### Deep\nNested\n## Sibling\nOther\n# Next\nEnd", []Section{
			{[]string{}, "Intro"}, {[]string{"Root"}, "Body"},
			{[]string{"Root", "Deep"}, "Nested"},
			{[]string{"Root", "Sibling"}, "Other"}, {[]string{"Next"}, "End"},
		}},
		{"fenced code", "# Root\n```md\n# Not heading\n```\nText", []Section{
			{[]string{"Root"}, "```md\n# Not heading\n```\nText"},
		}},
		{"setext and CRLF", "Root\r\n====\r\nBody\r\n\r\nChild\r\n----\r\nTail", []Section{
			{[]string{"Root"}, "Body"}, {[]string{"Root", "Child"}, "Tail"},
		}},
		{"empty sections and repeated headings", "# Root\n## Same\nA\n## Same\nB\n## Empty", []Section{
			{[]string{"Root", "Same"}, "A"}, {[]string{"Root", "Same"}, "B"},
		}},
		{"blockquote and indented code", "# Root\n> # Quote\n\n    # Code\n", []Section{
			{[]string{"Root"}, "> # Quote\n\n    # Code"},
		}},
		{"heading only", "# Root", []Section{}},
		{"no headings", "Plain text", []Section{{[]string{}, "Plain text"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMarkdown(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v; want %#v", got, tt.want)
			}
		})
	}
}

func TestSplitMarkdownUnicodeAndBudget(t *testing.T) {
	body := strings.Repeat("\u041f\u0440\u0438\u0432\u0435\u0442\U0001F600", 25)
	chunks, err := SplitMarkdown("# Root\n"+body+"\n## Child\nSecond", 40, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Fatalf("bad index: %d", c.ChunkIndex)
		}
		if !utf8.ValidString(c.EmbeddingText) || utf8.RuneCountInString(c.EmbeddingText) > 40 {
			t.Fatalf("invalid or oversized chunk: %q", c.EmbeddingText)
		}
		if c.EmbeddingText != "Section: "+c.SectionTitle+"\n\n"+c.Content {
			t.Fatal("missing heading prefix")
		}
	}
	last := chunks[len(chunks)-1]
	if !reflect.DeepEqual(last.HeadingPath, []string{"Root", "Child"}) || last.Content != "Second" {
		t.Fatalf("wrong final section: %#v", last)
	}
}

func TestSplitMarkdownPreservesBodyWithoutOverlap(t *testing.T) {
	body := strings.Repeat("\u042f\U0001F600", 100)
	chunks, err := SplitMarkdown("# A\n"+body, 25, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for _, c := range chunks {
		got.WriteString(c.Content)
	}
	if got.String() != body {
		t.Fatal("body text was lost or duplicated")
	}
}

func TestChunkValidation(t *testing.T) {
	for _, input := range []string{"", "  \n", string([]byte{0xff})} {
		if _, err := ParseMarkdown(input); err == nil {
			t.Fatalf("accepted invalid input %q", input)
		}
	}
	if _, err := SplitMarkdown("# Very long heading\nBody", 10, 0); err == nil {
		t.Fatal("accepted oversized prefix")
	}
	for _, overlap := range []int{-1, 20, 21} {
		if _, err := SplitText("body", 20, overlap); err == nil {
			t.Fatalf("accepted overlap %d", overlap)
		}
	}
	if _, err := SplitMarkdown("# A\nBody", 20, 19); err != nil {
		t.Fatal(err)
	}
}
