package api

// Operator-owned tenant default for the agentic master switch (AG-24).
//
// A session policy overrides the default. The default is not a lab-admin
// action: it is a deployment-level UI posture, while tenant autonomy levels
// (L0-L3) remain the authority boundary. The operator bearer is the same token
// that can halt a session or roll back its policy.

import (
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleTenantAgenticDefault(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
		if tenantID == "" {
			WriteAPIError(w, "bad_request", "tenant_id is required", nil)
			return
		}
		enabled, err := s.ledger.GetTenantAgenticDefault(tenantID)
		if err != nil {
			WriteAPIError(w, "internal", "tenant default read failed", nil)
			return
		}
		if enabled == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"tenant_id": tenantID, "enabled": nil, "source": "unset",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": tenantID, "enabled": *enabled, "source": "tenant-default",
		})
	case http.MethodPut:
		var body struct {
			TenantID string `json:"tenant_id"`
			Enabled  *bool  `json:"enabled"`
		}
		if err := decodeStrict(r, &body); err != nil || body.Enabled == nil {
			WriteAPIError(w, "bad_request", "tenant default needs tenant_id and enabled", nil)
			return
		}
		tenantID := strings.TrimSpace(body.TenantID)
		if tenantID == "" {
			WriteAPIError(w, "bad_request", "tenant_id is required", nil)
			return
		}
		if _, err := s.tenantMgr.GetTenant(tenantID); err != nil {
			WriteAPIError(w, "not_found", fmt.Sprintf("unknown tenant %q", tenantID), nil)
			return
		}
		if err := s.ledger.SetTenantAgenticDefault(tenantID, *body.Enabled); err != nil {
			WriteAPIError(w, "internal", "tenant default save failed", nil)
			return
		}
		if err := s.ledger.AppendEvent("global", "tenant_agentic_default_updated", map[string]any{
			"tenant_id": tenantID, "enabled": *body.Enabled, "actor": "operator",
		}); err != nil {
			WriteAPIError(w, "internal", "tenant default event failed: "+err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": tenantID, "enabled": *body.Enabled, "source": "tenant-default",
		})
	default:
		WriteAPIError(w, "method_not_allowed", "tenant agentic default supports GET and PUT", nil)
	}
}
