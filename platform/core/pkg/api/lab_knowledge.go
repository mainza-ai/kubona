package api

// Cumulative lab-knowledge registry (AG-9).
//
// Real outcomes publish entries automatically: a rejected/failed plan becomes
// a negative result, a collapsed hypothesis becomes a claim entry, and a signed
// manuscript becomes a report. A plan that overlaps a prior negative result is
// refused unless the caller explicitly acknowledges it, so a second campaign
// cannot silently repeat a known failure.
//
// Visibility is tenant-scoped. An owner operator may grant one grantee tenant
// access to one entry; QPU and foreign-record authority remains the UCAN grant
// rail, not this ACL.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

const priorNegativeThreshold = 0.8

func labKnowledgeSubpath(id string) (sessionID, rest string, ok bool) {
	idx := strings.Index(id, "/lab-knowledge")
	if idx <= 0 {
		return "", "", false
	}
	rest = strings.TrimPrefix(id[idx:], "/lab-knowledge")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

func (s *Server) handleLabKnowledgeRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	tenant := sess.TenantID
	if tenant == "" {
		tenant = "default"
	}
	switch {
	case rest == "":
		if r.Method != http.MethodGet {
			WriteAPIError(w, "method_not_allowed", "lab knowledge is GET-only; entries publish from outcomes", nil)
			return
		}
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		entries, err := s.ledger.ListKnowledge(tenant,
			strings.TrimSpace(r.URL.Query().Get("kind")),
			strings.TrimSpace(r.URL.Query().Get("q")), limit)
		if err != nil {
			WriteAPIError(w, "internal", "knowledge list failed", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenant_id": tenant, "entries": entries})
	case rest == "lookup":
		if r.Method != http.MethodGet {
			WriteAPIError(w, "method_not_allowed", "lookup is GET-only", nil)
			return
		}
		statement := strings.TrimSpace(r.URL.Query().Get("statement"))
		if statement == "" {
			WriteAPIError(w, "bad_request", "lookup needs a statement", nil)
			return
		}
		matches, err := s.ledger.LookupKnowledge(tenant, statement, 5)
		if err != nil {
			WriteAPIError(w, "internal", "knowledge lookup failed", nil)
			return
		}
		avoid := false
		for _, m := range matches {
			if m.Similarity >= priorNegativeThreshold {
				avoid = true
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": tenant, "statement": statement,
			"matches": matches, "avoid": avoid, "threshold": priorNegativeThreshold,
		})
	default:
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] == "share" {
			s.handleKnowledgeShare(w, r, tenant, parts[0])
			return
		}
		WriteAPIError(w, "not_found", "unknown lab-knowledge route", nil)
	}
}

func (s *Server) handleKnowledgeShare(w http.ResponseWriter, r *http.Request, tenant, id string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "share is POST-only", nil)
		return
	}
	if !s.requireOperator(w, r) {
		return
	}
	var body struct {
		GranteeTenant string `json:"grantee_tenant"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid share request: %v", err), nil)
		return
	}
	entry, err := s.ledger.ShareKnowledge(tenant, id, body.GranteeTenant)
	switch {
	case errors.Is(err, ledger.ErrKnowledgeNotFound):
		WriteAPIError(w, "not_found", "unknown knowledge entry", nil)
		return
	case errors.Is(err, ledger.ErrKnowledgeForbidden):
		WriteAPIError(w, "forbidden", "only the owning tenant may share this entry", nil)
		return
	case err != nil:
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(entry.SessionID, "knowledge_shared", map[string]any{
		"knowledge_id": entry.ID, "owner_tenant": tenant,
		"grantee_tenant": strings.TrimSpace(body.GranteeTenant),
	}); err != nil {
		slog.Warn("knowledge share event failed", "entry", entry.ID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry, "grantee_tenant": body.GranteeTenant})
}

// publishKnowledge writes one entry and seals a knowledge_published event. The
// primary operation has already committed, so a registry failure is logged and
// never rolls it back; the ledger rows remain the authority and the registry is
// a projection that can be rebuilt.
func (s *Server) publishKnowledge(sessionID, kind, title, statement string, evidence map[string]any) {
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil || sess == nil {
		return
	}
	raw, _ := json.Marshal(evidence)
	entry, err := s.ledger.PublishKnowledge(&ledger.KnowledgeEntry{
		TenantID: sess.TenantID, SessionID: sessionID, Kind: kind,
		Title: title, Statement: statement, Evidence: string(raw),
	})
	if err != nil {
		slog.Warn("knowledge publish failed", "session", sessionID, "kind", kind, "error", err)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "knowledge_published", map[string]any{
		"knowledge_id": entry.ID, "kind": entry.Kind, "confirmations": entry.Confirmations,
	}); err != nil {
		slog.Warn("knowledge publish event failed", "entry", entry.ID, "error", err)
	}
}

// publishPlanNegative records a rejected/failed plan as a negative result.
func (s *Server) publishPlanNegative(sessionID string, plan *ledger.ExperimentPlan, status, payload string) {
	var scored map[string]any
	_ = json.Unmarshal([]byte(payload), &scored)
	evidence := map[string]any{
		"plan_id": plan.ID, "hypothesis_id": plan.HypothesisID, "method": plan.Method,
		"observable": plan.Observable, "acceptance_metric": plan.AcceptanceMetric,
		"tolerance": plan.Tolerance, "status": status,
	}
	for k, v := range scored {
		evidence[k] = v
	}
	title := "Negative result: " + clipText(plan.Statement, 160)
	s.publishKnowledge(sessionID, ledger.KnowledgeNegativeResult, title, plan.Statement, evidence)
}

func clipText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
