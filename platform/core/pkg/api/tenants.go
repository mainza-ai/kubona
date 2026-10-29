package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// Tenant self-provisioning is open by design (self-hosted labs): anyone may
// register a lab tenant, which starts at L1 with the default ceiling and an
// empty admin key. The first SetAdminKey wins (ledger-sealed bootstrap);
// every mutation after that requires the admin's signature.
func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Labs view: anonymous access is single-lab-only (Q-57); multi-lab
		// callers authenticate, and a lab session sees only its own row.
		// Tenant rows carry admin key ids and spend totals, so the old public
		// list leaked the whole estate to any network reader.
		if hasNoCredentials(r) {
			all, err := s.tenantMgr.ListTenants()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(all) > 1 {
				WriteAPIError(w, "unauthorized", "multi-lab store: log in or present the operator token to list labs", nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tenants": all})
			return
		}
		tenant, ok := s.checkCallerAuth(w, r)
		if !ok {
			return
		}
		if tenant != "" {
			t, err := s.tenantMgr.GetTenant(tenant)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Phase-2 grants: a lab caller sees its own row plus the rows
			// of labs that granted it the read scope. No grant = own row
			// only (default deny preserved; anonymous/bearer paths below
			// and in handleTenantByID are untouched).
			visible := []tenants.Tenant{*t}
			if all, lerr := s.tenantMgr.ListTenants(); lerr == nil {
				for _, other := range all {
					if other.ID == tenant {
						continue
					}
					if s.tenantMgr.CheckGrant(other.ID, tenant, "", tenants.ReadScope) == nil {
						visible = append(visible, other)
					}
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"tenants": visible})
			return
		}
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
	// D-09 caller-vs-tenant binding (same rule as the list above): tenant
	// rows carry admin key ids and spend totals, so an unscoped read leaks
	// the estate lab by lab. Anonymous reads only while the store holds a
	// single lab (same tenants-table count as the list); lab callers read
	// only their own row; bearer reads all.
	if hasNoCredentials(r) {
		all, err := s.tenantMgr.ListTenants()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(all) > 1 {
			WriteAPIError(w, "unauthorized", "multi-lab store: log in or present the operator token to read labs", nil)
			return
		}
	} else if tenant, ok := s.checkCallerAuth(w, r); !ok {
		return
	} else if tenant != "" && tenant != id {
		// Phase-2 grants: a lab caller may read another lab's row when that
		// lab granted it the read scope (checked live on every read).
		// Without a covering grant this refuses exactly as before — and a
		// guessed id still 403s whether or not the lab exists (no oracle:
		// the grant lookup fails closed for ghosts the same way).
		if s.tenantMgr.CheckGrant(id, tenant, "", tenants.ReadScope) != nil {
			WriteAPIError(w, "forbidden", "tenant belongs to another lab: refused", nil)
			return
		}
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
	if t.Status == "suspended" {
		if _, err := s.jobRunner.CancelTenantJobs(t.ID, "tenant suspended: execution stops until reactivation"); err != nil {
			http.Error(w, "tenant suspended but job cancellation failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Pause every active autoresearch engine owned by this lab. The
		// engine's session carries the tenant; jobs are already fenced.
		s.autoresearchMu.RLock()
		type loopRef struct {
			id string
			e  *autoresearch.LoopEngine
		}
		var loops []loopRef
		for id, engine := range s.activeAutoloops {
			st := engine.State()
			if sess, serr := s.sessionMgr.GetSession(st.SessionID); serr == nil && sess.TenantID == t.ID {
				loops = append(loops, loopRef{id: id, e: engine})
			}
		}
		s.autoresearchMu.RUnlock()
		for _, l := range loops {
			l.e.Pause()
			_ = s.supervisor.Pause(l.id)
		}
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
