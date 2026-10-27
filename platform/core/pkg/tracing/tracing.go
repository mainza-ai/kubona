// Package tracing implements W3C trace-context propagation without an
// external collector. Every request gets a 128-bit trace id and a 64-bit
// span id; a valid inbound traceparent is continued, otherwise a new trace
// starts. Spans are emitted as structured slog records by the HTTP layer,
// and the traceparent response header lets callers correlate their logs.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey struct{}

// SpanContext is the subset of W3C trace context Kubona propagates.
type SpanContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Sampled      bool
}

// Traceparent renders the W3C traceparent header for this span.
func (s SpanContext) Traceparent() string {
	flags := "00"
	if s.Sampled {
		flags = "01"
	}
	return "00-" + s.TraceID + "-" + s.SpanID + "-" + flags
}

// FromContext returns the span stored by Start.
func FromContext(ctx context.Context) (SpanContext, bool) {
	s, ok := ctx.Value(contextKey{}).(SpanContext)
	return s, ok
}

// Start continues a valid inbound traceparent or starts a new trace. It
// never fails a request: malformed input is ignored and a fresh trace is
// minted.
func Start(ctx context.Context, r *http.Request) (context.Context, SpanContext) {
	parent, ok := parseTraceparent(r.Header.Get("traceparent"))
	span := SpanContext{SpanID: newID(8), Sampled: true}
	if ok {
		span.TraceID = parent.TraceID
		span.ParentSpanID = parent.SpanID
		span.Sampled = parent.Sampled
	} else {
		span.TraceID = newID(16)
	}
	return context.WithValue(ctx, contextKey{}, span), span
}

func newID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is exceptional; uuid still gives 128 bits.
		u := uuid.New()
		copy(b, u[:])
	}
	return hex.EncodeToString(b)
}

// parseTraceparent accepts version 00 (and future versions with the same
// first four fields). All-zero trace/span ids are invalid per W3C.
func parseTraceparent(h string) (SpanContext, bool) {
	parts := strings.Split(strings.TrimSpace(h), "-")
	if len(parts) < 4 || len(parts[0]) != 2 {
		return SpanContext{}, false
	}
	if parts[0] == "ff" || !isHex(parts[0], 2) || !isHex(parts[1], 32) || !isHex(parts[2], 16) || !isHex(parts[3], 2) {
		return SpanContext{}, false
	}
	if strings.Trim(parts[1], "0") == "" || strings.Trim(parts[2], "0") == "" {
		return SpanContext{}, false
	}
	var flags byte
	if b, err := hex.DecodeString(parts[3]); err != nil {
		return SpanContext{}, false
	} else {
		flags = b[0]
	}
	return SpanContext{
		TraceID: parts[1],
		SpanID:  parts[2],
		Sampled: flags&0x01 == 0x01,
	}, true
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
