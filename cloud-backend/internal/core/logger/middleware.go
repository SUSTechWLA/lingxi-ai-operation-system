package logger

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

const (
	HeaderRequestID = "X-Request-ID"
	HeaderDeviceID  = "X-Device-ID"

	ctxKeyRequestID = "logger-request-id"
)

// RequestIDMiddleware injects a unique trace ID into every HTTP request
// context and returns it in the response header. If the client sends an
// X-Request-ID header, that value is used instead.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(ctxKeyRequestID, requestID)
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
		requestID := c.GetString(ctxKeyRequestID)
		deviceID := c.GetHeader(HeaderDeviceID)

		correlation := observability.Correlation{
			TraceID: requestID,
			SpanID:  uuid.NewString(),
		}
		log := WithCorrelation(zap.L(), correlation)

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
			log.Error("http request", fields...)
		case status >= 400:
			log.Warn("http request", fields...)
		default:
			log.Info("http request", fields...)
		}
	}
}

// RequestIDFromContext extracts the request ID stored by RequestIDMiddleware.
func RequestIDFromContext(c *gin.Context) string {
	return c.GetString(ctxKeyRequestID)
}
