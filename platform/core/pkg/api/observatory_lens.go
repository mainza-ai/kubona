package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
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
}

type LensCam struct {
	Position    []float64 `json:"position"`
	Orientation []float64 `json:"orientation"` // [x, y, z, w]
}

// ObservatoryLensResponse returns the queued operation or resolved asset.
type ObservatoryLensResponse struct {
	OK             bool    `json:"ok"`
	OperationID    string  `json:"operation_id,omitempty"`
	AssetID        string  `json:"asset_id,omitempty"`
	AssetURL       string  `json:"asset_url,omitempty"`
	TaskType       string  `json:"task_type"`
	CostUSD        float64 `json:"cost_usd"`
	Watermark      string  `json:"watermark"`
	Status         string  `json:"status"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
	Message        string  `json:"message,omitempty"`
}

// lensCostUSD is the per-dispatch spend reservation. Single named const —
// never a second copy. TODO(P1): reconcile against the live Atlas rate card
// (billing dashboard per-task burn); the beta card can change and must not
// be embedded in client code.
const lensCostUSD = 0.50

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
	var req ObservatoryLensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}

	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}

	// 1. Verify session existence
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
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
	taskType := req.TaskType
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

	// atlasGenerate needs full RGBD context (image + depth + camera per
	// frame); P1 builds depth-only chisel context. Refuse loudly rather
	// than dispatching a payload the API would 422.
	if taskType == "atlasGenerate" {
		WriteAPIError(w, "bad_request", "atlasGenerate needs full RGBD context frames (image + depth + camera) — P2 work; chisel runs prompt-only or depth-conditioned today", nil)
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

	// 6. Cost Governance: 500 Atlas credits = $0.50 USD
	if err := s.sessionMgr.ReserveSpend(req.SessionID, lensCostUSD); err != nil {
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
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
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
				_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
				WriteAPIError(w, "bad_request", "inline snapshot exceeds 9.5 MiB decoded: use the asset upload flow instead", nil)
				return
			}
			assetID, err := uploadInlineAssetToAtlas(atlasBaseURL(apiBase), apiKey, idemKey+"-asset", imageURL, fmt.Sprintf("kubona-lens-%s.png", req.SessionID))
			if err != nil {
				_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
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
		// Reachable only for atlasChisel (atlasGenerate is refused
		// pre-spend above): prompt + target cameras, depth-conditioned when
		// a depth asset is supplied, prompt-only otherwise.
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
		if depthID := strings.TrimSpace(req.DepthAssetID); depthID != "" {
			// Depth-only context, 1:1 with the target: the encoder renders
			// depth AT this camera, so context and target share the pose.
			atlasPayload["contextFrames"] = []map[string]any{
				{
					"camera": targetCam,
					"depth": map[string]any{
						"depthAsset": map[string]any{"assetId": depthID},
					},
				},
			}
		}
	}

	payloadBytes, err := json.Marshal(atlasPayload)
	if err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
		WriteAPIError(w, "bad_request", fmt.Sprintf("failed to serialize Atlas payload: %v", err), nil)
		return
	}

	baseURL := atlasBaseURL(apiBase)
	reqURL := fmt.Sprintf("%s/tasks:%s", baseURL, taskType)
	httpReq, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
		WriteAPIError(w, "bad_request", fmt.Sprintf("failed to construct Atlas request: %v", err), nil)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("WLT-Api-Key", apiKey)
	httpReq.Header.Set("Idempotency-Key", idemKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("upstream Atlas error: %v", err), nil)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
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
		_ = s.sessionMgr.ReleaseSpend(req.SessionID, lensCostUSD)
		WriteAPIError(w, "bridge_unavailable", "failed to decode Atlas operation response", nil)
		return
	}
	if opResult.OperationID == "" && opResult.ID != "" {
		opResult.OperationID = opResult.ID
	}
	// Attribution registry for webhook deliveries (best-effort).
	lensOpSessions.Store(opResult.OperationID, req.SessionID)

	// 8. Commit immutable provenance annotation to the Merkle ledger.
	// Structured fields (task, operation, idempotency key) ride alongside
	// the human-readable text so the dispatch is resumable and auditable
	// without re-querying Atlas.
	_ = s.ledger.AppendEvent(req.SessionID, "annotation", map[string]any{
		"session_id":      req.SessionID,
		"text":            fmt.Sprintf("World Labs Atlas lens dispatched (%s): operation %s ($0.50 reserved)", taskType, opResult.OperationID),
		"task_type":       taskType,
		"operation_id":    opResult.OperationID,
		"idempotency_key": idemKey,
		"cost_usd":        lensCostUSD,
		"voxel_size":      req.VoxelSize,
		"ephemeral_purge": req.EphemeralPurge,
	})

	writeJSON(w, http.StatusOK, ObservatoryLensResponse{
		OK:             true,
		OperationID:    opResult.OperationID,
		TaskType:       taskType,
		CostUSD:        lensCostUSD,
		Watermark:      "SYNTHETIC LENS — WORLD LABS ATLAS",
		Status:         opResult.Status,
		IdempotencyKey: idemKey,
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
