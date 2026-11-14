// Package requestid provides request correlation identifiers.
package requestid

import (
	"context"

	"github.com/google/uuid"
)

// Header is the response header that carries the request identifier.
const Header = "X-Request-ID"

type contextKey struct{}

// New returns a new UUIDv7 request identifier.
func New() string {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails when the system entropy source fails. A random
		// UUID still provides a usable correlation identifier.
		return uuid.NewString()
	}
	return id.String()
}

// With stores the request identifier in the context.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// From returns the request identifier stored in the context, or an empty
// string when none is present.
func From(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}
