package api_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func postObservatory(t *testing.T, srv interface {
	Handler() http.Handler
}, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestObservatoryAssetsUpload(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	var gotBody map[string]any
	var gotIdem string
	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": "asset_test_1", "state": "READY"}`))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 10.0)
	rec := postObservatory(t, srv, "/api/observatory/assets", map[string]any{
		"session_id":   sessID,
		"data_base64":  "data:image/png;base64,iVBORw0KGgo=",
		"display_name": "probe.png",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		AssetID string `json:"asset_id"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if out.AssetID != "asset_test_1" {
		t.Fatalf("expected live-shape asset id, got %q", out.AssetID)
	}
	asset, _ := gotBody["asset"].(map[string]any)
	if asset["base64"] != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("expected inline base64 forwarded, got: %v", gotBody)
	}
	if gotIdem == "" {
		t.Fatalf("expected idempotency key on asset create")
	}

	// Non-data URLs are refused: task frames reference assets or public
	// HTTPS, never raw bytes smuggled through the upload rail.
	rec = postObservatory(t, srv, "/api/observatory/assets", map[string]any{
		"session_id":  sessID,
		"data_base64": "https://example.com/x.png",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-data URL, got %d", rec.Code)
	}
}

func TestObservatoryChiselWithDepth(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	var payload map[string]any
	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"operation_id": "op_depth_1", "status": "PENDING"}`))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":      sessID,
		"target_system":   "H2",
		"task_type":       "atlasChisel",
		"depth_asset_id":  "asset_depth_9",
		"voxel_size":      0.1,
		"ephemeral_purge": true,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	ctx, ok := payload["contextFrames"].([]any)
	if !ok || len(ctx) != 1 {
		t.Fatalf("expected 1 depth context frame, got: %v", payload["contextFrames"])
	}
	frame, _ := ctx[0].(map[string]any)
	depth, _ := frame["depth"].(map[string]any)
	dAsset, _ := depth["depthAsset"].(map[string]any)
	if dAsset["assetId"] != "asset_depth_9" {
		t.Fatalf("expected depth asset ref, got: %v", frame["depth"])
	}
	targets, _ := payload["targetCameras"].([]any)
	if len(targets) != 1 {
		t.Fatalf("expected 1 target camera, got %v", payload["targetCameras"])
	}
	// Context is rendered AT the target: same pose object.
	if fmt.Sprintf("%v", frame["camera"]) != fmt.Sprintf("%v", targets[0]) {
		t.Fatalf("context camera must equal target camera")
	}
	if payload["voxelSize"] != float64(0.1) {
		t.Fatalf("expected voxelSize forwarded, got: %v", payload["voxelSize"])
	}
	if payload["purgeContentOnOperationCompletion"] != true {
		t.Fatalf("expected purge flag forwarded, got: %v", payload)
	}
}

func TestObservatoryGenerateRefusedWithoutRGBD(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "atlasGenerate",
	}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for context-less generate, got %d (%s)", rec.Code, rec.Body.String())
	}
	if spend := lensSessionSpend(t, srv, sessID); spend != 0.0 {
		t.Fatalf("expected no spend reserved, got %f", spend)
	}
}

func TestObservatoryGenerateWithRGBD(t *testing.T) {
	var dispatchedPayload map[string]any
	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tasks:atlasGenerate") {
			_ = json.NewDecoder(r.Body).Decode(&dispatchedPayload)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"operation": {"id": "op-gen-rgbd-42"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer atlasMock.Close()

	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":     sessID,
		"target_system":  "H2",
		"task_type":      "atlasGenerate",
		"image_url":      "https://images.example.com/obs/h2-pose.png",
		"depth_asset_id": "asset-metric-depth-999",
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for atlasGenerate with RGBD context, got %d (%s)", rec.Code, rec.Body.String())
	}
	if spend := lensSessionSpend(t, srv, sessID); spend != 0.50 {
		t.Fatalf("expected 0.50 spend reserved, got %f", spend)
	}

	frames, ok := dispatchedPayload["contextFrames"].([]any)
	if !ok || len(frames) != 1 {
		t.Fatalf("expected 1 context frame dispatched, got %v", dispatchedPayload["contextFrames"])
	}
	cf, ok := frames[0].(map[string]any)
	if !ok {
		t.Fatalf("expected frame map, got %v", frames[0])
	}
	img, _ := cf["image"].(map[string]any)
	if img == nil || img["url"] != "https://images.example.com/obs/h2-pose.png" {
		t.Fatalf("expected image url in context frame, got %v", cf["image"])
	}
	depth, _ := cf["depth"].(map[string]any)
	if depth == nil {
		t.Fatalf("expected depth in context frame, got %v", cf["depth"])
	}
}

