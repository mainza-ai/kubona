package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/mainza-ai/kubona/core/proto"
)

// The bridge trusts Go's session mirror only when it carries this HMAC.
// Canonical form is byte-identical to bridge/server.py's helper
// (_SESSION_ASSERTION_FIELDS) and pinned by a cross-language vector.
func sessionAssertionPayload(sessionID string, ceiling, spent float64, tenant string, verified bool) string {
	return fmt.Sprintf("%s|%.6f|%.6f|%s|%t", sessionID, ceiling, spent, tenant, verified)
}

func sessionAssertionMAC(token, sessionID string, ceiling, spent float64, tenant string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(sessionAssertionPayload(sessionID, ceiling, spent, tenant, true)))
	return hex.EncodeToString(mac.Sum(nil))
}

// bridgeSessionAssertionInterceptor adds the HMAC to the two RPCs that
// mirror a Go session into Python. The transport token still authenticates
// the caller; this binds the money/tenant fields so the bridge never has to
// trust self-declared values.
func bridgeSessionAssertionInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var assertion string
		switch r := req.(type) {
		case *pb.QuantumSubmitRequest:
			if r.SessionVerified {
				assertion = sessionAssertionMAC(token, r.SessionId, r.SessionBudgetCeilingUsd, r.SessionSpentUsd, r.TenantId)
			}
		case *pb.WorkflowRunRequest:
			if r.SessionVerified {
				assertion = sessionAssertionMAC(token, r.SessionId, r.SessionBudgetCeilingUsd, r.SessionSpentUsd, r.TenantId)
			}
		}
		if assertion != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "x-kubona-session-assertion", assertion)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
