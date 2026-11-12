package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
)

type SearchInput struct {
	Question string `json:"question" jsonschema:"вопрос стажера на естественном языке"`
}

type DocFragment struct {
	Source   string  `json:"source"`
	Title    string  `json:"title"`
	Section  string  `json:"section,omitempty"`
	Content  string  `json:"content"`
	Distance float64 `json:"distance"`
}

type SearchDocsOutput struct {
	Found     bool          `json:"found"`
	Fragments []DocFragment `json:"fragments"`
	Message   string        `json:"message,omitempty"`
}

func RegisterSearchDocs(server *mcp.Server, svc *rag.ServiceRAG) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "search_docs",
		Description: "Ищет во внутреннней базе знаний (онбординг, процессы, команды ) фрагменты по вопросу" +
			"и возвращает их с источником. Если found=false — в документах ответа нет, не выдумывай.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchDocsOutput, error) {
		results, err := svc.Query(ctx, in.Question, nil) // teamID — после JWT
		if err != nil {
			return nil, SearchDocsOutput{}, err
		}
		out := SearchDocsOutput{Fragments: []DocFragment{}} // не nil: SDK проверяет схему
		for _, r := range results {
			out.Fragments = append(out.Fragments, DocFragment{
				Source: r.SourcePath, Title: r.Title, Section: r.SectionTitle,
				Content: r.Content, Distance: r.Distance,
			})
		}
		out.Found = len(out.Fragments) > 0
		if !out.Found {
			out.Message = "В базе знаний нет ответа на этот вопрос"
		}
		return nil, out, nil
	})
}
