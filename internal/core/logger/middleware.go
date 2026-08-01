package logger

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	HeaderRequestID = "X-Request-ID"
	HeaderDeviceID  = "X-Device-ID"
)

// RequestIDMiddleware injects a unique request ID into every HTTP request
// context and returns it in the response header. If the client sends an
// X-Request-ID header, that value is used instead.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(string(requestIDKey), requestID)
		c.Header(HeaderRequestID, requestID)
		c.Next()
	}
}

// StructuredLogMiddleware logs every HTTP request with structured fields:
// method, path, status, latency, request ID, and optionally device ID.
// Errors (4xx/5xx) are logged at warn/error level.
func StructuredLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		requestID := c.GetString(string(requestIDKey))
		deviceID := c.GetHeader(HeaderDeviceID)

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("requestId", requestID),
		}
		if deviceID != "" {
			fields = append(fields, zap.String("deviceId", deviceID))
		}

		switch {
		case status >= 500:
			zap.L().Error("http request", fields...)
		case status >= 400:
			zap.L().Warn("http request", fields...)
		default:
			zap.L().Info("http request", fields...)
		}
	}
}

// InjectRequestIDIntoContext is a helper that copies the request ID from a gin
// context into a Go context, for passing to background goroutines.
func InjectRequestIDIntoContext(ctx context.Context, c *gin.Context) context.Context {
	if requestID := c.GetString(string(requestIDKey)); requestID != "" {
		return WithRequestID(ctx, requestID)
	}
	return ctx
}
