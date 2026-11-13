package auth

import (
	"context"
	"fmt"
)

type contKey struct{}

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, contKey{}, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, error) {
	claims, ok := ctx.Value(contKey{}).(*Claims)
	if !ok || claims == nil {
		return nil, fmt.Errorf("empty claims")
	}

	return claims, nil
}
