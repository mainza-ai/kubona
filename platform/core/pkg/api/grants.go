package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mainza-ai/kubona/core/pkg/grants"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// trustRoots builds the did:key -> tenant map from registered lab admin keys.
func (s *Server) trustRoots() (map[string]string, error) {
	all, err := s.tenantMgr.ListTenants()
	if err != nil {
		return nil, err
	}
	admins := make([]grants.AdminDID, 0, len(all))
	for _, t := range all {
		admins = append(admins, grants.AdminDID{TenantID: t.ID, AdminPubkeyHex: t.AdminPubkeyHex, Status: t.Status})
	}
	return grants.TrustRootsForTenants(admins), nil
}

// handleGrantRegister verifies a presented cross-lab UCAN grant chain and
// seals it as a grant_issued ledger event. Registration is what makes the
// chain's CIDs discoverable for revocation; verification here is identical
// to dispatch-time verification (same function, no second rule).
func (s *Server) handleGrantRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.grantLimiter.allow(clientKey(r)) {
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Chain []string `json:"chain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	chain, err := grants.DecodeChainB64(req.Chain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	roots, err := s.trustRoots()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vg, err := grants.VerifyChain(chain, grants.VerifyOptions{
		TrustedRoots: roots,
		Revoked:      grants.RevokedCIDs(s.ledger.DB()),
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	rec, err := s.ledger.AppendSignedEvent(vg.TargetTenant, "grant_issued", map[string]any{
		"root_cid": vg.RootCID, "leaf_cid": vg.LeafCID,
		"granting_tenant": vg.GrantingTenant, "target_tenant": vg.TargetTenant,
		"max_budget_usd": vg.MaxBudgetUSD, "scopes": vg.Scopes, "command": vg.Command,
	}, s.signerPrivKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "root_cid": vg.RootCID, "leaf_cid": vg.LeafCID,
		"granting_tenant": vg.GrantingTenant, "target_tenant": vg.TargetTenant,
		"max_budget_usd": vg.MaxBudgetUSD, "scopes": vg.Scopes,
		"ledger_receipt": rec.EntryHash,
	})
}

// handleGrantRevoke revokes a grant chain by root CID (whole chain refuses)
// or leaf CID (chains containing that exact envelope refuse — W0.8).
// Exactly one of the two is required. Authorization: the caller must present
// a valid admin envelope for the GRANTING tenant (looked up from the stored
// grant_issued event) — only the lab that issued a grant can revoke it.
// Revocation is a ledger event, effective on next dispatch.
func (s *Server) handleGrantRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.grantLimiter.allow(clientKey(r)) {
		http.Error(w, "rate limit exceeded: slow down and retry", http.StatusTooManyRequests)
		return
	}
	var req struct {
		RootCID string              `json:"root_cid"`
		LeafCID string              `json:"leaf_cid"`
		Reason  string              `json:"reason"`
		Action  tenants.AdminAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Exactly one target: revoking "everything" is not an operation.
	if (req.RootCID == "") == (req.LeafCID == "") {
		http.Error(w, "exactly one of root_cid, leaf_cid is required", http.StatusBadRequest)
		return
	}
	granting, err := s.grantingTenantFor(req.RootCID, req.LeafCID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	params := map[string]string{"reason": req.Reason}
	if req.RootCID != "" {
		params["root_cid"] = req.RootCID
	} else {
		params["leaf_cid"] = req.LeafCID
	}
	bound, err := bindAdminAction(granting, "revoke_grant", params, req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// Cryptographic verification against the granting lab's CURRENT admin key
	// (rotation-safe): envelope must be signed by the key on the tenant row.
	verified, err := s.tenantMgr.VerifyAdminAction(bound)
	if err != nil || verified.ID != granting {
		msg := "admin signature verification failed"
		if err != nil {
			msg = err.Error()
		}
		http.Error(w, msg, http.StatusForbidden)
		return
	}
	rec, err := s.ledger.AppendSignedEvent(granting, "grant_revoked", map[string]any{
		"root_cid": req.RootCID, "leaf_cid": req.LeafCID,
		"reason": req.Reason, "revoked_by": req.Action.PubkeyHex,
	}, s.signerPrivKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "root_cid": req.RootCID, "leaf_cid": req.LeafCID,
		"ledger_receipt": rec.EntryHash,
	})
}

// grantingTenantFor resolves which lab issued a registered grant chain,
// matching by root CID (whole chain) or leaf CID (exact envelope) — W0.8.
// The scan covers ALL grant_issued events: the old LIMIT 50 made grants
// older than fifty registrations irrevocable while still spendable.
func (s *Server) grantingTenantFor(rootCID, leafCID string) (string, error) {
	rows, err := s.ledger.DB().Query(
		`SELECT payload FROM ledger_events WHERE event_type = 'grant_issued' ORDER BY id DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var payloadStr string
		var rec struct {
			RootCID        string `json:"root_cid"`
			LeafCID        string `json:"leaf_cid"`
			GrantingTenant string `json:"granting_tenant"`
		}
		if err := rows.Scan(&payloadStr); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(payloadStr), &rec); err != nil {
			continue
		}
		if rootCID != "" && rec.RootCID == rootCID {
			return rec.GrantingTenant, nil
		}
		if leafCID != "" && rec.LeafCID == leafCID {
			return rec.GrantingTenant, nil
		}
	}
	return "", fmt.Errorf("no registered grant matches root %q leaf %q", rootCID, leafCID)
}
