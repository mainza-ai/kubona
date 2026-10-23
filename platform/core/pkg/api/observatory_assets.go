package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// Observatory asset + resolve rails (P1).
//
// POST /api/observatory/assets — upload inline base64 media (<10 MiB, the
// Atlas inline limit) as a project asset. No Kubona spend is debited:
// asset creation carried no charge across the Q-38 probe (4 creates, no
// 402); Atlas-side billing, if any, surfaces through the account.
// POST /api/observatory/lens/resolve — read an Atlas operation to terminal
// state, persist promptUsed + outputs to the ledger once, and (only when
// the dispatch requested ephemeral_purge) download outputs locally before
// the cloud purge window closes.

// inlineAssetMaxDecodedBytes caps inline media at 9.5 MiB DECODED bytes
// (Atlas: inline media <10 MiB decoded; base64 inflates ~4/3 on the wire).
// A 7.4 MiB uncompressed RGBA-half depth EXR passes; genuinely oversize
// payloads fail closed. Verified live 2026-10-23.
const inlineAssetMaxDecodedBytes = int64(950 * 1024 * 1024 / 100)

// inlineDecodedSize estimates decoded bytes of a data: URL payload.
func inlineDecodedSize(dataURL string) int64 {
	comma := strings.Index(dataURL, ",")
	if comma < 0 {
		return int64(len(dataURL))
	}
	return int64(len(dataURL)-comma-1) * 3 / 4
}

// ObservatoryAssetRequest uploads one inline asset.
type ObservatoryAssetRequest struct {
	SessionID   string `json:"session_id"`
	DataBase64  string `json:"data_base64"` // data: URL
	DisplayName string `json:"display_name"`
}

// fetchAtlasOperation GETs one Atlas operation; callers own the body.
func fetchAtlasOperation(apiBase, apiKey, opID string) (*http.Response, []byte, error) {
	baseURL := atlasBaseURL(apiBase)
	reqURL := fmt.Sprintf("%s/operations/%s", baseURL, opID)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("WLT-Api-Key", apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return resp, bodyBytes, nil
}

func (s *Server) handleObservatoryAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to upload an observatory asset", nil)
		return
	}
	var req ObservatoryAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}
	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	if sess, err := s.sessionMgr.GetSession(req.SessionID); err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}
	dataURL := strings.TrimSpace(req.DataBase64)
	if !strings.HasPrefix(dataURL, "data:") {
		WriteAPIError(w, "bad_request", "data_base64 must be a data: URL (inline media only)", nil)
		return
	}
	if inlineDecodedSize(dataURL) > inlineAssetMaxDecodedBytes {
		WriteAPIError(w, "bad_request", "inline asset exceeds 9.5 MiB decoded: use the chunked upload flow instead", nil)
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = fmt.Sprintf("kubona-lens-%s", req.SessionID)
	}

	apiKey := os.Getenv("WLT_API_KEY")
	apiBase := os.Getenv("WLT_API_BASE_URL")
	if apiKey == "" {
		WriteAPIError(w, "llm_unavailable", "World Labs Atlas API key (WLT_API_KEY) unconfigured on server", nil)
		return
	}

	idemKey := "lens-asset-" + uuid.NewString()
	assetID, err := uploadInlineAssetToAtlas(atlasBaseURL(apiBase), apiKey, idemKey, dataURL, displayName)
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("Atlas asset upload failed: %v", err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "asset_id": assetID, "idempotency_key": idemKey,
	})
}

// ObservatoryLensResolveRequest reads one operation to a terminal record.
type ObservatoryLensResolveRequest struct {
	SessionID   string `json:"session_id"`
	OperationID string `json:"operation_id"`
}

// artifactsDir resolves ~/.kubona/artifacts (KUBONA_ARTIFACTS_DIR wins in
// tests so runs never touch the operator home).
func artifactsDir() (string, error) {
	if dir := os.Getenv("KUBONA_ARTIFACTS_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kubona", "artifacts"), nil
}

