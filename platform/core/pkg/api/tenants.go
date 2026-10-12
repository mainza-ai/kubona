package api

import (
	"encoding/json"
	"net/http"
	"strings"

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
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
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

type httpErr string

func (e httpErr) Error() string { return string(e) }

func httpError(msg string) error { return httpErr(msg) }

func (s *Server) handleTenantAdminKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
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
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
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
	bound, err := bindAdminAction(req.TenantID, "set_ceiling",
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

func (s *Server) handleTenantAutonomy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
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
	bound, err := bindAdminAction(req.TenantID, "grant_autonomy",
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
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
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
	bound, err := bindAdminAction(req.TenantID, "set_status",
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
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
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
	bound, err := bindAdminAction(req.TenantID, "revoke_autonomy",
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