func TestObservatoryVoxelValidation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":    sessID,
		"target_system": "H2",
		"task_type":     "atlasChisel",
		"voxel_size":    0.9,
	}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-range voxel, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestObservatoryLensResolve(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	imageBytes := []byte("fake-png-bytes")
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(imageBytes)
	}))
	defer imgSrv.Close()

	opBody := fmt.Sprintf(`{"done": true, "response": {"promptUsed": "test pavilion", "frames": [{"imageAsset": {"assetId": "a1", "url": %q}}]}}`, imgSrv.URL+"/f.png")
	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(opBody))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)
	tmp := t.TempDir()
	t.Setenv("KUBONA_ARTIFACTS_DIR", tmp)

	sessID := createLensTestSession(t, srv, 20.0)

	// Pending operation short-circuits.
	pendingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"done": false}`))
	}))
	defer pendingSrv.Close()
	t.Setenv("WLT_API_BASE_URL", pendingSrv.URL)
	rec := postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_pending",
	})
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"done":false`)) {
		t.Fatalf("expected done:false, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Done operation resolves, persists promptUsed, downloads the frame
	// only when the dispatch requested ephemeral purge (dispatch first).
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)
	rec = postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_res_1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("test pavilion")) {
		t.Fatalf("expected promptUsed in resolve record, got: %s", rec.Body.String())
	}
	// No dispatch annotation with purge flag: no local download.
	if _, err := os.Stat(filepath.Join(tmp, "op_res_1", "frame-0.png")); !os.IsNotExist(err) {
		t.Fatalf("expected no local download without ephemeral dispatch")
	}

	// Idempotent: second resolve returns the stored record, no duplicate.
	rec2 := postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_res_1",
	})
	if rec2.Code != http.StatusOK || !bytes.Contains(rec2.Body.Bytes(), []byte("test pavilion")) {
		t.Fatalf("expected stored record on re-resolve, got %d (%s)", rec2.Code, rec2.Body.String())
	}
}

func TestObservatoryLensResolveEphemeralPurge(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	imageBytes := []byte("fake-png-bytes")
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(imageBytes)
	}))
	defer imgSrv.Close()

	opBody := fmt.Sprintf(`{"done": true, "response": {"promptUsed": "purge pavilion", "frames": [{"imageAsset": {"assetId": "a1", "url": %q}}]}}`, imgSrv.URL+"/f.png")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/tasks:atlasChisel", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"operation_id": "op_purge_1", "status": "PENDING"}`))
	})
	mux.HandleFunc("/api/v2/operations/op_purge_1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(opBody))
	})
	var deleted []string
	mux.HandleFunc("/api/v2/assets/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/api/v2/assets/"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	atlasMock := httptest.NewServer(mux)
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)
	tmp := t.TempDir()
	t.Setenv("KUBONA_ARTIFACTS_DIR", tmp)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id":      sessID,
		"target_system":   "H2",
		"task_type":       "atlasChisel",
		"ephemeral_purge": true,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("dispatch failed: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_purge_1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve failed: %d (%s)", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("local_paths")) {
		t.Fatalf("expected local_paths in purge record, got: %s", rec.Body.String())
	}
	if len(deleted) != 1 || deleted[0] != "a1" {
		t.Fatalf("expected explicit DELETE of cloud output asset a1, got %v", deleted)
	}
	got, err := os.ReadFile(filepath.Join(tmp, "op_purge_1", "frame-0.png"))
	if err != nil || string(got) != string(imageBytes) {
		t.Fatalf("expected downloaded frame in artifacts dir: %v", err)
	}
}

