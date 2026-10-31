package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// ObservatoryLensRequest defines the parameter payload for requesting
// an illustrative 3D world lens from World Labs Atlas Marble 2.
type ObservatoryLensRequest struct {
	SessionID      string   `json:"session_id"`
	Prompt         string   `json:"prompt"`
	TargetSystem   string   `json:"target_system"`
	TaskType       string   `json:"task_type"` // "images2PosedRGBD" | "atlasTextToImage" | "atlasGenerate" | "atlasChisel"
	ImageURL       string   `json:"image_url,omitempty"`
	DepthAssetID   string   `json:"depth_asset_id,omitempty"` // chisel context depth (P1)
	VoxelSize      float64  `json:"voxel_size,omitempty"`     // chisel depth quantization, 0-0.4
	EphemeralPurge bool     `json:"ephemeral_purge,omitempty"`
	Camera         *LensCam `json:"camera,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	CachedView     bool     `json:"cached_view,omitempty"`
}

type LensCam struct {
	Position    []float64 `json:"position"`
	Orientation []float64 `json:"orientation"` // [x, y, z, w]
}

// ObservatoryLensResponse returns the queued operation or resolved asset.
type ObservatoryLensResponse struct {
	OK               bool    `json:"ok"`
	OperationID      string  `json:"operation_id,omitempty"`
	AssetID          string  `json:"asset_id,omitempty"`
	AssetURL         string  `json:"asset_url,omitempty"`
	TaskType         string  `json:"task_type"`
	CostUSD          float64 `json:"cost_usd"`
	Watermark        string  `json:"watermark"`
	Status           string  `json:"status"`
	IdempotencyKey   string  `json:"idempotency_key,omitempty"`
	Message          string  `json:"message,omitempty"`
	Cached           bool    `json:"cached,omitempty"`
	ProvenanceStatus string  `json:"provenance_status,omitempty"`
}

// observatoryTaskAllowlist is the closed task vocabulary. Unknown task
// types are refused BEFORE spend is reserved — a typo must never bill.
var observatoryTaskAllowlist = map[string]bool{
	"images2PosedRGBD": true,
	"atlasTextToImage": true,
	"atlasGenerate":    true,
	"atlasChisel":      true,
}

func (s *Server) handleObservatoryLens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleObservatoryLensStatus(w, r)
	case http.MethodPost:
		s.handleObservatoryLensDispatch(w, r)
	default:
		WriteAPIError(w, "method_not_allowed", "GET or POST required for observatory lens", nil)
	}
}

func (s *Server) handleObservatoryLensStatus(w http.ResponseWriter, r *http.Request) {
	opID := r.URL.Query().Get("operation_id")
	if opID == "" {
		WriteAPIError(w, "bad_request", "operation_id query parameter is required", nil)
		return
	}

	apiKey := os.Getenv("WLT_API_KEY")
	apiBase := os.Getenv("WLT_API_BASE_URL")
	if apiBase == "" {
		apiBase = "https://api.atlas-beta.worldlabs.ai"
	}

	if apiKey == "" {
		WriteAPIError(w, "llm_unavailable", "World Labs Atlas API key (WLT_API_KEY) unconfigured on server", nil)
		return
	}

	baseURL := strings.TrimRight(apiBase, "/")
	if !strings.HasSuffix(baseURL, "/api/v2") {
		baseURL = baseURL + "/api/v2"
	}

	reqURL := fmt.Sprintf("%s/operations/%s", baseURL, opID)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid operation request: %v", err), nil)
		return
	}
	req.Header.Set("WLT-Api-Key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("upstream Atlas error: %v", err), nil)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", "failed to read Atlas response", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(bodyBytes)
}

func (s *Server) handleObservatoryLensDispatch(w http.ResponseWriter, r *http.Request) {
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req ObservatoryLensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}

	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}

	// 1. Verify session existence + caller-vs-session tenant binding (T-09c)
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}

	// 1b. Q-39: Looking is free for cached views.
	// If the client requests a cached view (or references an existing depth asset
	// without requesting new generative prompt/geometry mutations), verify the
	// asset in this session's ledger history and return it for $0.00 without spend.
	taskType := req.TaskType
	if taskType == "" {
		taskType = "images2PosedRGBD"
	}
	if isCached, cachedOpID, cachedAssetID := isCachedLensRequest(s.ledger, &req); isCached {
		idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idemKey == "" {
			idemKey = strings.TrimSpace(req.IdempotencyKey)
		}
		if idemKey == "" {
			idemKey = "lens-cached-" + uuid.NewString()
		}
		_ = s.ledger.AppendEvent(req.SessionID, "annotation", map[string]any{
			"session_id":   req.SessionID,
			"text":         fmt.Sprintf("World Labs Atlas cached lens view reloaded ($0.00 billed, Q-39): operation %s", cachedOpID),
			"task_type":    taskType,
			"operation_id": cachedOpID,
			"asset_id":     cachedAssetID,
			"cost_usd":     0.0,
			"cached":       true,
		})
		writeJSON(w, http.StatusOK, ObservatoryLensResponse{
			OK:             true,
			OperationID:    cachedOpID,
			AssetID:        cachedAssetID,
			TaskType:       taskType,
			CostUSD:        0.0,
			Watermark:      "SYNTHETIC LENS — WORLD LABS ATLAS",
			Status:         "SUCCEEDED",
			IdempotencyKey: idemKey,
			Cached:         true,
		})
		return
	}

	// 1c. Atlas is an optional, metered lens. Production must opt in
	// explicitly; cached/ledger reads above stay available.
	if !atlasLensEnabled() {
		WriteAPIError(w, "feature_disabled", "optional Atlas lens disabled: set KUBONA_ATLAS_ENABLED=1 to opt in", nil)
		return
	}

	// 2. Sovereign Air-Gap Gate: Zero outbound cloud egress when active
	if s.llmSettings != nil && s.llmSettings.GetSnapshot().AirGapped {
		WriteAPIError(w, "forbidden", "Sovereign Air-Gap active: external cloud generative synthesis refused", nil)
		return
	}

	// 3. Task allowlist BEFORE spend: an unknown task_type is a client bug
	// and must never reserve budget. Empty defaults to images2PosedRGBD
	// (the console's only implemented lens path).
	taskType = req.TaskType
	if taskType == "" {
		taskType = "images2PosedRGBD"
	}
	if !observatoryTaskAllowlist[taskType] {
		WriteAPIError(w, "bad_request", fmt.Sprintf("unknown observatory task_type %q: use images2PosedRGBD, atlasTextToImage, atlasGenerate, or atlasChisel", req.TaskType), nil)
		return
	}

	// 4. Input validation BEFORE spend. images2PosedRGBD needs a real input
	// image: fail closed. No stock imagery is ever substituted — a
	// synthesized scene would masquerade as the operator's laboratory.
	if taskType == "images2PosedRGBD" && strings.TrimSpace(req.ImageURL) == "" {
		WriteAPIError(w, "bad_request", "image_required: images2PosedRGBD needs image_url (canvas snapshot or uploaded asset); no stock imagery is substituted", nil)
		return
	}

	// Voxel size is validated pre-spend: 0 (off) or within (0, 0.4].
	if req.VoxelSize < 0 || req.VoxelSize > 0.4 {
		WriteAPIError(w, "bad_request", "voxel_size must be 0 (off) or within (0, 0.4]", nil)
		return
	}

	// atlasGenerate needs full RGBD context (image + depth + camera per frame).
	// Fail-closed when context is missing (P2 requirement).
	if taskType == "atlasGenerate" && (strings.TrimSpace(req.ImageURL) == "" || strings.TrimSpace(req.DepthAssetID) == "") {
		WriteAPIError(w, "bad_request", "atlasGenerate needs full RGBD context frames (image_url and depth_asset_id required for multi-view conditioning)", nil)
		return
	}

	// 4b. Prompt egress hygiene (R-16): fail closed on secrets-shaped
	// content before it leaves the host. Narrow by construction: API keys,
	// private-key blocks, and cloud credential prefixes. Scientific prose
	// (including chemical names) passes untouched; the refusal names what
	// it blocked so legitimate prompts can be reworded.
	if bad := detectSecretsInPrompt(req.Prompt); bad != "" {
		WriteAPIError(w, "bad_request", fmt.Sprintf("prompt refused: looks like %s — remove secrets before cloud synthesis", bad), nil)
		return
	}

	// 5. Idempotency key: client header wins, then body field, then a
	// server UUID. Forwarded to Atlas so transport retries replay the same
	// operation instead of minting duplicate spend.
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idemKey == "" {
		idemKey = strings.TrimSpace(req.IdempotencyKey)
	}
	if idemKey == "" {
		idemKey = "lens-" + uuid.NewString()
	}

	// 6. Cost Governance: the card is the source of the per-dispatch cost.
	// A stale card refuses before any budget is reserved.
	if err := rates.Validate(time.Now().UTC()); err != nil {
		WriteAPIError(w, "rate_card_stale", "lens rate card is stale; update the runtime rate card before dispatching", nil)
		return
	}
	lensCost, ok := rates.LensCostUSD("atlas-lens")
	if !ok {
		WriteAPIError(w, "rate_card_stale", "lens rate is unavailable (no external vendor card in production or missing atlas-lens entry); refusing before spend", nil)
		return
	}
	if err := s.sessionMgr.ReserveSpend(req.SessionID, lensCost); err != nil {
		WriteAPIError(w, "ceiling_exceeded", fmt.Sprintf("spend limit exceeded: %v", err), nil)
		return
	}

	// 7. Resolve World Labs Atlas API Credentials
	apiKey := os.Getenv("WLT_API_KEY")
	apiBase := os.Getenv("WLT_API_BASE_URL")
	if apiBase == "" {
		apiBase = "https://api.atlas-beta.worldlabs.ai"
	}

	if apiKey == "" {
		// Refund spend and fail closed
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
		WriteAPIError(w, "llm_unavailable", "World Labs Atlas API key (WLT_API_KEY) unconfigured on server", nil)
		return
	}

	var atlasPayload map[string]any
	switch taskType {
	case "images2PosedRGBD":
		// ImageURL is guaranteed non-empty by pre-spend validation.
		// data: URLs (console canvas snapshots, Q-37) are uploaded as
		// project assets first — Atlas task frames reference assetIds,
		// never raw pixel bytes. Remote URLs pass through untouched.
		imageURL := strings.TrimSpace(req.ImageURL)
		imageAsset := map[string]any{"url": imageURL}
		if strings.HasPrefix(imageURL, "data:") {
			if inlineDecodedSize(imageURL) > inlineAssetMaxDecodedBytes {
				_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
				WriteAPIError(w, "bad_request", "inline snapshot exceeds 9.5 MiB decoded: use the asset upload flow instead", nil)
				return
			}
			assetID, err := uploadInlineAssetToAtlas(atlasBaseURL(apiBase), apiKey, idemKey+"-asset", imageURL, fmt.Sprintf("kubona-lens-%s.png", req.SessionID))
			if err != nil {
				_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
				WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("Atlas asset upload failed: %v", err), nil)
				return
			}
			imageAsset = map[string]any{"assetId": assetID}
		}
		atlasPayload = map[string]any{
			"frames": []map[string]any{
				{
					"imageAsset": imageAsset,
				},
			},
			"targetResolution": []int{1280, 720},
		}
	case "atlasTextToImage":
		prompt := req.Prompt
		if prompt == "" {
			prompt = fmt.Sprintf("Minimalist architectural subterranean observatory pavilion overlooking a crystalline %s quantum potential energy landscape, dark titanium trim, 6500K recessed lighting, floor-to-ceiling quartz observation windows", req.TargetSystem)
		}
		atlasPayload = map[string]any{
			"prompt":      prompt,
			"aspectRatio": "16:9",
		}
	default:
		// Handles atlasChisel and atlasGenerate (P2): prompt + target cameras,
		// depth-conditioned or full RGBD when image and depth assets are supplied.
		prompt := req.Prompt
		if prompt == "" {
			prompt = fmt.Sprintf("Minimalist architectural subterranean observatory pavilion overlooking a crystalline %s quantum potential energy landscape, dark titanium trim, 6500K recessed lighting, floor-to-ceiling quartz observation windows", req.TargetSystem)
		}
		camPos := []float64{0.0, 1.5, 3.5}
		camQuat := []float64{0.0, 0.0, 0.0, 1.0}
		if req.Camera != nil {
			if len(req.Camera.Position) == 3 {
				camPos = req.Camera.Position
			}
			if len(req.Camera.Orientation) == 4 {
				camQuat = req.Camera.Orientation
			}
		}

		// Live-schema PinholeCamera: camera-to-world extrinsics (XYZW
		// quaternion, rub = Three.js axes) + absolute-pixel intrinsics for
		// the 1280x720 generation grid. Defaults follow the docs' worked
		// example (fx=fy=900, centered principal point); P1 replaces these
		// with calibrated intrinsics from the depth-encoder rig.
		targetCam := map[string]any{
			"extrinsics": map[string]any{
				"position":         camPos,
				"quaternion":       camQuat,
				"coordinateSystem": "rub",
			},
			"intrinsics": map[string]any{
				"fx": 900, "fy": 900, "cx": 640, "cy": 360,
				"width": 1280, "height": 720,
			},
		}
		atlasPayload = map[string]any{
			"prompt":        prompt,
			"targetCameras": []map[string]any{targetCam},
		}
		if req.VoxelSize > 0 {
			atlasPayload["voxelSize"] = req.VoxelSize
		}
		if req.EphemeralPurge {
			atlasPayload["purgeContentOnOperationCompletion"] = true
		}

		var imageAsset map[string]any
		if imageURL := strings.TrimSpace(req.ImageURL); imageURL != "" {
			imageAsset = map[string]any{"url": imageURL}
			if strings.HasPrefix(imageURL, "data:") {
				if inlineDecodedSize(imageURL) > inlineAssetMaxDecodedBytes {
					_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
					WriteAPIError(w, "bad_request", "inline snapshot exceeds 9.5 MiB decoded: use the asset upload flow instead", nil)
					return
				}
				assetID, err := uploadInlineAssetToAtlas(atlasBaseURL(apiBase), apiKey, idemKey+"-asset", imageURL, fmt.Sprintf("kubona-lens-%s.png", req.SessionID))
				if err != nil {
					_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
					WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("Atlas asset upload failed: %v", err), nil)
					return
				}
				imageAsset = map[string]any{"assetId": assetID}
			}
		}

		depthID := strings.TrimSpace(req.DepthAssetID)
		if depthID != "" || imageAsset != nil {
			cf := map[string]any{
				"camera": targetCam,
			}
			if depthID != "" {
				cf["depth"] = map[string]any{
					"depthAsset": map[string]any{"assetId": depthID},
				}
			}
			if imageAsset != nil {
				cf["image"] = imageAsset
			}
			atlasPayload["contextFrames"] = []map[string]any{cf}
		}
	}

	payloadBytes, err := json.Marshal(atlasPayload)
	if err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
		WriteAPIError(w, "bad_request", fmt.Sprintf("failed to serialize Atlas payload: %v", err), nil)
		return
	}

	baseURL := atlasBaseURL(apiBase)
	reqURL := fmt.Sprintf("%s/tasks:%s", baseURL, taskType)
	httpReq, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
		WriteAPIError(w, "bad_request", fmt.Sprintf("failed to construct Atlas request: %v", err), nil)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("WLT-Api-Key", apiKey)
	httpReq.Header.Set("Idempotency-Key", idemKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("upstream Atlas error: %v", err), nil)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
		respBody, _ := io.ReadAll(resp.Body)
		code, msg := mapAtlasUpstreamError(resp, respBody)
		WriteAPIError(w, code, msg, map[string]any{"idempotency_key": idemKey})
		return
	}

	var opResult struct {
		OperationID string `json:"operation_id"`
		ID          string `json:"id"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&opResult); err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCost)
		WriteAPIError(w, "bridge_unavailable", "failed to decode Atlas operation response", nil)
		return
	}
	if opResult.OperationID == "" && opResult.ID != "" {
		opResult.OperationID = opResult.ID
	}
	// Attribution registry for webhook deliveries (best-effort).
	lensOpSessions.Store(opResult.OperationID, req.SessionID)

	// 8. Commit immutable provenance annotation to the Merkle ledger. A
	// ledger failure after dispatch is durably spooled; the response then
	// says provenance_status=pending instead of pretending the receipt is
	// sealed. If both the ledger and the spool fail, the dispatch is not
	// silently reported as fully recorded.
	sealed, provErr := s.ledger.AppendEventWithSpool(req.SessionID, "annotation", map[string]any{
		"session_id":      req.SessionID,
		"text":            fmt.Sprintf("World Labs Atlas lens dispatched (%s): operation %s ($%.2f reserved)", taskType, opResult.OperationID, lensCost),
		"task_type":       taskType,
		"operation_id":    opResult.OperationID,
		"prompt_used":     req.Prompt,
		"idempotency_key": idemKey,
		"cost_usd":        lensCost,
		"voxel_size":      req.VoxelSize,
		"ephemeral_purge": req.EphemeralPurge,
	})
	if provErr != nil {
		WriteAPIError(w, "provenance_unavailable",
			fmt.Sprintf("Atlas dispatch succeeded but provenance is not durable (operation %s): %v", opResult.OperationID, provErr),
			map[string]any{"operation_id": opResult.OperationID, "idempotency_key": idemKey})
		return
	}
	provenanceStatus := "sealed"
	if !sealed {
		provenanceStatus = "pending"
	}

	writeJSON(w, http.StatusOK, ObservatoryLensResponse{
		OK:               true,
		OperationID:      opResult.OperationID,
		TaskType:         taskType,
		CostUSD:          lensCost,
		Watermark:        "SYNTHETIC LENS — WORLD LABS ATLAS",
		Status:           opResult.Status,
		IdempotencyKey:   idemKey,
		ProvenanceStatus: provenanceStatus,
	})
}

