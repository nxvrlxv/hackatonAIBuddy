package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

const claimsKey = "claims"

type TokenParser interface {
	Parse(token string) (*auth.Claims, error)
}

// JWT токен переделывается под MCP
func Verifier(parser TokenParser) mcpauth.TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
		claims, err := parser.Parse(token)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
		}
		return &mcpauth.TokenInfo{
			UserID:     strconv.FormatInt(claims.UserID, 10), // SDK привязывает MCP-сессию к пользователю
			Expiration: claims.ExpiresAt.Time,                // без него SDK отклонит токен
			Extra:      map[string]any{claimsKey: claims},
		}, nil
	}
}

var ErrUnauthenticated = errors.New("unauthenticated")

// ClaimsFromRequest достаёт пользователя в тулзе.
func ClaimsFromRequest(req *mcp.CallToolRequest) (*auth.Claims, error) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return nil, ErrUnauthenticated
	}
	claims, ok := req.Extra.TokenInfo.Extra[claimsKey].(*auth.Claims)
	if !ok || claims == nil {
		return nil, ErrUnauthenticated
	}
	return claims, nil
}
