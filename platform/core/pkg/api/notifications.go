package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// Notification is the body-free, server-owned inbox projection. It is derived
// from durable obligations + selected ledger events; notification_states only
// records read/resolve disposition, so the audit chain stays the source.
type Notification struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Source     string `json:"source"`
	Message    string `json:"message"`
	Action     string `json:"action,omitempty"`
	CreatedAt  string `json:"created_at"`
	ReadAt     string `json:"read_at,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

var notificationKinds = map[string]bool{
	"job": true, "run": true, "proposal": true, "claim": true,
	"workflow": true, "memory": true, "allowance": true, "obligation": true,
}

func notificationForEvent(e ledger.EventRecord) (Notification, bool) {
	var p map[string]any
	_ = json.Unmarshal(e.Payload, &p)
	str := func(key string) string {
		if v, ok := p[key].(string); ok {
			return v
		}
		return ""
	}
	short := func(v string) string {
		if len(v) > 8 {
			return v[:8]
		}
		return v
	}
	n := Notification{Key: fmt.Sprintf("event:%d", e.ID), Source: e.EventType, CreatedAt: e.Timestamp}
	switch e.EventType {
	case "job_settled":
		n.Kind, n.Severity = "job", "info"
		n.Message = "Job " + short(str("id")) + " completed"
		n.Action = "job:" + str("id")
	case "job_cancelled":
		n.Kind, n.Severity = "job", "warning"
		n.Message = "Job " + short(str("id")) + " was cancelled"
		n.Action = "job:" + str("id")
	case "loop_completed":
		n.Kind, n.Severity = "run", "info"
		n.Message = "Run " + short(str("loop_id")) + " completed"
		n.Action = "run:" + str("loop_id")
	case "loop_aborted":
		n.Kind, n.Severity = "run", "warning"
		n.Message = "Run " + short(str("loop_id")) + " aborted"
		n.Action = "run:" + str("loop_id")
	case "loop_stalled_timeout":
		n.Kind, n.Severity = "run", "warning"
		n.Message = "Run " + short(str("loop_id")) + " stalled"
		n.Action = "run:" + str("loop_id")
	case "proposal_generated":
		n.Kind, n.Severity = "proposal", "info"
		n.Message = "A launchable proposal is ready"
		n.Action = "proposal:" + str("proposal_id")
	case "proposal_dismissed":
		n.Kind, n.Severity = "proposal", "info"
		n.Message = "Proposal " + short(str("proposal_id")) + " dismissed"
		n.Action = "proposal:" + str("proposal_id")
	case "proposal_launched":
		n.Kind, n.Severity = "proposal", "info"
		n.Message = "Proposal " + short(str("proposal_id")) + " launched"
		n.Action = "proposal:" + str("proposal_id")
	case "peer_review":
		n.Kind, n.Severity = "claim", "info"
		n.Message = "Claim review activity recorded"
		n.Action = "claim:" + str("claim_id")
	case "workflow_execution_completed":
		n.Kind, n.Severity = "workflow", "info"
		n.Message = "Workflow completed"
	case "workflow_execution_failed":
		n.Kind, n.Severity = "workflow", "warning"
		n.Message = "Workflow failed"
	case "memory_retracted":
		n.Kind, n.Severity = "memory", "info"
		n.Message = "A memory was retracted"
	case "run_allowance_suspended", "run_allowance_revoked":
		n.Kind, n.Severity = "allowance", "warning"
		n.Message = strings.ReplaceAll(e.EventType, "_", " ")
	default:
		return Notification{}, false
	}
	return n, true
}

func (s *Server) notificationList(sessionID string, limit int) ([]Notification, int, error) {
	states, err := s.ledger.NotificationStates(sessionID)
	if err != nil {
		return nil, 0, err
	}
	prefs, err := s.ledger.GetNotificationPrefs(sessionID)
	if err != nil {
		return nil, 0, err
	}
	muted := make(map[string]bool, len(prefs.MutedKinds))
	for _, kind := range prefs.MutedKinds {
		muted[kind] = true
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 300)
	if err != nil {
		return nil, 0, err
	}
	var out []Notification
	for _, event := range events {
		n, ok := notificationForEvent(event)
		if !ok {
			continue
		}
		if st, ok := states[n.Key]; ok {
			n.ReadAt, n.ResolvedAt = st.ReadAt, st.ResolvedAt
		}
		if n.ResolvedAt != "" || muted[n.Kind] {
			continue
		}
		out = append(out, n)
	}
	// Session-owned obligations are durable work, not ledger events.
	if obs, oerr := s.ledger.ListOpenObligations(100, "session:"+sessionID, ""); oerr == nil {
		for _, ob := range obs {
			key := "obligation:" + ob.ID
			n := Notification{
				Key: key, Kind: "obligation", Severity: "warning", Source: ob.SubjectKind,
				Message: "Decision needed: " + ob.SubjectKind + " " + ob.SubjectID,
				Action:  "obligation:" + ob.ID, CreatedAt: ob.UpdatedAt.UTC().Format(time.RFC3339),
			}
			if st, ok := states[key]; ok {
				n.ReadAt, n.ResolvedAt = st.ReadAt, st.ResolvedAt
			}
			if n.ResolvedAt == "" && !muted[n.Kind] {
				out = append(out, n)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	// Coalesce by subject: a completed run supersedes its earlier stalled
	// warning, and a launched proposal supersedes its "ready" notice. The
	// inbox is an actionable current-state view, not a second audit log.
	seenSubjects := map[string]bool{}
	coalesced := out[:0]
	for _, n := range out {
		subject := n.Kind + "|" + n.Action
		if seenSubjects[subject] {
			continue
		}
		seenSubjects[subject] = true
		coalesced = append(coalesced, n)
	}
	out = coalesced
	unread := 0
	for _, n := range out {
		if n.ReadAt == "" {
			unread++
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []Notification{}
	}
	return out, unread, nil
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
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
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		rows, unread, err := s.notificationList(sessionID, limit)
		if err != nil {
			WriteAPIError(w, "internal", "notification read failed", nil)
			return
		}
		prefs, _ := s.ledger.GetNotificationPrefs(sessionID)
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": sessionID, "notifications": rows, "unread": unread,
			"preferences":  prefs,
			"generated_at": time.Now().UTC().Format(time.RFC3339),
		})
	case http.MethodPost:
		var req struct {
			SessionID string   `json:"session_id"`
			Keys      []string `json:"keys"`
			Action    string   `json:"action"`
		}
		if err := decodeStrict(r, &req); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid notification state request: %v", err), nil)
			return
		}
		if req.SessionID == "" || len(req.Keys) == 0 {
			WriteAPIError(w, "bad_request", "session_id and keys are required", nil)
			return
		}
		if req.Action != "read" && req.Action != "resolve" {
			WriteAPIError(w, "bad_request", "action must be read or resolve", nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
			return
		}
		if err := s.ledger.MarkNotifications(req.SessionID, req.Keys, req.Action == "resolve"); err != nil {
			WriteAPIError(w, "internal", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": len(req.Keys)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if r.Method == http.MethodGet {
		if sessionID == "" {
			WriteAPIError(w, "bad_request", "session_id is required", nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		prefs, err := s.ledger.GetNotificationPrefs(sessionID)
		if err != nil {
			WriteAPIError(w, "internal", "notification preferences read failed", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "preferences": prefs})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID  string   `json:"session_id"`
		Quiet      bool     `json:"quiet"`
		MutedKinds []string `json:"muted_kinds"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid notification preferences: %v", err), nil)
		return
	}
	if req.SessionID == "" {
		req.SessionID = sessionID
	}
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	for _, kind := range req.MutedKinds {
		if !notificationKinds[kind] {
			WriteAPIError(w, "bad_request", "unknown notification kind: "+kind, nil)
			return
		}
	}
	if err := s.ledger.SetNotificationPrefs(req.SessionID, req.Quiet, req.MutedKinds); err != nil {
		WriteAPIError(w, "internal", err.Error(), nil)
		return
	}
	prefs, _ := s.ledger.GetNotificationPrefs(req.SessionID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "preferences": prefs})
}
