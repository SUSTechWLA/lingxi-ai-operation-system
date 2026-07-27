package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

type correlationContextKey struct{}

func WithCorrelation(ctx context.Context, correlation Correlation) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, correlationContextKey{}, correlation)
}

func CorrelationFromContext(ctx context.Context) Correlation {
	if ctx == nil {
		return Correlation{}
	}
	correlation, _ := ctx.Value(correlationContextKey{}).(Correlation)
	return correlation
}

// WithOutboundTraceparent creates one W3C client child span for req. The
// returned request carries the child correlation in its context and header,
// while the caller's context remains unchanged for sibling outbound calls.
func WithOutboundTraceparent(req *http.Request) *http.Request {
	if req == nil {
		return nil
	}
	parentCtx := EnsureCorrelation(req.Context())
	parent := CorrelationFromContext(parentCtx)
	traceID := outboundTraceID(parent.TraceID)
	parentSpanID := outboundSpanID(parent.SpanID)
	child := parent
	child.TraceID = traceID
	child.ParentSpanID = parentSpanID
	child.SpanID = randomNonZeroHex(8)
	childCtx := WithCorrelation(parentCtx, child)
	outbound := req.Clone(childCtx)
	outbound.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, child.SpanID))
	return outbound
}

func outboundTraceID(value string) string {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "trc_")
	if w3cTraceIDPattern.MatchString(value) && !allZero(value) {
		return value
	}
	if value == "" {
		return randomNonZeroHex(16)
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:16])
}

func outboundSpanID(value string) string {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "spn_")
	if w3cSpanIDPattern.MatchString(value) && !allZero(value) {
		return value
	}
	if value == "" {
		return randomNonZeroHex(8)
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
