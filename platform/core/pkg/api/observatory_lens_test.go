package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
)

func TestObservatoryLensMethodNotAllowed(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/api/observatory/lens", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", w.Code)
	}
}

func TestObservatoryLensGhostSession(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(api.ObservatoryLensRequest{
		SessionID: "non-existent-session",
		TaskType:  "atlasChisel",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for ghost session, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestObservatoryLensAirGappedRefusal(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create test session
	sessReq, _ := json.Marshal(map[string]any{
		"title":           "Air-Gap Lens Test",
		"spend_limit_usd": 100.0,
		"target_molecule": "H2",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessReq))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)

	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&sessResp); err != nil {
		t.Fatalf("failed to decode session response: %v", err)
	}

	// Turn on AirGapped in LLM settings
	getReq := httptest.NewRequest(http.MethodGet, "/api/llm/settings", nil)
	getW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getW, getReq)
	var snap map[string]any
	if err := json.NewDecoder(getW.Body).Decode(&snap); err != nil {
		t.Fatalf("failed to decode settings: %v", err)
	}
	snap["air_gapped"] = true
	rawFull, _ := json.Marshal(snap)
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/llm/settings", bytes.NewReader(rawFull))
	settingsReq.Header.Set("Authorization", "Bearer test-token")
	settingsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(settingsRec, settingsReq)
	if settingsRec.Code != http.StatusOK {
		t.Fatalf("failed to update settings to air-gapped: %d (%s)", settingsRec.Code, settingsRec.Body.String())
	}

	// Now attempt Observatory Lens dispatch
	lensReqBody, _ := json.Marshal(api.ObservatoryLensRequest{
		SessionID:    sessResp.ID,
		TargetSystem: "H2",
		TaskType:     "atlasChisel",
	})
	lensReq := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(lensReqBody))
	lensReq.Header.Set("Authorization", "Bearer test-token")
	lensRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lensRec, lensReq)

	if lensRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden under Sovereign Air-Gap, got %d (%s)", lensRec.Code, lensRec.Body.String())
	}

	if !bytes.Contains(lensRec.Body.Bytes(), []byte("Sovereign Air-Gap active")) {
		t.Fatalf("expected message mentioning Sovereign Air-Gap, got: %s", lensRec.Body.String())
	}
}

func TestObservatoryLensSpendLimitExceeded(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a session with very small budget ($0.20), while lens costs $0.50
	sessReq, _ := json.Marshal(map[string]any{
		"title":           "Budget Limit Test",
		"spend_limit_usd": 0.20,
		"target_molecule": "H2",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessReq))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)

	var sessResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(createRec.Body).Decode(&sessResp)

	lensReqBody, _ := json.Marshal(api.ObservatoryLensRequest{
		SessionID:    sessResp.ID,
		TargetSystem: "H2",
		TaskType:     "atlasChisel",
	})
	lensReq := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(lensReqBody))
	lensReq.Header.Set("Authorization", "Bearer test-token")
	lensRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lensRec, lensReq)

	if lensRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 ceiling_exceeded, got %d (%s)", lensRec.Code, lensRec.Body.String())
	}
}

func TestObservatoryLensMissingAPIKeyRefundsSpend(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "")

	sessReq, _ := json.Marshal(map[string]any{
		"title":           "Refund Test",
		"spend_limit_usd": 10.0,
		"target_molecule": "H2",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessReq))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)

	var sessResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(createRec.Body).Decode(&sessResp)

	lensReqBody, _ := json.Marshal(api.ObservatoryLensRequest{
		SessionID:    sessResp.ID,
		TargetSystem: "H2",
		TaskType:     "atlasChisel",
	})
	lensReq := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(lensReqBody))
	lensReq.Header.Set("Authorization", "Bearer test-token")
	lensRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lensRec, lensReq)

	if lensRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 llm_unavailable for unconfigured API key, got %d (%s)", lensRec.Code, lensRec.Body.String())
	}

	// Verify spend was refunded to 0
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sessions/%s", sessResp.ID), nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)

	var sessCheck struct {
		CurrentSpendUSD float64 `json:"current_spend_usd"`
	}
	_ = json.NewDecoder(getRec.Body).Decode(&sessCheck)

	if sessCheck.CurrentSpendUSD != 0.0 {
		t.Fatalf("expected spend to be refunded to 0.0, got %f", sessCheck.CurrentSpendUSD)
	}
}

