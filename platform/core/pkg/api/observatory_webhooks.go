package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Atlas webhook receiver (P6): Standard Webhooks Ed25519 (v1a) verification.
// Signature headers (NOT HMAC, no WLT-Signature — the live contract uses
// Ed25519 `v1a` over "{webhook-id}.{timestamp}.{raw body}").
//
// Threat model: the route is PUBLIC (Atlas cannot present our bearer), so the
// signature IS the authentication. Unverified payloads are refused before any
// state changes. Delivery is best-effort on both sides (24h retries); the
// poll+resolve rail stays the correctness path — webhooks only shorten the
// wait. Delivery never mints spend and never dispatches tasks.

// lensOpSessions maps Atlas operation IDs to Kubona sessions for deliveries
// that arrive before any poll. Best-effort (process lifetime); correctness
// never depends on it.
var lensOpSessions sync.Map // opID string -> sessionID string

// atlasJWKS is the cached JWKS document with its expiry.
var atlasJWKS struct {
	sync.Mutex
	doc     map[string]any
	expires time.Time
}

// atlasJWKSURL resolves the JWKS endpoint (WLT_JWKS_URL wins in tests).
func atlasJWKSURL() string {
	if u := os.Getenv("WLT_JWKS_URL"); u != "" {
		return u
	}
	return "https://api.atlas-beta.worldlabs.ai/.well-known/webhooks/jwks.json"
}

// fetchAtlasJWKS returns the cached JWKS document, refreshing past expiry.
func fetchAtlasJWKS() (map[string]any, error) {
	atlasJWKS.Lock()
	defer atlasJWKS.Unlock()
	if atlasJWKS.doc != nil && time.Now().Before(atlasJWKS.expires) {
		return atlasJWKS.doc, nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(atlasJWKSURL())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jwks rejected (%d)", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	atlasJWKS.doc = doc
	atlasJWKS.expires = time.Now().Add(24 * time.Hour)
	return doc, nil
}

// verifyAtlasWebhookSignature checks Ed25519 `v1a` signatures from the
// Standard Webhooks headers against every listed JWKS key (rotation-safe:
// any valid signature wins; callers refetch once on total failure).
func verifyAtlasWebhookSignature(jwksDoc map[string]any, headers http.Header, rawBody []byte) error {
	id := headers.Get("webhook-id")
	ts := headers.Get("webhook-timestamp")
	sigs := headers.Get("webhook-signature")
	if id == "" || ts == "" || sigs == "" {
		return fmt.Errorf("missing webhook signature headers")
	}
	sentAt, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	if err != nil {
		return fmt.Errorf("bad webhook timestamp")
	}
	if abs64(time.Now().Unix()-sentAt) > 300 {
		return fmt.Errorf("webhook timestamp outside 300s tolerance (replay risk)")
	}
	signed := fmt.Sprintf("%s.%s.", id, strings.TrimSpace(ts))
	signedBytes := append([]byte(signed), rawBody...)

	keys, _ := jwksDoc["keys"].([]any)
	for _, part := range strings.Fields(sigs) {
		if !strings.HasPrefix(part, "v1a,") {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(part, "v1a,"))
		if err != nil {
			continue
		}
		for _, k := range keys {
			km, _ := k.(map[string]any)
			if km["kty"] != "OKP" || km["crv"] != "Ed25519" {
				continue
			}
			xStr, _ := km["x"].(string)
			pub, err := base64.RawURLEncoding.DecodeString(xStr)
			if err != nil || len(pub) != ed25519.PublicKeySize {
				continue
			}
			if ed25519.Verify(ed25519.PublicKey(pub), signedBytes, sig) {
				return nil
			}
		}
	}
	return fmt.Errorf("no listed key verifies this delivery")
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// atlasWebhookEvent is the thin delivery envelope (no inputs, no results).
type atlasWebhookEvent struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	SchemaVersion string `json:"schemaVersion"`
	CreatedAt     string `json:"createdAt"`
	Data          struct {
		AccountID   string `json:"accountId"`
		ProjectID   string `json:"projectId"`
		OperationID string `json:"operationId"`
		Operation   string `json:"operation"`
		TaskName    string `json:"taskName"`
		State       string `json:"state"`
	} `json:"data"`
}

func (s *Server) handleObservatoryWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required for Atlas webhook delivery", nil)
		return
	}
	rawBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		WriteAPIError(w, "bad_request", "unreadable delivery body", nil)
		return
	}
	jwks, err := fetchAtlasJWKS()
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("cannot fetch webhook keys: %v", err), nil)
		return
	}
	if err := verifyAtlasWebhookSignature(jwks, r.Header, rawBody); err != nil {
		// One refetch before refusing: the key may have rotated.
		atlasJWKS.Lock()
		atlasJWKS.doc = nil
		atlasJWKS.expires = time.Time{}
		atlasJWKS.Unlock()
		if jwks2, err2 := fetchAtlasJWKS(); err2 == nil {
			if err = verifyAtlasWebhookSignature(jwks2, r.Header, rawBody); err == nil {
				jwks = jwks2
			}
		}
		if err != nil {
			WriteAPIError(w, "unauthorized", fmt.Sprintf("invalid webhook signature: %v", err), nil)
			return
		}
		_ = jwks
	}

	var evt atlasWebhookEvent
	if err := json.Unmarshal(rawBody, &evt); err != nil || evt.Data.OperationID == "" {
		WriteAPIError(w, "bad_request", "malformed webhook delivery", nil)
		return
	}
	if evt.Type != "task.succeeded" && evt.Type != "task.failed" {
		WriteAPIError(w, "bad_request", fmt.Sprintf("unknown webhook event %q", evt.Type), nil)
		return
	}

	// Attribute via the dispatch registry; unattributed deliveries are
	// accepted (2xx stops the retry clock) but resolve nothing — the poll
	// rail stays the correctness path.
	sessID, ok := lensOpSessions.Load(evt.Data.OperationID)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "attributed": false})
		return
	}
	record, err := s.resolveLensOperation(sessID.(string), evt.Data.OperationID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "attributed": true, "resolved": false, "error": err.Error()})
		return
	}
	record["attributed"] = true
	writeJSON(w, http.StatusOK, record)
}

