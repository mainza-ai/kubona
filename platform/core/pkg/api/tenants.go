package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// Tenant self-provisioning is open by design (self-hosted labs): anyone may
// register a lab tenant, which starts at L1 with the default ceiling and an
// empty admin key. The first SetAdminKey wins (ledger-sealed bootstrap);
// every mutation after that requires the admin's signature.
func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Labs view: every registered lab with ceiling, spent, autonomy.
		all, err := s.tenantMgr.ListTenants()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenants": all})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// R4: open provisioning is not unlimited provisioning.
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	t, err := s.tenantMgr.CreateTenant(req.ID, req.DisplayName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleTenantByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tenants/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "missing tenant id", http.StatusBadRequest)
		return
	}
	t, err := s.tenantMgr.GetTenant(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// bindAdminAction verifies that an admin envelope actually authorizes the
// stated mutation: the embedded params hash must match the server-computed
// hash of the mutation parameters, binding signature to intent.
func bindAdminAction(tenantID, actionName string, params any, raw tenants.AdminAction) (*tenants.AdminAction, error) {
	if raw.TenantID != tenantID || raw.Action != actionName {
		return nil, httpError("admin envelope tenant/action mismatch")
	}
	want, err := tenants.ParamsHashOf(params)
	if err != nil {
		return nil, err
	}
	if raw.ParamsHash != want {
		return nil, httpError("admin envelope params mismatch: signature does not bind these parameters")
	}
	return &raw, nil
}

// sessionAdminAction resolves the httpOnly lab-admin session into an
// in-process admin action. The cookie already proved possession of the
// tenant admin key at login; the fingerprint must still match the current
// registered key (rotation invalidates old sessions' authority).
func (s *Server) sessionAdminAction(r *http.Request, tenantID string) (*tenants.AdminAction, bool) {
	if s.authSessions == nil {
		return nil, false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	tid, fp, ok := s.authSessions.validateToken(c.Value)
	if !ok || tid != tenantID {
		return nil, false
	}
	t, err := s.tenantMgr.GetTenant(tenantID)
	if err != nil || t.AdminPubkeyHex == "" {
		return nil, false
	}
	if !strings.EqualFold(fp, tenants.Fingerprint(t.AdminPubkeyHex)) {
		return nil, false
	}
	return &tenants.AdminAction{
		TenantID: tenantID, PubkeyHex: t.AdminPubkeyHex,
		Expiry: time.Now().UTC().Add(5 * time.Minute), ViaSession: true,
	}, true
}

// bindAdminActionOrSession accepts either a signed envelope (machine/bearer
// callers) or a validated lab-admin session cookie (console after reload).
// It never falls back to the session when a signature was supplied.
func (s *Server) bindAdminActionOrSession(r *http.Request, tenantID, actionName string,
	params any, raw tenants.AdminAction) (*tenants.AdminAction, error) {
	if raw.SignatureHex != "" || raw.PubkeyHex != "" {
		return bindAdminAction(tenantID, actionName, params, raw)
	}
	act, ok := s.sessionAdminAction(r, tenantID)
	if !ok {
		return nil, httpError("admin signature or lab-admin session required")
	}
	want, err := tenants.ParamsHashOf(params)
	if err != nil {
		return nil, err
	}
	act.Action = actionName
	act.ParamsHash = want
	return act, nil
}

type httpErr string

func (e httpErr) Error() string { return string(e) }

func httpError(msg string) error { return httpErr(msg) }

func (s *Server) handleTenantAdminKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		TenantID       string               `json:"tenant_id"`
		NewAdminPubkey string               `json:"new_admin_pubkey_hex"`
		Action         *tenants.AdminAction `json:"action,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var action *tenants.AdminAction
	if req.Action != nil {
		bound, err := bindAdminAction(req.TenantID, "set_admin_key",
			map[string]string{"new_admin_pubkey_hex": req.NewAdminPubkey}, *req.Action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		action = bound
	}
	t, err := s.tenantMgr.SetAdminKey(req.TenantID, req.NewAdminPubkey, action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTenantCeiling(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		TenantID string              `json:"tenant_id"`
		Ceiling  float64             `json:"ceiling_usd"`
		Action   tenants.AdminAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	bound, err := s.bindAdminActionOrSession(r, req.TenantID, "set_ceiling",
		map[string]float64{"ceiling_usd": req.Ceiling}, req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	t, err := s.tenantMgr.SetCeiling(bound, req.Ceiling)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// handleTenantRefund returns consumed spend to a lab's books (audited
// refund primitive). Admin envelope binds tenant + amount + reason;
// suspended labs refuse (reactivate first — books never move frozen).
func (s *Server) handleTenantRefund(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "refund is POST-only", nil)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		TenantID string              `json:"tenant_id"`
		Amount   float64             `json:"amount_usd"`
		Reason   string              `json:"reason"`
		Action   tenants.AdminAction `json:"action"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}
	bound, err := s.bindAdminActionOrSession(r, req.TenantID, "refund_spend",
		map[string]any{"tenant_id": req.TenantID, "amount_usd": req.Amount, "reason": req.Reason}, req.Action)
	if err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	t, err := s.tenantMgr.RefundSpend(bound, req.TenantID, req.Amount, req.Reason)
	if err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTenantAutonomy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		TenantID string              `json:"tenant_id"`
		Level    string              `json:"level"`
		Action   tenants.AdminAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	bound, err := s.bindAdminActionOrSession(r, req.TenantID, "grant_autonomy",
		map[string]string{"level": req.Level}, req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	t, err := s.tenantMgr.GrantAutonomy(bound, req.Level)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// handleTenantStatus flips active/suspended (Q-33). Same rails as the
// other tenant mutations: bearer + limiter + admin-signed envelope.
func (s *Server) handleTenantStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		TenantID string              `json:"tenant_id"`
		Status   string              `json:"status"`
		Action   tenants.AdminAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	bound, err := s.bindAdminActionOrSession(r, req.TenantID, "set_status",
		map[string]string{"status": req.Status}, req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	t, err := s.tenantMgr.SetStatus(bound, req.Status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTenantRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		TenantID string              `json:"tenant_id"`
		Reason   string              `json:"reason"`
		Action   tenants.AdminAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	bound, err := s.bindAdminActionOrSession(r, req.TenantID, "revoke_autonomy",
		map[string]string{"reason": req.Reason}, req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	t, err := s.tenantMgr.RevokeAutonomy(bound, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// Revocation propagates to execution: abort every unsettled job in the
	// tenant's sessions so a revoked lab cannot keep burning budget.
	aborted, err := s.jobRunner.CancelTenantJobs(t.ID, "tenant autonomy revoked: "+req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 t.ID,
		"autonomy_level":     t.AutonomyLevel,
		"status":             t.Status,
		"aborted_jobs_count": aborted,
	})
}
