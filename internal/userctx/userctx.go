// Package userctx carries the authenticated user id through request contexts. In single-user mode
// (VIRTA_HOSTED=0) no id is ever set, and FromContext returns "". In hosted mode every
// authenticated request sets the id via the scoped() middleware; store repos use it to scope all
// reads and writes to the current user's rows.
package userctx

import "context"

type ctxKey struct{}

// WithUser returns a context that carries userID.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// FromContext returns the user id stored in ctx, or "" when none is set (single-user mode).
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