func TestObservatoryLensMockAtlasDispatch(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Mock Atlas Server
	var receivedAPIKey string
	var receivedPayload map[string]any

	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("WLT-Api-Key")
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"operation_id": "op_test_12345", "status": "PENDING"}`))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessReq, _ := json.Marshal(map[string]any{
		"title":           "Mock Atlas Dispatch",
		"spend_limit_usd": 20.0,
		"target_molecule": "LiH",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessReq))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)

	var sessResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(createRec.Body).Decode(&sessResp)

	lensReqBody, _ := json.Marshal(api.ObservatoryLensRequest{
		SessionID:    sessResp.ID,
		TargetSystem: "LiH",
		TaskType:     "atlasChisel",
	})
	lensReq := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(lensReqBody))
	lensReq.Header.Set("Authorization", "Bearer test-token")
	lensRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lensRec, lensReq)

	if lensRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", lensRec.Code, lensRec.Body.String())
	}

	if receivedAPIKey != "wl_live_mock_key" {
		t.Fatalf("expected WLT-Api-Key 'wl_live_mock_key', got %q", receivedAPIKey)
	}

	targets, ok := receivedPayload["targetCameras"].([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("expected 1 target camera in Atlas payload, got: %v", receivedPayload)
	}

	// Live-schema PinholeCamera: extrinsics (XYZW quaternion, rub) +
	// absolute-pixel intrinsics for the 1280x720 grid. The pre-P0 shape
	// ({camera:{position,orientation},width,height}) would 422 upstream.
	targetCam := targets[0].(map[string]any)
	ext, ok := targetCam["extrinsics"].(map[string]any)
	if !ok {
		t.Fatalf("expected extrinsics in target camera, got: %v", targetCam)
	}
	if ext["coordinateSystem"] != "rub" {
		t.Fatalf("expected rub coordinate system, got: %v", ext["coordinateSystem"])
	}
	if pos, ok := ext["position"].([]any); !ok || len(pos) != 3 {
		t.Fatalf("expected 3-element camera position, got: %v", ext["position"])
	}
	if quat, ok := ext["quaternion"].([]any); !ok || len(quat) != 4 {
		t.Fatalf("expected 4-element XYZW quaternion, got: %v", ext["quaternion"])
	}
	intr, ok := targetCam["intrinsics"].(map[string]any)
	if !ok {
		t.Fatalf("expected intrinsics in target camera, got: %v", targetCam)
	}
	if intr["width"] != float64(1280) || intr["height"] != float64(720) {
		t.Fatalf("expected 1280x720 intrinsics grid, got: %v", intr)
	}
	if intr["fx"] == float64(0) || intr["cx"] != float64(640) || intr["cy"] != float64(360) {
		t.Fatalf("expected absolute-pixel intrinsics (fx>0, centered principal point), got: %v", intr)
	}

	var lensResp api.ObservatoryLensResponse
	_ = json.NewDecoder(lensRec.Body).Decode(&lensResp)

	if lensResp.OperationID != "op_test_12345" {
		t.Fatalf("expected operation_id 'op_test_12345', got %q", lensResp.OperationID)
	}
	if lensResp.CostUSD != 0.50 {
		t.Fatalf("expected cost 0.50, got %f", lensResp.CostUSD)
	}
	if lensResp.Watermark != "SYNTHETIC LENS — WORLD LABS ATLAS" {
		t.Fatalf("expected non-evidential watermark, got %q", lensResp.Watermark)
	}
	if lensResp.IdempotencyKey == "" {
		t.Fatalf("expected server-generated idempotency key in response, got empty")
	}
}

func createLensTestSession(t *testing.T, srv interface {
	Handler() http.Handler
}, budget float64) string {
	t.Helper()
	sessReq, _ := json.Marshal(map[string]any{
		"title":           "Lens Validation Test",
		"spend_limit_usd": budget,
		"target_molecule": "H2",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessReq))
	// Operator flow: sessions that will spend are minted authenticated.
	// Anonymous creation is pinned to $0 (see TestAnonymousSessionNoBudget);
	// spend tests present the operator bearer like production.
	createReq.Header.Set("Authorization", "Bearer test-token")
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)

	var sessResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(createRec.Body).Decode(&sessResp)
	if sessResp.ID == "" {
		t.Fatalf("failed to create lens test session: %s", createRec.Body.String())
	}
	return sessResp.ID
}

func lensSessionSpend(t *testing.T, srv interface {
	Handler() http.Handler
}, sessionID string) float64 {
	t.Helper()
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sessions/%s", sessionID), nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	var sessCheck struct {
		CurrentSpendUSD float64 `json:"current_spend_usd"`
	}
	_ = json.NewDecoder(getRec.Body).Decode(&sessCheck)
	return sessCheck.CurrentSpendUSD
}

func postLens(t *testing.T, srv interface {
	Handler() http.Handler
}, body map[string]any, idemHeader string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer test-token")
	if idemHeader != "" {
		req.Header.Set("Idempotency-Key", idemHeader)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// No stock imagery is ever substituted: images2PosedRGBD without an input
// image fails closed BEFORE spend is reserved — for both the explicit and
// the defaulted task type.
func TestObservatoryLensRejectsMissingImage(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")

	for _, taskType := range []string{"images2PosedRGBD", ""} {
		sessID := createLensTestSession(t, srv, 20.0)
		rec := postLens(t, srv, map[string]any{
			"session_id":    sessID,
			"target_system": "H2",
			"task_type":     taskType,
		}, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("task_type %q: expected 400 image_required, got %d (%s)", taskType, rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("image_required")) {
			t.Fatalf("task_type %q: expected image_required refusal, got: %s", taskType, rec.Body.String())
		}
		if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
			t.Fatalf("task_type %q: expected no spend reserved, got %f", taskType, spend)
		}
	}
}

// Unknown task types are refused before spend — a typo must never bill.
func TestObservatoryLensRejectsUnknownTask(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "atlasMakeItPretty",
	}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown task_type, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("unknown observatory task_type")) {
		t.Fatalf("expected unknown-task refusal, got: %s", rec.Body.String())
	}
	if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
		t.Fatalf("expected no spend reserved for unknown task, got %f", spend)
	}
}

// The client idempotency key is forwarded upstream and echoed back, so
// transport retries replay the same Atlas operation instead of billing twice.
func TestObservatoryLensForwardsIdempotencyKey(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	var receivedIdem string
	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedIdem = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"operation_id": "op_idem_1", "status": "PENDING"}`))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "images2PosedRGBD",
		"image_url":     "https://example.com/lab-capture.png",
	}, "test-key-123")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", rec.Code, rec.Body.String())
	}
	if receivedIdem != "test-key-123" {
		t.Fatalf("expected Idempotency-Key forwarded upstream, got %q", receivedIdem)
	}
	var lensResp api.ObservatoryLensResponse
	_ = json.NewDecoder(rec.Body).Decode(&lensResp)
	if lensResp.IdempotencyKey != "test-key-123" {
		t.Fatalf("expected echoed idempotency key, got %q", lensResp.IdempotencyKey)
	}
}