// detectSecretsInPrompt scans free-text prompts for secrets-shaped content
// before cloud egress. Matches are anchored to high-entropy credential
// shapes (provider key prefixes with long random tails, PEM blocks), not
// to science vocabulary — "flask-shaped pavilion" must (and does) pass.
func detectSecretsInPrompt(prompt string) string {
	lower := strings.ToLower(prompt)
	for _, marker := range []struct {
		needle string
		label  string
	}{
		{"wl_live_", "a World Labs API key"},
		{"sk-live", "a Stripe live secret key"},
		{"sk_test", "a Stripe test secret key"},
		{"ghp_", "a GitHub personal access token"},
		{"gho_", "a GitHub OAuth token"},
		{"xoxb-", "a Slack bot token"},
		{"-----begin", "a private key block"},
	} {
		if strings.Contains(lower, marker.needle) {
			return marker.label
		}
	}
	if providerKeyRe.MatchString(prompt) {
		return "a provider secret key"
	}
	if awsKeyRe.MatchString(prompt) {
		return "an AWS access key id"
	}
	return ""
}

var (
	// sk-<16+ alnum>: OpenAI-style keys. The length floor keeps words
	// like "flask-" / "mask-" / "task-" out of the match.
	providerKeyRe = regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`)
	// AKIA + 16 uppercase alphanumerics.
	awsKeyRe = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
)

// mapAtlasUpstreamError branches on status + the machine-readable `code`
// field (docs: Errors and retries) — never on message wording. Callers
// refund spend on every path; codes are catalogued in
// platform/contracts/error-catalog.csv.
func mapAtlasUpstreamError(resp *http.Response, body []byte) (string, string) {
	var parsed struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &parsed)
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) > 280 {
		trimmed = trimmed[:280] + "…"
	}

	switch resp.StatusCode {
	case http.StatusPaymentRequired: // 402
		switch parsed.Code {
		case "SPEND_LIMIT_EXCEEDED":
			return "spend_limit_exceeded", fmt.Sprintf("Atlas project/account spend limit reached: %s", firstNonEmpty(parsed.Detail, trimmed))
		default: // CREDITS_UNAVAILABLE or unmarked
			return "credits_unavailable", fmt.Sprintf("Atlas account out of credits: %s", firstNonEmpty(parsed.Detail, trimmed))
		}
	case http.StatusNotFound: // 404: task not entitled on this account
		return "task_not_entitled", "Atlas task not enabled on this account (HTTP 404): request beta access or check the project key"
	case http.StatusTooManyRequests: // 429
		retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After"))
		if retryAfter == "" {
			return "rate_limited", "Atlas rate limit hit: back off with jitter, then retry the same idempotency key"
		}
		return "rate_limited", fmt.Sprintf("Atlas rate limit hit: retry after %s seconds with the same idempotency key", retryAfter)
	default:
		return "bridge_unavailable", fmt.Sprintf("Atlas rejected request (%d): %s", resp.StatusCode, firstNonEmpty(parsed.Detail, trimmed))
	}
}

// atlasBaseURL normalizes the configured root to the /api/v2 Developer API.
// validAtlasOperationID bounds the id before it is interpolated into an
// upstream URL and a filesystem path. Atlas ids are opaque alphanumerics.
func validAtlasOperationID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func atlasBaseURL(apiBase string) string {
	baseURL := strings.TrimRight(apiBase, "/")
	if !strings.HasSuffix(baseURL, "/api/v2") {
		baseURL = baseURL + "/api/v2"
	}
	return baseURL
}

// uploadInlineAssetToAtlas stores <10 MiB inline media (canvas snapshots,
// depth PNGs) as a project asset and returns its assetId. The asset call
// carries a derived idempotency key: replaying the same body with the same
// task key must never collide with a different body (409).
func uploadInlineAssetToAtlas(baseURL, apiKey, idemKey, dataURL, displayName string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"asset": map[string]any{
			"displayName": displayName,
			"base64":      dataURL,
		},
	})
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+"/assets", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("WLT-Api-Key", apiKey)
	httpReq.Header.Set("Idempotency-Key", idemKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("asset create rejected (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var created struct {
		Name    string `json:"name"`
		AssetID string `json:"assetId"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &created); err != nil {
		return "", err
	}
	// Live contract returns {"id": "asset_..."}; accept the tutorial's
	// resource-name shape too.
	if created.AssetID != "" {
		return created.AssetID, nil
	}
	if created.ID != "" {
		return created.ID, nil
	}
	parts := strings.Split(strings.TrimSuffix(created.Name, "/"), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", fmt.Errorf("asset create returned no id")
	}
	return parts[len(parts)-1], nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// isCachedLensRequest checks if the request targets an existing resolved lens