func TestObservatoryLensResolveAuth(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	for _, path := range []string{"/api/observatory/assets", "/api/observatory/lens/resolve", "/api/observatory/lens/history?session_id=x"} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{}`)))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401 without authority, got %d", path, rec.Code)
		}
	}
}

// P6: per-session lens history merges dispatch + resolve annotations from
// our own reservation ledger (rated invoices stay on the billing dashboard).
func TestObservatoryLensHistory(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	opBody := `{"done": true, "response": {"promptUsed": "history pavilion", "frames": [{"imageAsset": {"assetId": "a1", "url": "https://cdn.example/f.png"}}]}}`
	atlasMux := http.NewServeMux()
	atlasMux.HandleFunc("/api/v2/tasks:atlasChisel", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"operation_id": "op_hist_1", "status": "PENDING"}`))
	})
	atlasMux.HandleFunc("/api/v2/operations/op_hist_1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(opBody))
	})
	atlasMock := httptest.NewServer(atlasMux)
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)

	get := func() []map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/observatory/lens/history?session_id="+sessID, nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("history failed: %d (%s)", rec.Code, rec.Body.String())
		}
		var out struct {
			Operations []map[string]any `json:"operations"`
		}
		_ = json.NewDecoder(rec.Body).Decode(&out)
		return out.Operations
	}

	if ops := get(); len(ops) != 0 {
		t.Fatalf("expected empty history, got %v", ops)
	}

	postLens(t, srv, map[string]any{
		"session_id": sessID, "target_system": "H2", "task_type": "atlasChisel",
	}, "")
	ops := get()
	if len(ops) != 1 || ops[0]["task_type"] != "atlasChisel" || ops[0]["cost_usd"] != 0.5 {
		t.Fatalf("expected dispatch record, got %v", ops)
	}
	if _, hasPrompt := ops[0]["prompt_used"]; hasPrompt && ops[0]["prompt_used"] != "" {
		t.Fatalf("expected no prompt before resolve, got %v", ops[0])
	}

	postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_hist_1",
	})
	ops = get()
	if len(ops) != 1 || ops[0]["prompt_used"] != "history pavilion" || ops[0]["resolved"] != true {
		t.Fatalf("expected merged resolve record, got %v", ops)
	}
}

// P6: end-to-end webhook delivery — signed callback resolves the attributed
// operation; garbage signatures refuse; unattributed ops accept quietly.
func TestObservatoryWebhookEndToEnd(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf(`{"keys":[{"kty":"OKP","crv":"Ed25519","x":%q}]}`, base64.RawURLEncoding.EncodeToString(pub))))
	}))
	defer jwksSrv.Close()

	opBody := `{"done": true, "response": {"promptUsed": "webhook pavilion", "frames": [{"imageAsset": {"assetId": "a1", "url": "https://cdn.example/f.png"}}]}}`
	atlasMux := http.NewServeMux()
	atlasMux.HandleFunc("/api/v2/tasks:atlasChisel", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"operation_id": "op_hook_1", "status": "PENDING"}`))
	})
	atlasMux.HandleFunc("/api/v2/operations/op_hook_1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(opBody))
	})
	atlasMock := httptest.NewServer(atlasMux)
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)
	t.Setenv("WLT_JWKS_URL", jwksSrv.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postLens(t, srv, map[string]any{
		"session_id": sessID, "target_system": "H2", "task_type": "atlasChisel",
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("dispatch failed: %d (%s)", rec.Code, rec.Body.String())
	}

	deliver := func(t *testing.T, body string, idem string, sign bool) *httptest.ResponseRecorder {
		var sig string
		ts := fmt.Sprintf("%d", time.Now().Unix())
		if sign {
			sig = "v1a," + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(idem+"."+ts+"."+body)))
		} else {
			sig = "v1a," + base64.StdEncoding.EncodeToString([]byte("bogus-signature-payload-0000000000"))
		}
		req := httptest.NewRequest(http.MethodPost, "/api/observatory/webhooks", bytes.NewReader([]byte(body)))
		req.Header.Set("webhook-id", idem)
		req.Header.Set("webhook-timestamp", ts)
		req.Header.Set("webhook-signature", sig)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	delivery := `{"id":"evt_hook_1","type":"task.succeeded","data":{"accountId":"a","projectId":"p","operationId":"op_hook_1","operation":"op_hook_1","taskName":"atlasChisel","state":"SUCCEEDED"}}`

	// Unsigned/tampered delivery refuses before any state changes.
	if rec := deliver(t, delivery, "del_bad", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad signature, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Signed delivery resolves the attributed operation.
	rec = deliver(t, delivery, "del_ok", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("webhook pavilion")) {
		t.Fatalf("expected prompt_used in webhook record, got: %s", rec.Body.String())
	}

	// Unknown operation: accepted (stops the retry clock), resolves nothing.
	unknown := `{"id":"evt_hook_2","type":"task.succeeded","data":{"accountId":"a","projectId":"p","operationId":"op_ghost_9","operation":"op_ghost_9","taskName":"atlasChisel","state":"SUCCEEDED"}}`
	rec = deliver(t, unknown, "del_unknown", true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"attributed":false`)) {
		t.Fatalf("expected unattributed accept, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestObservatoryLensResolveFailedOp(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	opBody := `{"done": true, "error": {"code": 14, "message": "upstream unavailable", "details": []}}`
	atlasMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(opBody))
	}))
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	rec := postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_failed_1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with error envelope, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"done":true`)) || !bytes.Contains(rec.Body.Bytes(), []byte("upstream unavailable")) {
		t.Fatalf("expected terminal error surfaced, got: %s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"resolved":true`)) {
		t.Fatalf("failed op must not record a resolution: %s", rec.Body.String())
	}
	// Deterministic repeat: no accumulating state on the failure path.
	rec2 := postObservatory(t, srv, "/api/observatory/lens/resolve", map[string]any{
		"session_id": sessID, "operation_id": "op_failed_1",
	})
	if rec2.Body.String() != rec.Body.String() {
		t.Fatalf("error resolve not idempotent:\n%s\nvs\n%s", rec.Body.String(), rec2.Body.String())
	}
}