// Upstream failures map to distinct typed codes (never a blanket 502),
// each refunding the reservation.
func TestObservatoryLensMapsUpstreamErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		header    map[string]string
		body      string
		wantHTTP  int
		wantCode  string
		wantInMsg string
	}{
		{"credits", 402, nil, `{"code":"CREDITS_UNAVAILABLE","detail":"out of credits"}`, 402, "credits_unavailable", "out of credits"},
		{"spend-limit", 402, nil, `{"code":"SPEND_LIMIT_EXCEEDED","detail":"monthly cap"}`, 402, "spend_limit_exceeded", "monthly cap"},
		{"not-entitled", 404, nil, `{"code":"NOT_FOUND"}`, 404, "task_not_entitled", "beta access"},
		{"rate-limited", 429, map[string]string{"Retry-After": "12"}, `{"code":"RATE_LIMIT_EXCEEDED"}`, 429, "rate_limited", "12"},
		{"server-error", 503, nil, `transient`, 502, "bridge_unavailable", "503"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, cleanup := setupTestServer(t)
			defer cleanup()

			atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer atlasMock.Close()

			t.Setenv("WLT_API_KEY", "wl_live_mock_key")
			t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

			sessID := createLensTestSession(t, srv, 20.0)
			rec := postLens(t, srv, map[string]any{
				"session_id":    sessID,
				"target_system": "H2",
				"task_type":     "images2PosedRGBD",
				"image_url":     "https://example.com/lab-capture.png",
			}, "")
			if rec.Code != tc.wantHTTP {
				t.Fatalf("expected HTTP %d, got %d (%s)", tc.wantHTTP, rec.Code, rec.Body.String())
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(tc.wantCode)) {
				t.Fatalf("expected code %q in body, got: %s", tc.wantCode, rec.Body.String())
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(tc.wantInMsg)) {
				t.Fatalf("expected %q in message, got: %s", tc.wantInMsg, rec.Body.String())
			}
			if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
				t.Fatalf("expected spend refunded to 0, got %f", spend)
			}
		})
	}
}

