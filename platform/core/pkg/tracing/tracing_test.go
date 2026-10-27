package tracing

import (
	"net/http/httptest"
	"testing"
)

func TestStartContinuesValidTraceparent(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/health", nil)
	r.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	ctx, span := Start(r.Context(), r)
	if span.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id not continued: %s", span.TraceID)
	}
	if span.ParentSpanID != "00f067aa0ba902b7" || span.SpanID == span.ParentSpanID {
		t.Fatalf("span must be new with parent set: %+v", span)
	}
	if !span.Sampled {
		t.Fatal("sampled flag not continued")
	}
	got, ok := FromContext(ctx)
	if !ok || got != span {
		t.Fatalf("span not stored in context: %+v %v", got, ok)
	}
	if got.Traceparent()[:3] != "00-" || len(got.Traceparent()) != 55 {
		t.Fatalf("bad traceparent rendering: %s", got.Traceparent())
	}
}

func TestStartRejectsMalformedAndAllZero(t *testing.T) {
	for _, h := range []string{
		"",
		"garbage",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-xyz-00f067aa0ba902b7-01",
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("traceparent", h)
		_, span := Start(r.Context(), r)
		if span.ParentSpanID != "" {
			t.Fatalf("malformed %q must not be continued: %+v", h, span)
		}
		if len(span.TraceID) != 32 || len(span.SpanID) != 16 {
			t.Fatalf("fresh ids must be 128/64-bit: %+v", span)
		}
	}
}

func TestSampledFlag(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	_, span := Start(r.Context(), r)
	if span.Sampled {
		t.Fatal("unsampled parent must stay unsampled")
	}
	if span.Traceparent()[53:] != "00" {
		t.Fatalf("flags must render 00: %s", span.Traceparent())
	}
}
