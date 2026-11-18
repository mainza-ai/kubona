package api

// One-time tool approvals (AG-21b).
//
// A session policy can mark a tool `ask`. The dispatch gate then holds the
// exact call: it seals a durable request (tool + canonical argument digest),
// refuses that invocation with a remedy, and proceeds only after a human
// approves that same call. An approval is spent once; a different argument
// shape needs its own approval. A denial is durable and visible.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

const (
	toolApprovalPreviewRunes = 4000
	toolApprovalDecisionTTL  = time.Hour
)

func toolApprovalSubpath(id string) (sessionID, approvalID string, ok bool) {
	idx := strings.Index(id, "/tool-approvals")
	if idx <= 0 {
		return "", "", false
	}
	rest := strings.TrimPrefix(id[idx:], "/tool-approvals")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

// toolCallDigest canonicalizes arguments so whitespace or key order cannot
// create a second approval slot for the same call. The digest covers the full
// arguments; the returned preview is only what a reviewer can read.
func toolCallDigest(name, argsJSON string) (hash, preview string, truncated bool) {
	canonical := strings.TrimSpace(argsJSON)
	if canonical == "" {
		canonical = "{}"
	}
	var decoded any
	if err := json.Unmarshal([]byte(canonical), &decoded); err == nil {
		if compact, err := json.Marshal(decoded); err == nil {
			canonical = string(compact)
		}
	}
	sum := sha256.Sum256([]byte(name + "\x00" + canonical))
	preview = canonical
	if len(preview) > toolApprovalPreviewRunes {
		preview = preview[:toolApprovalPreviewRunes] + "…(truncated)"
		truncated = true
	}
	return hex.EncodeToString(sum[:]), preview, truncated
}

func (s *Server) handleToolApprovalsRoute(w http.ResponseWriter, r *http.Request, sessionID, approvalID string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	if approvalID == "" {
		if r.Method != http.MethodGet {
			WriteAPIError(w, "method_not_allowed", "tool approvals are GET-only; decide on /tool-approvals/{id}", nil)
			return
		}
		items, err := s.ledger.ListToolApprovals(sessionID, 100)
		if err != nil {
			WriteAPIError(w, "internal", "tool approval read failed", nil)
			return
		}
		pending := make([]ledger.ToolApproval, 0, len(items))
		recent := make([]ledger.ToolApproval, 0, len(items))
		counts := map[string]int{"pending": 0, "approved": 0, "denied": 0, "used": 0, "expired": 0}
		cutoff := time.Now().UTC().Add(-toolApprovalDecisionTTL)
		for _, item := range items {
			if item.Status == ledger.ToolApprovalPending {
				pending = append(pending, item)
				counts["pending"]++
				continue
			}
			if at, err := time.Parse(time.RFC3339, item.RequestedAt); err == nil && at.Before(cutoff) && item.Status != ledger.ToolApprovalApproved {
				continue
			}
			recent = append(recent, item)
			if item.Expired {
				counts["expired"]++
				continue
			}
			if _, ok := counts[item.Status]; ok {
				counts[item.Status]++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": sessionID, "pending": pending, "recent": recent, "counts": counts,
		})
		return
	}
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "tool approval decisions are POST-only", nil)
		return
	}
	var body struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid tool approval request: %v", err), nil)
		return
	}
	decision := strings.ToLower(strings.TrimSpace(body.Decision))
	if decision != "approve" && decision != "deny" {
		WriteAPIError(w, "bad_request", "decision must be approve or deny", nil)
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len(reason) > 1000 {
		WriteAPIError(w, "bad_request", "reason is limited to 1000 characters", nil)
		return
	}
	updated, err := s.ledger.DecideToolApproval(sessionID, approvalID, decision, reason, "session")
	if errors.Is(err, ledger.ErrToolApprovalNotFound) {
		WriteAPIError(w, "not_found", "unknown tool approval", nil)
		return
	}
	if errors.Is(err, ledger.ErrToolApprovalNotPending) {
		WriteAPIError(w, "tool_approval_not_pending",
			"this tool call was already decided or spent; a new call creates a new request", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "tool approval decision failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "tool_approval_decided", map[string]any{
		"approval_id": updated.ID, "tool": updated.Tool, "decision": decision,
		"reason": reason, "expires_at": updated.ExpiresAt,
	}); err != nil {
		WriteAPIError(w, "internal", "tool approval event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approval": updated})
}

// gateToolDispatch enforces allow | ask | block before the tool body runs.
// `ask` is a real one-time approval rail: a matching unexpired grant is spent
// atomically; otherwise the request is recorded and the call refuses with the
// request id. A denied call is not retried automatically.
func (s *Server) gateToolDispatch(sessionID string, policy autonomy.Policy, policyErr error, toolName, argsJSON string) error {
	if policyErr != nil {
		_ = s.recordManuscriptEvent(sessionID, "tool_permission_refused", map[string]any{
			"tool": toolName, "permission": "unreadable",
		})
		return fmt.Errorf("session autonomy policy is unreadable: refusing tool %q", toolName)
	}
	switch policy.ToolPermission(toolName) {
	case "allow":
		return nil
	case "block":
		_ = s.recordManuscriptEvent(sessionID, "tool_permission_refused", map[string]any{
			"tool": toolName, "permission": "block",
		})
		return fmt.Errorf("tool %q is blocked by the session policy", toolName)
	default: // ask
		hash, preview, truncated := toolCallDigest(toolName, argsJSON)
		if id, spent, err := s.ledger.ConsumeToolApproval(sessionID, toolName, hash); err != nil {
			return fmt.Errorf("tool approval lookup failed: refusing tool %q", toolName)
		} else if spent {
			_ = s.recordManuscriptEvent(sessionID, "tool_approval_consumed", map[string]any{
				"approval_id": id, "tool": toolName,
			})
			return nil
		}
		request, created, err := s.ledger.RequestToolApproval(sessionID, toolName, hash, preview, truncated)
		if err != nil {
			return fmt.Errorf("tool %q needs approval and the request could not be recorded: %v", toolName, err)
		}
		if created {
			_ = s.recordManuscriptEvent(sessionID, "tool_approval_requested", map[string]any{
				"approval_id": request.ID, "tool": toolName, "args_hash": hash,
				"args_preview": preview, "args_truncated": truncated,
			})
		}
		_ = s.recordManuscriptEvent(sessionID, "tool_permission_refused", map[string]any{
			"tool": toolName, "permission": "ask", "approval_id": request.ID,
		})
		return fmt.Errorf("tool %q needs a one-time approval for these exact arguments: approve request %s in the Approvals inbox, then ask again", toolName, request.ID)
	}
}
