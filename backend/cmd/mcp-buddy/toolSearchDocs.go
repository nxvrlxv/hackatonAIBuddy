package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
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

// DocSearcher — поиск по базе знаний (реализует rag.ServiceRAG).
type DocSearcher interface {
	Query(ctx context.Context, question string, teamID *int64) ([]rag.SearchResult, error)
}

func RegisterSearchDocs(server *mcp.Server, svc DocSearcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "search_docs",
		Description: "Ищет во внутренней базе знаний (онбординг, процессы, команды) фрагменты по вопросу" +
			" и возвращает их с источником. Если found=false — в документах ответа нет, не выдумывай.",
	}, searchDocs(svc))
}

// searchDocs ищет с фильтром по команде пользователя из токена.
func searchDocs(svc DocSearcher) mcp.ToolHandlerFor[SearchInput, SearchDocsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchDocsOutput, error) {
		claims, err := mcpserver.ClaimsFromRequest(req)
		if err != nil {
			return nil, SearchDocsOutput{}, err
		}
		results, err := svc.Query(ctx, in.Question, &claims.TeamID)
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
	}
}
