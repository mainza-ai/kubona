package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestSessionAssertionVector(t *testing.T) {
	// Shared vector with platform/bridge/test_quantum_rpc.py.
	got := sessionAssertionMAC("k", "s1", 25.0, 0.25, "lab")
	want := "4e0b142452c9b1d33d7d916a409ef2434b24a97d1c6a9d4acdc842178530c7ea"
	if got != want {
		t.Fatalf("session assertion drifted from the Python vector: got %s want %s", got, want)
	}
}

func TestTraceBridgeInterceptorPropagatesTraceparent(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/quantum/jobs", nil)
	ctx, span := tracing.Start(context.Background(), req)
	var got string
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		if vals := md.Get("traceparent"); len(vals) > 0 {
			got = vals[0]
		}
		return nil
	}
	if err := traceBridgeInterceptor(ctx, "/QuantumBridge/QuantumSubmit", nil, nil, nil, invoker); err != nil {
		t.Fatal(err)
	}
	if got != span.Traceparent() {
		t.Fatalf("traceparent not propagated: got %q want %q", got, span.Traceparent())
	}
}
