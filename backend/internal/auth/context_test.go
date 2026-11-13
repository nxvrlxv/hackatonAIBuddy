package auth

import (
	"context"
	"testing"
)

func TestClaimsContext(t *testing.T) {
	t.Run("empty context", func(t *testing.T) {
		claims, err := ClaimsFromContext(context.Background())
		if err == nil || claims != nil {
			t.Fatalf("want error and nil claims, got %+v, %v", claims, err)
		}
	})

	t.Run("nil claims", func(t *testing.T) {
		claims, err := ClaimsFromContext(WithClaims(context.Background(), nil))
		if err == nil || claims != nil {
			t.Fatalf("want error and nil claims, got %+v, %v", claims, err)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		want := &Claims{UserID: 5, TeamID: 3, Role: "intern"}
		got, err := ClaimsFromContext(WithClaims(context.Background(), want))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatal("must return the stored claims")
		}
	})
}
