package main

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
)

// fakeSearcher запоминает аргументы и отдаёт заданный результат.
type fakeSearcher struct {
	results  []rag.SearchResult
	err      error
	question string
	teamID   *int64
}

func (f *fakeSearcher) Query(_ context.Context, question string, teamID *int64) ([]rag.SearchResult, error) {
	f.question, f.teamID = question, teamID
	return f.results, f.err
}

func TestSearchDocsFound(t *testing.T) {
	svc := &fakeSearcher{results: []rag.SearchResult{
		{SourcePath: "travel/onboarding.md", Title: "Онбординг", SectionTitle: "Первый день", Content: "Получи ноутбук", Distance: 0.2},
	}}
	user := &auth.User{ID: 5, TeamID: 3}

	_, out, err := searchDocs(svc)(context.Background(), requestFor(t, user), SearchInput{Question: "что делать в первый день?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.question != "что делать в первый день?" || svc.teamID == nil || *svc.teamID != 3 {
		t.Fatalf("search must be filtered by team from token: question %q, team %v", svc.question, svc.teamID)
	}
	if !out.Found || len(out.Fragments) != 1 || out.Message != "" {
		t.Fatalf("unexpected output: %+v", out)
	}
	f := out.Fragments[0]
	if f.Source != "travel/onboarding.md" || f.Section != "Первый день" || f.Content != "Получи ноутбук" || f.Distance != 0.2 {
		t.Fatalf("unexpected fragment: %+v", f)
	}
}

func TestSearchDocsNotFound(t *testing.T) {
	_, out, err := searchDocs(&fakeSearcher{})(context.Background(), requestFor(t, &auth.User{ID: 5, TeamID: 3}), SearchInput{Question: "?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Found || out.Fragments == nil || out.Message == "" {
		t.Fatalf("want found=false, empty non-nil fragments and message, got %+v", out)
	}
}

func TestSearchDocsErrors(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		svc := &fakeSearcher{}
		_, _, err := searchDocs(svc)(context.Background(), &mcp.CallToolRequest{}, SearchInput{Question: "?"})
		if !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
		if svc.question != "" {
			t.Fatal("search must not run without user")
		}
	})
	t.Run("search error", func(t *testing.T) {
		searchErr := errors.New("ollama down")
		_, _, err := searchDocs(&fakeSearcher{err: searchErr})(context.Background(), requestFor(t, &auth.User{ID: 5}), SearchInput{Question: "?"})
		if !errors.Is(err, searchErr) {
			t.Fatalf("err = %v, want search error", err)
		}
	})
}