func (s *Server) handleObservatoryLensResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to resolve a lens operation", nil)
		return
	}
	var req ObservatoryLensResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}
	if req.SessionID == "" || req.OperationID == "" {
		WriteAPIError(w, "bad_request", "session_id and operation_id are required", nil)
		return
	}
	if sess, err := s.sessionMgr.GetSession(req.SessionID); err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}

	record, err := s.resolveLensOperation(req.SessionID, req.OperationID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "unconfigured"):
			WriteAPIError(w, "llm_unavailable", msg, nil)
		case strings.HasPrefix(msg, "credits_unavailable:"),
			strings.HasPrefix(msg, "spend_limit_exceeded:"),
			strings.HasPrefix(msg, "task_not_entitled:"),
			strings.HasPrefix(msg, "rate_limited:"):
			parts := strings.SplitN(msg, ": ", 2)
			detail := msg
			if len(parts) == 2 {
				detail = parts[1]
			}
			WriteAPIError(w, parts[0], detail, nil)
		default:
			WriteAPIError(w, "bridge_unavailable", msg, nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// handleObservatoryLensHistory lists this session's lens dispatches merged
// with their resolve records (prompt_used, local paths). This is OUR
// reservation ledger — rated invoices live on the Atlas billing dashboard,
// so the UI must present these as reservations, never as billed amounts.
func (s *Server) handleObservatoryLensHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required for lens history", nil)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id query parameter is required", nil)
		return
	}
	if sess, err := s.sessionMgr.GetSession(sessionID); err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", sessionID), nil)
		return
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read history: %v", err), nil)
		return
	}
	type opRec struct {
		OperationID string   `json:"operation_id"`
		TaskType    string   `json:"task_type,omitempty"`
		CostUSD     float64  `json:"cost_usd,omitempty"`
		PromptUsed  string   `json:"prompt_used,omitempty"`
		Resolved    bool     `json:"resolved"`
		Ephemeral   bool     `json:"ephemeral_purge,omitempty"`
		LocalPaths  []string `json:"local_paths,omitempty"`
	}
	byOp := map[string]*opRec{}
	var order []string
	for _, e := range events {
		if e.EventType != "annotation" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		id, _ := p["operation_id"].(string)
		if id == "" {
			continue
		}
		// Only lens dispatches carry task_type or idempotency_key.
		if _, isResolve := p["resolved"]; !isResolve {
			if _, ok := p["task_type"]; !ok {
				if _, ok := p["idempotency_key"]; !ok {
					continue
				}
			}
		}
		rec, ok := byOp[id]
		if !ok {
			rec = &opRec{OperationID: id}
			byOp[id] = rec
			order = append(order, id)
		}
		if t, _ := p["task_type"].(string); t != "" && rec.TaskType == "" {
			rec.TaskType = t
		}
		if c, ok := p["cost_usd"].(float64); ok && rec.CostUSD == 0 {
			rec.CostUSD = c
		}
		if pu, _ := p["prompt_used"].(string); pu != "" {
			rec.PromptUsed = pu
		}
		if r, _ := p["resolved"].(bool); r {
			rec.Resolved = true
		}
		if ep, _ := p["ephemeral_purge"].(bool); ep {
			rec.Ephemeral = true
		}
	}
	// Events arrive newest-first; reverse for chronological order.
	ops := make([]opRec, 0, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		ops = append(ops, *byOp[order[i]])
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "operations": ops})
}

// findLensAnnotation scans newest-first for the dispatch (resolved=false)
// or resolve (resolved=true) annotation of one operation. Returns the
// stored record shape so repeat resolves are reads, not duplicates.
func findLensAnnotation(l *ledger.Ledger, sessionID, opID string, resolved bool) map[string]any {
	events, err := l.GetRecentEvents(sessionID, 500)
	if err != nil {
		return nil
	}
	for _, e := range events {
		if e.EventType != "annotation" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		id, _ := p["operation_id"].(string)
		if id == "" || id != opID {
			continue
		}
		_, isResolve := p["resolved"]
		if isResolve != resolved {
			continue
		}
		if !resolved {
			return p
		}
		return map[string]any{
			"ok": true, "done": true,
			"operation_id": opID,
			"prompt_used":  p["prompt_used"],
		}
	}
	return nil
}

func downloadFile(url, dst string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url) //nolint:gosec // output URLs are https Atlas storage links from a verified operation record
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download rejected (%d)", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