// or uploaded depth asset in this session without requesting fresh generative
// cloud synthesis (Q-39 ruling: looking is free, probing is costly).
func isCachedLensRequest(l *ledger.Ledger, req *ObservatoryLensRequest) (bool, string, string) {
	if req == nil || req.SessionID == "" {
		return false, "", ""
	}
	// Explicit cached_view flag or viewing an existing depth asset with no fresh prompt
	if !req.CachedView && (strings.TrimSpace(req.DepthAssetID) == "" || strings.TrimSpace(req.Prompt) != "") {
		return false, "", ""
	}
	events, err := l.GetRecentEvents(req.SessionID, 500)
	if err != nil {
		return false, "", ""
	}
	targetDepth := strings.TrimSpace(req.DepthAssetID)
	for _, e := range events {
		if e.EventType != "annotation" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		opID, _ := p["operation_id"].(string)
		assetID, _ := p["asset_id"].(string)

		// Check if DepthAssetID matches a resolved asset in this session
		if targetDepth != "" && (assetID == targetDepth || strings.Contains(fmt.Sprint(p["asset_urls"]), targetDepth)) {
			return true, opID, assetID
		}
		// If explicit CachedView requested, return the latest resolved lens operation
		if req.CachedView {
			if r, _ := p["resolved"].(bool); r && opID != "" {
				return true, opID, assetID
			}
			if req.IdempotencyKey != "" {
				if idem, _ := p["idempotency_key"].(string); idem == req.IdempotencyKey && opID != "" {
					return true, opID, assetID
				}
			}
		}
	}
	return false, "", ""
}

