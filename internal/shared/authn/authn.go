// Package authn carries the authenticated principal through request contexts.
package authn

import "context"

// Principal identifies the authenticated user and session.
type Principal struct {
	UserID    string
	SessionID string
}

type contextKey struct{}

// With stores the principal in the context.
func With(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

// From returns the principal stored in the context.
func From(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}
