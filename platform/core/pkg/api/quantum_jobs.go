package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// Human quantum dispatch proxy (R-34) + provider cancel (R-33) + refund
// history read (R-32). Thin transport over the bridge RPCs, which call
// the SAME submit_job the agents use — no parallel governance exists
// here. Every handler: bearer auth, tenant autonomy (suspended/L0
// refuse), provider allowlist validation, Sovereign Air-Gap gate,
// cross-tenant session isolation, then proxy. Go's SQLite ledger is the
// single book of record for session + tenant spend (F-NEW-01): Go mirrors
// session ceiling + current spend into Python's preflight via
// QuantumSubmitRequest, and atomically reserves estimated_usd in SQLite on
// non-replay non-dry-run admissions so POST /api/sessions/refund,
// DeleteBlockers, and console spend bars operate on one unified ledger.

// quantumJobTimeout bounds the proxy call. Submit/status/cancel return
// fast (async dispatch, store read, fence); the bridge never blocks on
// hardware. Canary gets its own longer budget below.
const quantumJobTimeout = 60 * time.Second

// defaultHumanJurisdictions returns the jurisdiction set applied when an
// operator console dispatch omits caller_jurisdictions. Overridable via
// KUBONA_DEFAULT_JURISDICTIONS (comma-separated ISO/region codes); an
// explicit empty array in the JSON request ([]string{}) is preserved as
// empty so jurisdiction-mismatch refusals remain testable and enforceable.
func defaultHumanJurisdictions() []string {
	if raw := strings.TrimSpace(os.Getenv("KUBONA_DEFAULT_JURISDICTIONS")); raw != "" {
		var out []string
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{"US", "UK", "EU", "CA", "DE"}
}

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
// tenant_id is DERIVED from the session or authenticated caller, never
// trusted from the request body.
func (s *Server) handleQuantumJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to submit a quantum job", nil)
		return
	}
	var req struct {
		SessionID           string    `json:"session_id"`
		QASM                string    `json:"qasm"`
		Provider            string    `json:"provider"`
		Shots               int       `json:"shots"`
		IdempotencyKey      string    `json:"idempotency_key"`
		DryRun              bool      `json:"dry_run"`
		MandateRef          string    `json:"mandate_ref"`
		CallerJurisdictions *[]string `json:"caller_jurisdictions"`
		CallerExportClass   string    `json:"caller_export_class"`
		RequireReview       bool      `json:"require_review"`
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
	// Sovereign Air-Gap Gate: zero outbound cloud QPU egress when active.
	if s.llmSettings != nil && s.llmSettings.GetSnapshot().AirGapped &&
		provider != "local-simulator" && provider != "qiskit-aer-simulator" {
		WriteAPIError(w, "forbidden", "Sovereign Air-Gap active: external cloud QPU dispatch refused", nil)
		return
	}
	shots := req.Shots
	if shots <= 0 {
		shots = 1024
	}
	// Session attribution + autonomy + cross-tenant isolation:
	// - Known sessions resolve their tenant, enforce caller-vs-session
	//   tenant binding (T-09c), and enforce tenant autonomy.
	// - Unknown explicit sessions refuse (ghost_session).
	// - Empty session_id means implicit (W0.16 $10 cap), but STILL enforces
	//   the caller tenant's autonomy (suspended/L0 tenants cannot bypass
	//   autonomy by omitting session_id — F-NEW-05).
	tenantID := ""
	sessionVerified := false
	sessionCeilingUSD := 0.0
	sessionSpentUSD := 0.0
	if req.SessionID != "" {
		sess, err := s.sessionMgr.GetSession(req.SessionID)
		if err != nil || sess == nil {
			WriteAPIError(w, "ghost_session", "unknown session_id: dispatch refused", nil)
			return
		}
		if !s.authorizeSessionRead(w, r, sess) {
			return
		}
		tenantID = sess.TenantID
		if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
			WriteAPIError(w, "forbidden", err.Error(), nil)
			return
		}
		sessionVerified = true
		sessionCeilingUSD = sess.SpendLimitUSD
		if sessionCeilingUSD <= 0 {
			sessionCeilingUSD = session.DefaultBudgetUSD
		}
		sessionSpentUSD = sess.CurrentSpendUSD
		if t, terr := s.tenantMgr.GetTenant(tenantID); terr == nil && t != nil && t.SpendCeilingUSD > 0 {
			tenantRem := t.SpendCeilingUSD - t.SpentUSD
			if tenantRem < 0 {
				tenantRem = 0
			}
			if sessionSpentUSD+tenantRem < sessionCeilingUSD {
				sessionCeilingUSD = sessionSpentUSD + tenantRem
			}
		}
	} else {
		callerTenant, _, _ := s.checkAuth(r)
		if callerTenant == "" {
			callerTenant = tenants.DefaultTenantID
		}
		tenantID = callerTenant
		if err := s.requireTenantDispatchAutonomy(tenantID); err != nil {
			WriteAPIError(w, "forbidden", err.Error(), nil)
			return
		}
	}

	var jurisdictions []string
	if req.CallerJurisdictions != nil {
		jurisdictions = *req.CallerJurisdictions
	} else {
		jurisdictions = defaultHumanJurisdictions()
	}
	exportClass := strings.TrimSpace(req.CallerExportClass)
	if exportClass == "" && req.CallerJurisdictions == nil {
		exportClass = "allied"
	}

	bridge := s.quantumBridge(w, "quantum_submit")
	if bridge == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), quantumJobTimeout)
	defer cancel()
	resp, err := bridge.QuantumSubmit(ctx, &pb.QuantumSubmitRequest{
		Qasm:                    req.QASM,
		Provider:                provider,
		Shots:                   int32(shots),
		SessionId:               req.SessionID,
		TenantId:                tenantID,
		IdempotencyKey:          req.IdempotencyKey,
		DryRun:                  req.DryRun,
		MandateRefJson:          req.MandateRef,
		CallerJurisdictions:     jurisdictions,
		CallerExportClass:       exportClass,
		RequireReview:           req.RequireReview,
		SessionVerified:         sessionVerified,
		SessionBudgetCeilingUsd: sessionCeilingUSD,
		SessionSpentUsd:         sessionSpentUSD,
	})
	if err != nil {
		writeBridgeUnavailable(w, "quantum_submit", "QuantumSubmit RPC failed: "+err.Error())
		return
	}
	if !resp.Ok {
		WriteAPIError(w, "bridge_unavailable", resp.ErrorMessage, nil)
		return
	}

	// Single book of record (F-NEW-01 / F-NEW-05): when a real (non-dry-run,
	// non-replay) job is admitted, ensure the target session exists in SQLite,
	// reserve positive estimated_usd against tenant + session ceilings, and
	// record the job row in SQLite jobs so DeleteBlockers, ListJobs,
	// POST /api/sessions/refund, and console spend bars operate on one unified
	// ledger.
	if !req.DryRun && !resp.IdempotentReplay {
		var est struct {
			EstimatedUSD float64 `json:"estimated_usd"`
		}
		_ = json.Unmarshal([]byte(resp.EstimateJson), &est)
		targetSessionID := resp.SessionId
		if targetSessionID == "" {
			targetSessionID = req.SessionID
		}
		if req.SessionID == "" && targetSessionID != "" {
			_, _ = s.sessionMgr.EnsureSessionWithID(targetSessionID, "Implicit Quantum Session", 10.0, tenantID)
		}
		if est.EstimatedUSD > 0 {
			if err := s.tenantMgr.ReserveSpend(tenantID, est.EstimatedUSD); err != nil {
				_, _ = bridge.QuantumCancel(ctx, &pb.QuantumCancelRequest{JobId: resp.JobId, Reason: "tenant ceiling exceeded"})
				WriteAPIError(w, "ceiling_exceeded", err.Error(), nil)
				return
			}
			if targetSessionID != "" {
				if err := s.sessionMgr.ReserveSpend(targetSessionID, est.EstimatedUSD); err != nil {
					_ = s.tenantMgr.ReleaseSpend(tenantID, est.EstimatedUSD)
					_, _ = bridge.QuantumCancel(ctx, &pb.QuantumCancelRequest{JobId: resp.JobId, Reason: "session ceiling exceeded"})
					WriteAPIError(w, "ceiling_exceeded", err.Error(), nil)
					return
				}
			}
		}
		if targetSessionID != "" && resp.JobId != "" {
			_ = s.jobRunner.RecordQuantumBridgeJob(
				resp.JobId, targetSessionID, provider, req.QASM, shots,
				tenantID, est.EstimatedUSD, req.MandateRef,
			)
		}
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
	if resp.SessionId != "" {
		if sess, serr := s.sessionMgr.GetSession(resp.SessionId); serr == nil && sess != nil {
			if !s.authorizeSessionRead(w, r, sess) {
				return
			}
		}
	}
	s.jobRunner.SyncQuantumBridgeJobStatus(resp.JobId, resp.Status, firstNonEmpty(resp.ResultJson, "{}"))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "job_id": resp.JobId, "status": resp.Status,
		"session_id": resp.SessionId,
		"result":     json.RawMessage(firstNonEmpty(resp.ResultJson, "{}")),
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
	if stResp, stErr := bridge.QuantumJobStatus(ctx, &pb.QuantumJobStatusRequest{JobId: jobID}); stErr == nil && stResp != nil && stResp.Ok && stResp.SessionId != "" {
		if sess, serr := s.sessionMgr.GetSession(stResp.SessionId); serr == nil && sess != nil {
			if !s.authorizeSessionRead(w, r, sess) {
				return
			}
			if err := s.requireDispatchAutonomy(stResp.SessionId); err != nil {
				WriteAPIError(w, "forbidden", err.Error(), nil)
				return
			}
		}
	}
	resp, err := bridge.QuantumCancel(ctx, &pb.QuantumCancelRequest{JobId: jobID, Reason: req.Reason})
	if err != nil {
		writeBridgeUnavailable(w, "quantum_cancel", "QuantumCancel RPC failed: "+err.Error())
		return
	}
	if !resp.Ok {
		WriteAPIError(w, "bridge_unavailable", resp.ErrorMessage, nil)
		return
	}
	s.jobRunner.SyncQuantumBridgeJobStatus(resp.JobId, "cancelled", `{"cancelled":true}`)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "job_id": resp.JobId,
		"provider_recall_attempted": resp.ProviderRecallAttempted,
		"provider_recall_status":    resp.ProviderRecallStatus,
	})
}
