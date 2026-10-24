package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

// Human quantum dispatch proxy (R-34) + provider cancel (R-33) + refund
// history read (R-32). Thin transport over the bridge RPCs, which call
// the SAME submit_job the agents use — no parallel governance exists
// here. Every handler: bearer auth, tenant autonomy (suspended/L0
// refuse), provider allowlist validation, then proxy. Spend authority
// lives in the Python rail (preflight + record_spend); Go never reserves
// QPU spend (that would double-count the single book of record).

// quantumJobTimeout bounds the proxy call. Submit/status/cancel return
// fast (async dispatch, store read, fence); the bridge never blocks on
// hardware. Canary gets its own longer budget below.
const quantumJobTimeout = 60 * time.Second

func (s *Server) quantumBridge(w http.ResponseWriter, computation string) pb.QuantumBridgeServiceClient {
	bridge := s.jobRunner.BridgeClient()
	if bridge == nil {
		writeBridgeUnavailable(w, computation, "bridge client is nil.")
		return nil
	}
	return bridge
}

// POST /api/quantum/jobs — human dispatch. Body: session_id (optional;
// empty = implicit capped session, same W0.16 rule as agents), qasm,
// provider, shots, idempotency_key (REQUIRED — no key, no dispatch),
// dry_run (estimate-only: full validation, no spend, no enqueue),
// tenant_id is DERIVED from the session, never trusted from the caller.
func (s *Server) handleQuantumJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to submit a quantum job", nil)
		return
	}
	var req struct {
		SessionID      string `json:"session_id"`
		QASM           string `json:"qasm"`
		Provider       string `json:"provider"`
		Shots          int    `json:"shots"`
		IdempotencyKey string `json:"idempotency_key"`
		DryRun         bool   `json:"dry_run"`
		// Reserved: recorded, not enforced — the Q-10 ruling decides
		// whether human QPU dispatch requires a per-job mandate.
		MandateRef string `json:"mandate_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}
	if req.IdempotencyKey == "" {
		WriteAPIError(w, "bad_request", "idempotency_key is required: retries without a key risk double hardware spend", nil)
		return
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = s.quantumSettings.DefaultBackend
	}
	if !knownQuantumProvider(provider) {
		WriteAPIError(w, "bad_request", "unknown provider "+provider+": pick one from /api/quantum/backends", nil)
		return
	}
	shots := req.Shots
	if shots <= 0 {
		shots = 1024
	}
	// Session attribution + autonomy: known sessions resolve their
	// tenant; unknown explicit sessions refuse (ghost); empty means
	// implicit (the bridge applies the W0.16 implicit-session cap).
	tenantID := ""
	if req.SessionID != "" {
		sess, err := s.sessionMgr.GetSession(req.SessionID)
		if err != nil || sess == nil {
			WriteAPIError(w, "ghost_session", "unknown session_id: dispatch refused", nil)
			return
		}
		tenantID = sess.TenantID
		if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
			WriteAPIError(w, "forbidden", err.Error(), nil)
			return
		}
	}
	bridge := s.quantumBridge(w, "quantum_submit")
	if bridge == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), quantumJobTimeout)
	defer cancel()
	resp, err := bridge.QuantumSubmit(ctx, &pb.QuantumSubmitRequest{
		Qasm:           req.QASM,
		Provider:       provider,
		Shots:          int32(shots),
		SessionId:      req.SessionID,
		TenantId:       tenantID,
		IdempotencyKey: req.IdempotencyKey,
		DryRun:         req.DryRun,
		MandateRefJson: req.MandateRef,
	})
	if err != nil {
		writeBridgeUnavailable(w, "quantum_submit", "QuantumSubmit RPC failed: "+err.Error())
		return
	}
	if !resp.Ok {
		WriteAPIError(w, "bridge_unavailable", resp.ErrorMessage, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "job_id": resp.JobId, "queued": resp.Queued,
		"session_id": resp.SessionId, "dry_run": req.DryRun,
		"estimate": json.RawMessage(firstNonEmpty(resp.EstimateJson, "{}")),
		"fidelity": json.RawMessage(firstNonEmpty(resp.FidelityJson, "{}")),
	})
}

// GET /api/quantum/jobs + POST /api/quantum/jobs/:id/cancel share the
// subpath router. Reads are session-scoped (same rule as ledger export);
// cancel is bearer + autonomy gated like dispatch.
func (s *Server) handleQuantumJobActions(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/quantum/jobs/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		WriteAPIError(w, "bad_request", "job_id is required", nil)
		return
	}
	jobID, verb := parts[0], ""
	if len(parts) > 1 {
		verb = parts[1]
	}
	switch {
	case r.Method == http.MethodGet && verb == "":
		s.handleQuantumJobStatus(w, r, jobID)
	case r.Method == http.MethodPost && verb == "cancel":
		s.handleQuantumJobCancel(w, r, jobID)
	default:
		WriteAPIError(w, "method_not_allowed", "GET job status, POST job cancel", nil)
	}
}

func (s *Server) handleQuantumJobStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	bridge := s.quantumBridge(w, "quantum_job_status")
	if bridge == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), quantumJobTimeout)
	defer cancel()
	resp, err := bridge.QuantumJobStatus(ctx, &pb.QuantumJobStatusRequest{JobId: jobID})
	if err != nil {
		writeBridgeUnavailable(w, "quantum_job_status", "QuantumJobStatus RPC failed: "+err.Error())
		return
	}
	if !resp.Ok {
		WriteAPIError(w, "not_found", resp.ErrorMessage, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "job_id": resp.JobId, "status": resp.Status,
		"result": json.RawMessage(firstNonEmpty(resp.ResultJson, "{}")),
	})
}

func (s *Server) handleQuantumJobCancel(w http.ResponseWriter, r *http.Request, jobID string) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // reason optional; empty allowed
	bridge := s.quantumBridge(w, "quantum_cancel")
	if bridge == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), quantumJobTimeout)
	defer cancel()
	resp, err := bridge.QuantumCancel(ctx, &pb.QuantumCancelRequest{JobId: jobID, Reason: req.Reason})
	if err != nil {
		writeBridgeUnavailable(w, "quantum_cancel", "QuantumCancel RPC failed: "+err.Error())
		return
	}
	if !resp.Ok {
		WriteAPIError(w, "bridge_unavailable", resp.ErrorMessage, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "job_id": resp.JobId,
		"provider_recall_attempted": resp.ProviderRecallAttempted,
		"provider_recall_status":    resp.ProviderRecallStatus,
	})
}
