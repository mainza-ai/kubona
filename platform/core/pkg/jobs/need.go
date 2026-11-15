package jobs

// C2 — typed "need" on jobs. A need names a missing precondition and exactly
// one remedy from a CLOSED vocabulary in platform/contracts/need-kinds.json
// (the single source). The console renders one sentence and one button from
// it; it no longer infers "something is wrong" from free text.
//
// Classification is deliberately conservative. It uses evidence the runner
// actually owns — gRPC status codes and our own sentinels — never substring
// matching over vendor prose. A failure whose cause is not known from
// structured evidence emits NO need: the error text stands alone and the
// reviewer is not offered a remedy that may not help.

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	NeedCredentials    = "credentials"
	NeedBudgetExceeded = "budget_exceeded"
	NeedBackendOffline = "backend_offline"
	NeedQuotaExhausted = "quota_exhausted"
	NeedApproval       = "approval"
	NeedAnchorMissing  = "anchor_missing"
	NeedRetry          = "retry"
)

// needKinds is the closed set. Adding a kind means adding a row to
// contracts/need-kinds.json first — the conformance test asserts identity.
var needKinds = map[string]bool{
	NeedCredentials:    true,
	NeedBudgetExceeded: true,
	NeedBackendOffline: true,
	NeedQuotaExhausted: true,
	NeedApproval:       true,
	NeedAnchorMissing:  true,
	NeedRetry:          true,
}

// Need is the wire shape persisted beside a job's error and projected by the
// HTTP view. Past marks a need that has since been overcome: the card then
// says what ended short instead of offering a live remedy.
type Need struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
	Past   bool   `json:"past,omitempty"`
}

// ErrBridgeUnavailable marks a host-side refusal because no compute plane was
// configured. Wrapped with %w so classification never depends on the string.
var ErrBridgeUnavailable = errors.New("bridge unavailable")

// ClassifyNeed maps a runner-owned error to a need. nil means "no known
// remedy" — the honest answer for an unrecognised failure.
func ClassifyNeed(err error) *Need {
	if err == nil {
		return nil
	}
	reason := err.Error()
	if errors.Is(err, ErrBridgeUnavailable) {
		return &Need{Kind: NeedBackendOffline, Reason: reason}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Need{Kind: NeedRetry, Reason: reason}
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable:
			return &Need{Kind: NeedBackendOffline, Reason: reason}
		case codes.Unauthenticated, codes.PermissionDenied:
			return &Need{Kind: NeedCredentials, Reason: reason}
		case codes.ResourceExhausted:
			return &Need{Kind: NeedQuotaExhausted, Reason: reason}
		case codes.DeadlineExceeded, codes.Aborted, codes.Canceled, codes.Internal, codes.Unknown:
			// The cause is not specific enough to name a configuration fix,
			// but the run stopped before producing a result and the sentence
			// says the cause *may* be transient — it does not promise success.
			return &Need{Kind: NeedRetry, Reason: reason}
		default:
			// InvalidArgument / FailedPrecondition / NotFound / Unimplemented
			// / AlreadyExists / OutOfRange / DataLoss: deterministic refusals.
			// Retrying would not help, so no remedy is offered.
			return nil
		}
	}
	return nil
}

// Valid reports whether a need carries a catalogued kind and a reason. The
// HTTP view refuses (omits) anything else rather than shipping an unknown
// vocabulary entry to the console.
func (n *Need) Valid() bool {
	return n != nil && needKinds[n.Kind] && n.Reason != ""
}

// needError classifies and stamps the backend target from the job's params.
// A need without a target still renders; the target only sharpens the remedy.
func needForJob(j *Job, err error) *Need {
	need := ClassifyNeed(err)
	if need == nil || j == nil {
		return need
	}
	if need.Target == "" {
		need.Target = backendTarget(j.ParamsJSON)
	}
	if !need.Valid() {
		return nil
	}
	return need
}

// backendTarget reads the backend name from admitted params without inventing
// one. Empty is the honest answer when the caller did not name a backend.
func backendTarget(params []byte) string {
	if len(params) == 0 {
		return ""
	}
	var raw struct {
		RequestedBackend string `json:"requested_backend"`
		Backend          string `json:"backend"`
		ResolvedBackend  string `json:"resolved_backend"`
	}
	if err := json.Unmarshal(params, &raw); err != nil {
		return ""
	}
	for _, candidate := range []string{raw.RequestedBackend, raw.Backend, raw.ResolvedBackend} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