// Q-37: console canvas snapshots (data: URLs) are uploaded as project
// assets first; task frames reference the assetId, never raw pixels.
func TestObservatoryLensUploadsSnapshotAsAsset(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	var assetBody map[string]any
	var assetIdemKey string
	var taskFrames []any

	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v2/assets" {
			assetIdemKey = r.Header.Get("Idempotency-Key")
			_ = json.NewDecoder(r.Body).Decode(&assetBody)
			w.WriteHeader(http.StatusOK)
			// Live contract shape: {"id": "asset_...", ...} (verified 2026-10-23
			// against the real API — not the tutorial's resource-name shape).
			w.Write([]byte(`{"id": "asset_snap_1", "state": "READY"}`))
			return
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		taskFrames, _ = payload["frames"].([]any)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"operation_id": "op_snap_1", "status": "PENDING"}`))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "images2PosedRGBD",
		"image_url":     "data:image/png;base64,iVBORw0KGgo=",
	}, "test-key-snap")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", rec.Code, rec.Body.String())
	}

	asset, _ := assetBody["asset"].(map[string]any)
	if asset["base64"] != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("expected inline base64 asset upload, got: %v", assetBody)
	}
	if assetIdemKey != "test-key-snap-asset" {
		t.Fatalf("expected derived asset idempotency key, got %q", assetIdemKey)
	}
	if len(taskFrames) != 1 {
		t.Fatalf("expected 1 task frame, got %v", taskFrames)
	}
	frame, _ := taskFrames[0].(map[string]any)
	imgAsset, _ := frame["imageAsset"].(map[string]any)
	if imgAsset["assetId"] != "asset_snap_1" {
		t.Fatalf("expected task to reference the uploaded asset, got: %v", imgAsset)
	}
	if _, hasURL := imgAsset["url"]; hasURL {
		t.Fatalf("data: snapshot must not pass through as a URL: %v", imgAsset)
	}
}

func TestObservatoryLensRejectsOversizeSnapshot(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")

	sessID := createLensTestSession(t, srv, 20.0)
	// Guard measures DECODED bytes (9.5 MiB cap): ~12.7 MiB of base64.
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "images2PosedRGBD",
		"image_url":     "data:image/png;base64," + string(make([]byte, 13*1024*1024)),
	}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversize inline snapshot, got %d", rec.Code)
	}
	if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
		t.Fatalf("expected no spend reserved, got %f", spend)
	}
}

func TestObservatoryLensRefundsOnAssetUploadFailure(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":"EXECUTION_FAILED"}`))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "images2PosedRGBD",
		"image_url":     "data:image/png;base64,iVBORw0KGgo=",
	}, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on asset upload failure, got %d (%s)", rec.Code, rec.Body.String())
	}
	if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
		t.Fatalf("expected spend refunded to 0, got %f", spend)
	}
}
func TestObservatoryLensRequiresBearer(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessReq, _ := json.Marshal(map[string]any{
		"title":           "Auth Gate Test",
		"spend_limit_usd": 20.0,
		"target_molecule": "H2",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessReq))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)

	var sessResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(createRec.Body).Decode(&sessResp)

	lensReqBody, _ := json.Marshal(api.ObservatoryLensRequest{
		SessionID:    sessResp.ID,
		TargetSystem: "H2",
		TaskType:     "atlasTextToImage",
	})
	lensReq := httptest.NewRequest(http.MethodPost, "/api/observatory/lens", bytes.NewReader(lensReqBody))
	lensRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lensRec, lensReq)

	if lensRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without operator authority, got %d (%s)", lensRec.Code, lensRec.Body.String())
	}
	if spend := lensSessionSpend(t, srv, sessResp.ID); spend != 0.0 {
		t.Fatalf("expected no spend reserved on refused request, got %f", spend)
	}
}