// resolveLensOperation is the shared resolve path (handler + webhook).
func (s *Server) resolveLensOperation(sessionID, opID string) (map[string]any, error) {
	if !validAtlasOperationID(opID) {
		return nil, fmt.Errorf("invalid Atlas operation id %q", opID)
	}
	apiKey := os.Getenv("WLT_API_KEY")
	apiBase := os.Getenv("WLT_API_BASE_URL")
	if apiKey == "" {
		return nil, fmt.Errorf("World Labs Atlas API key (WLT_API_KEY) unconfigured on server")
	}
	resp, bodyBytes, err := fetchAtlasOperation(apiBase, apiKey, opID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, msg := mapAtlasUpstreamError(resp, bodyBytes)
		return nil, fmt.Errorf("%s: %s", code, msg)
	}
	var op struct {
		Done     bool `json:"done"`
		Error    any  `json:"error"`
		Response *struct {
			PromptUsed string `json:"promptUsed"`
			Frames     []struct {
				ImageAsset *struct {
					AssetID string `json:"assetId"`
					URL     string `json:"url"`
				} `json:"imageAsset"`
			} `json:"frames"`
		} `json:"response"`
	}
	if err := json.Unmarshal(bodyBytes, &op); err != nil {
		return nil, fmt.Errorf("failed to decode Atlas operation response")
	}
	if !op.Done {
		return map[string]any{"ok": true, "done": false}, nil
	}
	if op.Error != nil {
		return map[string]any{"ok": true, "done": true, "error": op.Error}, nil
	}
	if existing := findLensAnnotation(s.ledger, sessionID, opID, true); existing != nil {
		existing["cached"] = true
		return existing, nil
	}
	promptUsed := ""
	var imageURLs []string
	var assetIDs []string
	if op.Response != nil {
		promptUsed = op.Response.PromptUsed
		for _, f := range op.Response.Frames {
			if f.ImageAsset != nil && f.ImageAsset.URL != "" {
				imageURLs = append(imageURLs, f.ImageAsset.URL)
				assetIDs = append(assetIDs, f.ImageAsset.AssetID)
			}
		}
	}

	// Ephemeral purge: the dispatch annotation carries the flag. Download
	// outputs locally first (cloud copies purge ~1h after completion), then
	// explicitly DELETE the cloud output assets: the per-request purge flag
	// is the platform mechanism, the DELETE is defense in depth. Source
	// depth assets (user library inputs) are never touched — only the
	// frames this operation generated.
	var localPaths []string
	var purged []string
	if disp := findLensAnnotation(s.ledger, sessionID, opID, false); disp != nil {
		if purge, _ := disp["ephemeral_purge"].(bool); purge && len(imageURLs) > 0 {
			if dir, err := artifactsDir(); err == nil {
				opDir := filepath.Join(dir, opID)
				if err := os.MkdirAll(opDir, 0o755); err == nil {
					for i, u := range imageURLs {
						dst := filepath.Join(opDir, fmt.Sprintf("frame-%d.png", i))
						if dlErr := downloadFile(u, dst, 60*time.Second); dlErr == nil {
							localPaths = append(localPaths, dst)
						}
					}
				}
			}
			if len(localPaths) == len(imageURLs) {
				for _, aid := range assetIDs {
					if aid == "" {
						continue
					}
					if derr := deleteAtlasAsset(apiBase, apiKey, aid); derr == nil {
						purged = append(purged, aid)
					}
				}
			}
		}
	}
	record := map[string]any{
		"ok": true, "done": true,
		"operation_id": opID,
		"prompt_used":  promptUsed,
		"asset_urls":   imageURLs,
		"resolved":     true,
	}
	if len(localPaths) > 0 {
		record["local_paths"] = localPaths
		record["ephemeral_purged"] = true
	}
	if len(purged) > 0 {
		record["purged_assets"] = purged
	}
	_ = s.ledger.AppendEvent(sessionID, "annotation", map[string]any{
		"session_id":   sessionID,
		"text":         fmt.Sprintf("World Labs Atlas lens resolved: operation %s (%d frames)", opID, len(imageURLs)),
		"operation_id": opID,
		"prompt_used":  promptUsed,
		"resolved":     true,
	})
	return record, nil
}

// deleteAtlasAsset removes one project asset (ephemeral-purge defense in
// depth). A refusal is returned as error; callers keep the local copy and
// leave the cloud asset for the platform-side purge window.
func deleteAtlasAsset(apiBase, apiKey, assetID string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/assets/%s", atlasBaseURL(apiBase), assetID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("WLT-Api-Key", apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("asset delete rejected (%d)", resp.StatusCode)
	}
	return nil
}