func TestObservatoryWebhookKeyRotation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	pub1, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)
	cur := pub1
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf(`{"keys":[{"kty":"OKP","crv":"Ed25519","x":%q}]}`, base64.RawURLEncoding.EncodeToString(cur))))
	}))
	defer jwksSrv.Close()

	opBody := `{"done": true, "response": {"promptUsed": "rotation pavilion", "frames": []}}`
	atlasMux := http.NewServeMux()
	atlasMux.HandleFunc("/api/v2/tasks:atlasChisel", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"operation_id": "op_rot_1", "status": "PENDING"}`))
	})
	atlasMux.HandleFunc("/api/v2/operations/op_rot_1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(opBody))
	})
	atlasMock := httptest.NewServer(atlasMux)
	defer atlasMock.Close()

	t.Setenv("WLT_API_KEY", "wl_live_mock_key")
	t.Setenv("WLT_API_BASE_URL", atlasMock.URL)
	t.Setenv("WLT_JWKS_URL", jwksSrv.URL)

	sessID := createLensTestSession(t, srv, 20.0)
	if rec := postLens(t, srv, map[string]any{
		"session_id": sessID, "target_system": "H2", "task_type": "atlasChisel",
	}, ""); rec.Code != http.StatusOK {
		t.Fatalf("dispatch failed: %d", rec.Code)
	}

	deliver := func(body, idem string, priv ed25519.PrivateKey) *httptest.ResponseRecorder {
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := "v1a," + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(idem+"."+ts+"."+body)))
		req := httptest.NewRequest(http.MethodPost, "/api/observatory/webhooks", bytes.NewReader([]byte(body)))
		req.Header.Set("webhook-id", idem)
		req.Header.Set("webhook-timestamp", ts)
		req.Header.Set("webhook-signature", sig)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	delivery := `{"id":"evt_rot_1","type":"task.succeeded","data":{"accountId":"a","projectId":"p","operationId":"op_rot_1","operation":"op_rot_1","taskName":"atlasChisel","state":"SUCCEEDED"}}`

	// Prime the JWKS cache with key1.
	if rec := deliver(delivery, "del_rot_1", priv1); rec.Code != http.StatusOK {
		t.Fatalf("key1 delivery failed: %d (%s)", rec.Code, rec.Body.String())
	}
	// Rotate the key set; the stale cache must miss, trigger one refetch,
	// and verify against the fresh key2 document.
	cur = pub2
	rec := deliver(delivery, "del_rot_2", priv2)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("rotation pavilion")) {
		t.Fatalf("expected rotation refetch to succeed, got %d (%s)", rec.Code, rec.Body.String())
	}
}
