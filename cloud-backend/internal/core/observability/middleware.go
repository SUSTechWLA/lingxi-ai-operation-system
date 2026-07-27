package observability

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var (
	w3cTraceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	w3cSpanIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
	w3cFlagsPattern   = regexp.MustCompile(`^[0-9a-f]{2}$`)
)

func Middleware(emitter *Emitter) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID, parentSpanID, flags, ok := parseTraceparent(c.GetHeader("traceparent"))
		if !ok {
			traceID = randomNonZeroHex(16)
			parentSpanID = ""
			flags = "00"
		}
		spanID := randomNonZeroHex(8)

		correlation := CorrelationFromContext(c.Request.Context())
		correlation.TraceID = traceID
		correlation.SpanID = spanID
		correlation.ParentSpanID = parentSpanID
		ctx := WithCorrelation(c.Request.Context(), correlation)
		c.Request = c.Request.WithContext(ctx)
		c.Header("traceparent", fmt.Sprintf("00-%s-%s-%s", traceID, spanID, flags))

		started := time.Now()
		defer func() {
			if recover() != nil {
				// Do not pass panic values to logs: they can contain request data.
				c.AbortWithStatus(http.StatusInternalServerError)
			}
			durationMs := time.Since(started).Milliseconds()

			status := ExecutionStatusCompleted
			severity := SeverityInfo
			if c.Writer.Status() >= http.StatusInternalServerError {
				status = ExecutionStatusFailed
				severity = SeverityError
			}
			if emitter != nil {
				err := emitter.Emit(ctx, Event{
					Severity:    severity,
					EventType:   EventTypeRequestAccepted,
					MessageKey:  "request.accepted",
					Correlation: correlation,
					Execution: Execution{
						Status:     status,
						Attempt:    1,
						DurationMs: &durationMs,
					},
					Privacy: Privacy{
						Classification: PrivacyInternal,
						RedactedFields: []string{},
					},
				})
				if err != nil {
					zap.L().Error("Failed to emit HTTP observability event", zap.Error(err))
				}
			}

			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			zap.L().Info("HTTP request completed",
				zap.String("method", c.Request.Method),
				zap.String("route", route),
				zap.Int("status", c.Writer.Status()),
				zap.Int64("durationMs", durationMs),
				zap.String("traceId", correlation.TraceID),
				zap.String("spanId", correlation.SpanID),
				zap.String("parentSpanId", correlation.ParentSpanID),
			)
		}()
		c.Next()
	}
}

func parseTraceparent(value string) (traceID, parentSpanID, flags string, ok bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), "-")
	if len(parts) != 4 || parts[0] != "00" {
		return "", "", "", false
	}
	if !w3cTraceIDPattern.MatchString(parts[1]) || allZero(parts[1]) {
		return "", "", "", false
	}
	if !w3cSpanIDPattern.MatchString(parts[2]) || allZero(parts[2]) {
		return "", "", "", false
	}
	if !w3cFlagsPattern.MatchString(parts[3]) {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

func randomNonZeroHex(bytes int) string {
	for {
		value := randomHex(bytes)
		if !allZero(value) {
			return value
		}
	}
}

func allZero(value string) bool {
	return strings.Trim(value, "0") == ""
}
