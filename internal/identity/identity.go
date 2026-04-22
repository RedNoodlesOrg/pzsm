// Package identity carries the authenticated user through request context.
package identity

import "context"

type ctxKey struct{}

// WithUser returns a copy of ctx carrying the given user email.
func WithUser(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, ctxKey{}, email)
}

// User returns the email attached to ctx, or empty string if none is set.
func User(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
