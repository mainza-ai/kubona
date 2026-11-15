package api

// C11 — run-scoped allowance mint / read / revoke. The operator signs a
// canonical scope with the reviewer key (tenant admin key also accepted), the
// server verifies it, binds it to one metered backend and the live
// rate-card/calibration basis, and stores it. Admission is the enforcement
// point; this file is authority + audit only.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/money"
	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// allowanceScope is the exact bytes the operator signs. Stable field order;
// integer micro-dollars avoid float formatting drift across languages.
type allowanceScope struct {
	TenantID          string   `json:"tenant_id"`
	SessionID         string   `json:"session_id"`
	CampaignID        string   `json:"campaign_id"`
	Backend           string   `json:"backend"`
	JobTypes          []string `json:"job_types"`
	MaxPerJobMicroUSD int64    `json:"max_per_job_micro_usd"`
	MaxTotalMicroUSD  int64    `json:"max_total_micro_usd"`
	MaxJobs           int      `json:"max_jobs"`
	ShotsCap          int      `json:"shots_cap"`
	ExpiresAt         string   `json:"expires_at"`
	Nonce             string   `json:"nonce"`
}

func (s allowanceScope) canonical() ([]byte, error) {
	return json.Marshal(s)
}

type allowanceView struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	SessionID       string    `json:"session_id"`
	CampaignID      string    `json:"campaign_id,omitempty"`
	Backend         string    `json:"backend"`
	JobTypes        []string  `json:"job_types"`
	MaxPerJobUSD    float64   `json:"max_per_job_usd"`
	MaxTotalUSD     float64   `json:"max_total_usd"`
	RemainingUSD    float64   `json:"remaining_usd"`
	MaxJobs         int       `json:"max_jobs"`
	JobsUsed        int       `json:"jobs_used"`
	ShotsCap        int       `json:"shots_cap"`
	Status          string    `json:"status"`
	SuspendedReason string    `json:"suspended_reason,omitempty"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	SignerPubkeyHex string    `json:"signer_pubkey_hex"`
	ScopeDigest     string    `json:"scope_digest"`
}

func allowanceViewFrom(a *ledger.RunAllowance) allowanceView {
	backend := ""
	if len(a.Backends) > 0 {
		backend = a.Backends[0]
	}
	return allowanceView{
		ID: a.ID, TenantID: a.TenantID, SessionID: a.SessionID, CampaignID: a.CampaignID,
		Backend: backend, JobTypes: a.JobTypes,
		MaxPerJobUSD: float64(a.MaxPerJobMicroUSD) / 1e6,
		MaxTotalUSD:  float64(a.MaxTotalMicroUSD) / 1e6,
		RemainingUSD: float64(a.RemainingMicroUSD) / 1e6,
		MaxJobs:      a.MaxJobs, JobsUsed: a.JobsUsed, ShotsCap: a.ShotsCap,
		Status: a.Status, SuspendedReason: a.SuspendedReason,
		IssuedAt: a.IssuedAt, ExpiresAt: a.ExpiresAt,
		SignerPubkeyHex: a.SignerPubkeyHex, ScopeDigest: a.ScopeDigest,
	}
}

func (s *Server) allowanceSignerAllowed(tenantID, pubkeyHex string) bool {
	if pubkeyHex == "" {
		return false
	}
	if tenant, err := s.tenantMgr.GetTenant(tenantID); err == nil && tenant != nil &&
		tenant.AdminPubkeyHex != "" && strings.EqualFold(tenant.AdminPubkeyHex, pubkeyHex) {
		return true
	}
	if s.reviewerKeyConfigured || ephemeralAllowed() {
		return strings.EqualFold(hex.EncodeToString(s.signerPubKey), pubkeyHex)
	}
	return false
}

// allowanceCalibrationBasis binds the allowance to the device profile the
// plan was priced against (same identity the proposal rail uses).
func (s *Server) allowanceCalibrationBasis(backend string) (string, error) {
	if s.fidelityGate == nil {
		return "", fmt.Errorf("fidelity gate unavailable")
	}
	device, err := s.deviceForBackend(backend)
	if err != nil {
		return "", err
	}
	profile, ok := s.fidelityGate.Profile(device)
	if !ok {
		return "", fmt.Errorf("no calibration profile for %q", backend)
	}
	digest := fmt.Sprintf("%s|%s|%s|%s", profile.Name, profile.Source, profile.Version, profile.CalibratedAt)
	sum := sha256Bytes([]byte(digest))
	return "sha256:" + hex.EncodeToString(sum), nil
}

func (s *Server) handleRunAllowances(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			WriteAPIError(w, "bad_request", "session_id is required", nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		rows, err := s.ledger.ListRunAllowances(sessionID)
		if err != nil {
			WriteAPIError(w, "internal", "allowance list failed", nil)
			return
		}
		views := make([]allowanceView, 0, len(rows))
		for i := range rows {
			views = append(views, allowanceViewFrom(&rows[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "allowances": views, "count": len(views)})
	case http.MethodPost:
		var req struct {
			SessionID       string   `json:"session_id"`
			CampaignID      string   `json:"campaign_id"`
			Backend         string   `json:"backend"`
			JobTypes        []string `json:"job_types"`
			MaxPerJobUSD    float64  `json:"max_per_job_usd"`
			MaxTotalUSD     float64  `json:"max_total_usd"`
			MaxJobs         int      `json:"max_jobs"`
			ShotsCap        int      `json:"shots_cap"`
			ExpiresAt       string   `json:"expires_at"`
			Nonce           string   `json:"nonce"`
			SignerPubkeyHex string   `json:"signer_pubkey_hex"`
			SignatureHex    string   `json:"signature_hex"`
		}
		if err := decodeStrict(r, &req); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid allowance request: %v", err), nil)
			return
		}
		sid, serr := s.requireSession(req.SessionID)
		if serr != nil {
			WriteAPIError(w, "not_found", serr.Error(), nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
			return
		}
		sess, gerr := s.sessionMgr.GetSession(sid)
		if gerr != nil || sess == nil {
			WriteAPIError(w, "not_found", "session not found", nil)
			return
		}
		tenantID := sess.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		backend, berr := jobs.NormalizeBackend(req.Backend)
		if berr != nil {
			WriteAPIError(w, "bad_request", berr.Error(), nil)
			return
		}
		if jobs.IsSimulatorBackend(backend) {
			WriteAPIError(w, "bad_request", "run allowances cover metered hardware only; simulator runs need no allowance", nil)
			return
		}
		if len(req.JobTypes) == 0 {
			req.JobTypes = []string{"circuit_run"}
		}
		for _, jobType := range req.JobTypes {
			if jobType != "circuit_run" {
				WriteAPIError(w, "bad_request", fmt.Sprintf("run allowance v1 covers circuit_run only; %q is not supported", jobType), nil)
				return
			}
		}
		perJob, merr := money.FromUSD(req.MaxPerJobUSD)
		if merr != nil || req.MaxPerJobUSD <= 0 {
			WriteAPIError(w, "bad_request", "max_per_job_usd must be positive", nil)
			return
		}
		total, merr := money.FromUSD(req.MaxTotalUSD)
		if merr != nil || req.MaxTotalUSD <= 0 || perJob > total {
			WriteAPIError(w, "bad_request", "max_total_usd must be positive and at least max_per_job_usd", nil)
			return
		}
		if req.MaxJobs < 1 || req.MaxJobs > 100 {
			WriteAPIError(w, "bad_request", "max_jobs must be 1..100", nil)
			return
		}
		if req.ShotsCap < 1 {
			WriteAPIError(w, "bad_request", "shots_cap must be positive", nil)
			return
		}
		expires, terr := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
		if terr != nil {
			WriteAPIError(w, "bad_request", "expires_at must be RFC3339", nil)
			return
		}
		now := time.Now().UTC()
		if !expires.After(now.Add(1*time.Minute)) || expires.After(now.Add(24*time.Hour)) {
			WriteAPIError(w, "bad_request", "expires_at must be between one minute and 24 hours from now", nil)
			return
		}
		nonce := strings.TrimSpace(req.Nonce)
		if len(nonce) < 8 || len(nonce) > 128 {
			WriteAPIError(w, "bad_request", "nonce must be 8..128 characters", nil)
			return
		}
		if !s.allowanceSignerAllowed(tenantID, req.SignerPubkeyHex) {
			WriteAPIError(w, "forbidden", "allowance signer is not the tenant admin or a commissioned reviewer key", nil)
			return
		}
		scope := allowanceScope{
			TenantID: tenantID, SessionID: sid, CampaignID: strings.TrimSpace(req.CampaignID),
			Backend: backend, JobTypes: req.JobTypes,
			MaxPerJobMicroUSD: int64(perJob), MaxTotalMicroUSD: int64(total),
			MaxJobs: req.MaxJobs, ShotsCap: req.ShotsCap,
			ExpiresAt: expires.UTC().Format(time.RFC3339), Nonce: nonce,
		}
		canonical, cerr := scope.canonical()
		if cerr != nil {
			WriteAPIError(w, "internal", "allowance scope canonicalization failed", nil)
			return
		}
		pub, perr := hex.DecodeString(strings.TrimSpace(req.SignerPubkeyHex))
		if perr != nil || len(pub) != ed25519.PublicKeySize {
			WriteAPIError(w, "bad_request", "signer_pubkey_hex must be a 32-byte Ed25519 key", nil)
			return
		}
		sig, serr2 := hex.DecodeString(strings.TrimSpace(req.SignatureHex))
		if serr2 != nil || len(sig) != ed25519.SignatureSize {
			WriteAPIError(w, "bad_request", "signature_hex must be a 64-byte Ed25519 signature", nil)
			return
		}
		if !ed25519.Verify(ed25519.PublicKey(pub), canonical, sig) {
			WriteAPIError(w, "forbidden", "allowance signature does not verify the canonical scope", nil)
			return
		}
		calBasis, cberr := s.allowanceCalibrationBasis(backend)
		if cberr != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("cannot bind calibration basis: %v", cberr), nil)
			return
		}
		a, aerr := s.ledger.MintRunAllowance(ledger.RunAllowance{
			ID: uuid.NewString(), TenantID: tenantID, SessionID: sid, CampaignID: scope.CampaignID,
			ScopeDigest: "sha256:" + hex.EncodeToString(sha256Bytes(canonical)),
			Backends:    []string{backend}, JobTypes: req.JobTypes,
			MaxPerJobMicroUSD: int64(perJob), MaxTotalMicroUSD: int64(total),
			MaxJobs: req.MaxJobs, ShotsCap: req.ShotsCap,
			ExpiresAt:              expires.UTC(),
			BasisRateCard:          rates.CardSnapshot().EffectiveDate,
			BasisCalibrationDigest: calBasis,
			Nonce:                  nonce, SignerPubkeyHex: strings.ToLower(req.SignerPubkeyHex),
			SignatureHex: strings.ToLower(req.SignatureHex),
		})
		if aerr != nil {
			WriteAPIError(w, "bad_request", aerr.Error(), nil)
			return
		}
		_ = s.ledger.AppendEvent(sid, "run_allowance_minted", map[string]any{
			"allowance_id": a.ID, "backend": backend, "max_total_usd": req.MaxTotalUSD,
			"max_jobs": req.MaxJobs, "expires_at": scope.ExpiresAt, "signer": a.SignerPubkeyHex,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "allowance": allowanceViewFrom(a)})
	default:
		WriteAPIError(w, "method_not_allowed", "GET or POST required for run allowances", nil)
	}
}

func (s *Server) handleRunAllowanceByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/run-allowances/")
	id, action, _ := strings.Cut(rest, "/")
	if id == "" || action != "revoke" || r.Method != http.MethodPost {
		WriteAPIError(w, "bad_request", "allowance path is /api/run-allowances/{id}/revoke (POST)", nil)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Reason    string `json:"reason"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid revoke request: %v", err), nil)
		return
	}
	sid, serr := s.requireSession(req.SessionID)
	if serr != nil {
		WriteAPIError(w, "not_found", serr.Error(), nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
		return
	}
	a, err := s.ledger.GetRunAllowance(id)
	if err != nil {
		WriteAPIError(w, "internal", "allowance read failed", nil)
		return
	}
	if a == nil || a.SessionID != sid {
		WriteAPIError(w, "not_found", "unknown allowance for this session", nil)
		return
	}
	reason := boundRunes(strings.TrimSpace(req.Reason), 200)
	if err := s.ledger.SuspendRunAllowance(id, reason, ledger.RunAllowanceRevoked); err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	_ = s.ledger.AppendEvent(sid, "run_allowance_revoked", map[string]any{
		"allowance_id": id, "reason": reason,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": ledger.RunAllowanceRevoked})
}

// suspendActiveAllowances is the fail-closed hook for air-gap engagement.
func (s *Server) suspendActiveAllowances(reason string) {
	active, err := s.ledger.ListActiveRunAllowances()
	if err != nil {
		return
	}
	for i := range active {
		if err := s.ledger.SuspendRunAllowance(active[i].ID, reason, ledger.RunAllowanceSuspended); err == nil {
			_ = s.ledger.AppendEvent(active[i].SessionID, "run_allowance_suspended", map[string]any{
				"allowance_id": active[i].ID, "reason": reason,
			})
		}
	}
}

func sha256Bytes(raw []byte) []byte {
	sum := sha256.Sum256(raw)
	return sum[:]
}

// deviceForBackend resolves the calibrated device profile name for a backend.
func (s *Server) deviceForBackend(backend string) (string, error) {
	return fidelity.ResolveDeviceForBackend(backend)
}
