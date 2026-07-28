// Package trustedcontext carries server-authenticated identity across internal
// asynchronous boundaries. Values are deliberately not part of public event JSON.
package trustedcontext

import "context"

type userIDKey struct{}

func WithUserID(ctx context.Context, userID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userIDKey{}, userID)
}

func UserID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	value, ok := ctx.Value(userIDKey{}).(string)
	return value, ok && value != ""
}
