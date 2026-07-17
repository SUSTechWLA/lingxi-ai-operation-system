package auth

import "context"

type contextKey string

const (
	userIDContextKey   contextKey = "auth.user_id"
	deviceIDContextKey contextKey = "auth.device_id"
)

func ContextWithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

func ContextWithDevice(ctx context.Context, deviceID string) context.Context {
	return context.WithValue(ctx, deviceIDContextKey, deviceID)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok && userID != ""
}

func DeviceIDFromContext(ctx context.Context) (string, bool) {
	deviceID, ok := ctx.Value(deviceIDContextKey).(string)
	return deviceID, ok && deviceID != ""
}