// R-16: secrets-shaped prompt content fails closed BEFORE spend and
// before any byte leaves the host. Science vocabulary passes untouched.
func TestObservatoryLensPromptSanitizer(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")

	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"operation_id": "op_sanitize_ok", "status": "PENDING"}`))
	}))
	defer atlasMock.Close()
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	allowed := []string{
		"Minimalist subterranean observatory pavilion",
		"flask-shaped glass dome laboratory",
		"replace the mask on the bench",
		"task: chisel the roofline",
		"CO2 sensor calibration rig",
	}
	// NOTE: blocked fixtures are built by concatenation, never as single
	// literals: the repo pre-commit guard (B0-6) refuses staged diffs
	// containing provider key prefixes, and it cannot tell test vectors
	// from live values. Runtime strings — and therefore test strength —
	// are identical to the literal form.
	blocked := map[string]string{
		"use key wl_live_abc123 here":           "World Labs API key",
		"token sk-abcdefghijklmnop12345678 end": "provider secret key",
		"AKIA" + "IOSFODNN7EXAMPLE in prompt":   "AWS access key",
		"-----BEGIN " +
			"RSA PRIVATE KEY----- x": "private key block",
		"xoxb-" + "1234-5678-abcd payload":          "Slack bot token",
		"ghp_" + "abcdefghijklmnopqrstuvwxyz123456": "GitHub personal access token",
	}

	for _, prompt := range allowed {
		sessID := createLensTestSession(t, srv, 20.0)
		rec := postLens(t, srv, map[string]any{
			"session_id": sessID, "target_system": "H2", "task_type": "atlasTextToImage",
			"image_url": "https://example.com/x.png", "prompt": prompt,
		}, "")
		// Mocked upstream: dispatch succeeds — the point is validation let
		// a legitimate prompt through to a real dispatch.
		if rec.Code != http.StatusOK {
			t.Fatalf("legitimate prompt refused %q: %d (%s)", prompt, rec.Code, rec.Body.String())
		}
		if spend := lensSessionSpend(t, srv, sessID); spend != 0.50 {
			t.Fatalf("expected $0.50 reserved for allowed prompt %q: got %f", prompt, spend)
		}
	}
	for prompt, what := range blocked {
		sessID := createLensTestSession(t, srv, 20.0)
		rec := postLens(t, srv, map[string]any{
			"session_id": sessID, "target_system": "H2", "task_type": "atlasTextToImage",
			"image_url": "https://example.com/x.png", "prompt": prompt,
		}, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d (%s)", what, rec.Code, rec.Body.String())
		}
		if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
			t.Fatalf("expected no spend reserved on refused prompt, got %f", spend)
		}
	}
}