// handleObservatoryLensImage proxies an Atlas image through the daemon so the
// browser never needs a remote img-src exception. The URL must be HTTPS on a
// World Labs host; redirects are re-validated and the body is size-capped.
func (s *Server) handleObservatoryLensImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required for lens image proxy", nil)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		WriteAPIError(w, "bad_request", "url query parameter is required", nil)
		return
	}
	u, err := url.Parse(raw)
	if err != nil || !allowedAtlasImageURL(u) {
		WriteAPIError(w, "bad_request", "lens image URL must be HTTPS on a worldlabs.ai host or the Atlas output bucket", nil)
		return
	}
	client := &http.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return fmt.Errorf("too many redirects")
			}
			if !allowedAtlasImageURL(req.URL) {
				return fmt.Errorf("redirected outside World Labs HTTPS hosts")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		WriteAPIError(w, "bad_request", "invalid lens image request", nil)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", "Atlas image fetch failed: "+err.Error(), nil)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("Atlas image returned HTTP %d", resp.StatusCode), nil)
		return
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "image/") {
		WriteAPIError(w, "bad_gateway", "Atlas image response is not an image", nil)
		return
	}
	const maxBytes = 20 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", "failed to read Atlas image", nil)
		return
	}
	if len(body) > maxBytes {
		WriteAPIError(w, "bad_gateway", "Atlas image exceeds 20 MiB", nil)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func allowedAtlasImageHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "worldlabs.ai" || strings.HasSuffix(host, ".worldlabs.ai")
}

// Atlas returns generated frames as signed URLs in its GCS output bucket,
// not on a worldlabs.ai host. Allow only that bucket path, never all of GCS.
func allowedAtlasImageURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" {
		return false
	}
	if allowedAtlasImageHost(u.Hostname()) {
		return true
	}
	return strings.EqualFold(u.Hostname(), "storage.googleapis.com") &&
		strings.HasPrefix(u.EscapedPath(), "/wlt-marble-outputs-prod/")
}
