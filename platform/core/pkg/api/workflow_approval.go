package api

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// workflowApprovalPayload is the exact bytes signed by the reviewer key. The
// workflow hash binds the approval to the submitted definition; the session
// binds it to the spend owner; the expiry bounds replay.
type workflowApprovalPayload struct {
	Version        int    `json:"v"`
	SessionID      string `json:"session_id"`
	WorkflowSHA256 string `json:"workflow_sha256"`
	ExpiresAt      string `json:"expires_at"`
	ApprovedBy     string `json:"approved_by"`
}

type workflowApproveRequest struct {
	SessionID  string          `json:"session_id"`
	Workflow   json.RawMessage `json:"workflow"`
	TTLSeconds int             `json:"ttl_seconds,omitempty"`
}

func workflowSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Server) signWorkflowApproval(sessionID, workflowHash, approvedBy string, expires time.Time) (string, workflowApprovalPayload, error) {
	p := workflowApprovalPayload{
		Version: 1, SessionID: sessionID, WorkflowSHA256: workflowHash,
		ExpiresAt: expires.UTC().Format(time.RFC3339), ApprovedBy: approvedBy,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", p, err
	}
	sig := ed25519.Sign(s.signerPrivKey, raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), p, nil
}

func (s *Server) verifyWorkflowApproval(token, sessionID, workflowHash string) (workflowApprovalPayload, error) {
	var p workflowApprovalPayload
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return p, fmt.Errorf("malformed workflow approval token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return p, fmt.Errorf("malformed workflow approval payload")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return p, fmt.Errorf("malformed workflow approval signature")
	}
	if !ed25519.Verify(s.signerPubKey, raw, sig) {
		return p, fmt.Errorf("workflow approval signature is invalid")
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("decode workflow approval: %w", err)
	}
	if p.Version != 1 || p.SessionID != sessionID || p.WorkflowSHA256 != workflowHash {
		return p, fmt.Errorf("workflow approval does not bind this session and workflow")
	}
	expires, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		return p, fmt.Errorf("workflow approval has expired")
	}
	return p, nil
}

// POST /api/workflows/approve — human/operator approval for one exact
// workflow definition. The bearer/session is the human authority; the
// returned token is signed by the server reviewer key and stored single-use.
func (s *Server) handleWorkflowApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to approve a workflow", nil)
		return
	}
	if !s.reviewerKeyConfigured && !ephemeralAllowed() {
		WriteAPIError(w, "server_misconfigured", "reviewer signing key not configured: workflow approvals are closed", nil)
		return
	}
	var req workflowApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "invalid JSON body: "+err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.SessionID) == "" || len(req.Workflow) == 0 {
		WriteAPIError(w, "bad_request", "session_id and workflow are required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "not_found", fmt.Sprintf("session %s not found", req.SessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	if err := s.requireDispatchAutonomy(sess.ID); err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	var wf struct {
		CreatedByAgent bool `json:"created_by_agent"`
		Steps          []struct {
			RequiresApproval bool `json:"requires_approval"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(req.Workflow, &wf); err != nil || len(wf.Steps) == 0 {
		WriteAPIError(w, "bad_request", "workflow must contain a non-empty steps list", nil)
		return
	}
	needsApproval := wf.CreatedByAgent
	for _, step := range wf.Steps {
		needsApproval = needsApproval || step.RequiresApproval
	}
	if !needsApproval {
		WriteAPIError(w, "bad_request", "workflow does not require approval; run it without an approval token", nil)
		return
	}
	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = 3600
	}
	if ttl > 24*3600 {
		WriteAPIError(w, "bad_request", "ttl_seconds exceeds the 24h approval ceiling", nil)
		return
	}
	tenantID, fp, _ := s.checkAuth(r)
	approvedBy := "operator-bearer"
	if tenantID != "" {
		approvedBy = "tenant:" + tenantID
		if fp != "" {
			approvedBy += ":key:" + fp
		}
	}
	expires := time.Now().UTC().Add(time.Duration(ttl) * time.Second)
	workflowHash := workflowSHA256(req.Workflow)
	token, payload, err := s.signWorkflowApproval(req.SessionID, workflowHash, approvedBy, expires)
	if err != nil {
		WriteAPIError(w, "internal", "could not sign workflow approval", nil)
		return
	}
	tokenHash := workflowSHA256([]byte(token))
	if err := s.ledger.CreateWorkflowApproval(ledger.WorkflowApproval{
		TokenHash: tokenHash, SessionID: req.SessionID, WorkflowSHA256: workflowHash,
		ApprovedBy: approvedBy, ExpiresAt: payload.ExpiresAt,
	}); err != nil {
		WriteAPIError(w, "internal", "could not persist workflow approval: "+err.Error(), nil)
		return
	}
	s.ledger.AppendEvent(sess.ID, "workflow_approved", map[string]any{
		"workflow_sha256": workflowHash, "approved_by": approvedBy, "expires_at": payload.ExpiresAt,
		"approval_hash": tokenHash,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "approval_token": token, "workflow_sha256": workflowHash,
		"expires_at": payload.ExpiresAt, "approved_by": approvedBy,
	})
}
