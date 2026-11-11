package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkbenchLayoutRevisionCAS(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"workbench v2"}`)))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, mk)
	if mw.Code != http.StatusCreated && mw.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", mw.Code, mw.Body.String())
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(mw.Body).Decode(&session); err != nil || session.ID == "" {
		t.Fatalf("decode session: %v", err)
	}

	put := func(revision int, document string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"revision": revision, "document": json.RawMessage(document)})
		req := httptest.NewRequest(http.MethodPut, "/api/workbench/layout?session_id="+session.ID, bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	doc := `{"version":2,"session_id":"` + session.ID + `","objects":[{"id":"campaign","kind":"campaign","position":{"x":0,"y":0},"ref":{"domain":"campaign","id":"` + session.ID + `"}}],"links":[],"areas":[],"camera":{"x":0,"y":0,"zoom":1}}`
	if w := put(0, doc); w.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", w.Code, w.Body.String())
	}
	if w := put(0, doc); w.Code != http.StatusConflict {
		t.Fatalf("stale revision must 409, got %d", w.Code)
	}
	if w := put(1, doc); w.Code != http.StatusOK {
		t.Fatalf("fresh revision: %d %s", w.Code, w.Body.String())
	}
	if w := put(2, `{"version":2,"objects":[{"id":"x","kind":"bogus","position":{"x":0,"y":0}}]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind must 400, got %d", w.Code)
	}
	if w := put(2, `{"version":2,"objects":[{"id":"x","kind":"note","position":{"x":99999999,"y":0}}]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("out-of-bounds position must 400, got %d", w.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/workbench/layout?session_id="+session.ID, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Revision int64           `json:"revision"`
		Document json.RawMessage `json:"document"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Revision != 2 || !strings.Contains(string(got.Document), `"campaign"`) {
		t.Fatalf("unexpected layout: revision=%d document=%s", got.Revision, got.Document)
	}
}

func TestWorkbenchLayoutSaveIsIdempotentWithRequestID(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"workbench idem"}`)))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, mk)
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(mw.Body).Decode(&session); err != nil || session.ID == "" {
		t.Fatalf("decode session: %v", err)
	}
	doc := `{"version":2,"session_id":"` + session.ID + `","objects":[],"links":[],"areas":[],"camera":{"x":0,"y":0,"zoom":1}}`
	save := func(revision int, document, requestID string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{
			"revision": revision, "document": json.RawMessage(document), "request_id": requestID,
		})
		req := httptest.NewRequest(http.MethodPut, "/api/workbench/layout?session_id="+session.ID, bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	first := save(0, doc, "save-layout-1")
	if first.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", first.Code, first.Body.String())
	}
	replay := save(0, doc, "save-layout-1")
	if replay.Code != http.StatusOK || replay.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("replay must return the stored receipt: %d %q", replay.Code, replay.Header().Get("X-Idempotent-Replay"))
	}
	opReq := httptest.NewRequest(http.MethodGet, "/api/operations/save-layout-1?session_id="+session.ID, nil)
	opW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(opW, opReq)
	if opW.Code != http.StatusOK {
		t.Fatalf("operation status: %d %s", opW.Code, opW.Body.String())
	}
	var op struct {
		Status     string          `json:"status"`
		StatusCode int             `json:"status_code"`
		Receipt    json.RawMessage `json:"receipt"`
	}
	if err := json.Unmarshal(opW.Body.Bytes(), &op); err != nil || op.Status != "complete" || op.StatusCode != http.StatusOK {
		t.Fatalf("unexpected operation receipt: %s (%v)", opW.Body.String(), err)
	}
	conflict := save(0, strings.Replace(doc, `"objects":[]`, `"objects":[{"id":"n","kind":"note","position":{"x":1,"y":1}}]`, 1), "save-layout-1")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("same request id with different content must 409, got %d %s", conflict.Code, conflict.Body.String())
	}
	next := save(1, doc, "save-layout-2")
	if next.Code != http.StatusOK {
		t.Fatalf("next revision: %d %s", next.Code, next.Body.String())
	}
}

func TestWorkbenchViewRevisionCASAndDigestIdempotency(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"workbench view"}`)))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, mk)
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(mw.Body).Decode(&session); err != nil || session.ID == "" {
		t.Fatalf("create session: %v", err)
	}

	putView := func(rev int, view, reqID string) *httptest.ResponseRecorder {
		t.Helper()
		payload := map[string]any{"view_revision": rev, "view": json.RawMessage(view)}
		if reqID != "" {
			payload["request_id"] = reqID
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/api/workbench/view?session_id="+session.ID, bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	// Bad zoom and content smuggling must fail closed with 400.
	if w := putView(0, `{"camera":{"x":0,"y":0,"zoom":99}}`, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range zoom must 400, got %d", w.Code)
	}
	if w := putView(0, `{"camera":{"x":0,"y":0,"zoom":1},"content":"secret"}`, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("content body smuggling must 400, got %d", w.Code)
	}

	view1 := `{"camera":{"x":120,"y":80,"zoom":1},"selected":["campaign"]}`
	if w := putView(0, view1, "view-1"); w.Code != http.StatusOK {
		t.Fatalf("first view save: %d %s", w.Code, w.Body.String())
	}
	// Stale revision conflicts with 409 carrying current_view_revision.
	if w := putView(0, `{"camera":{"x":140,"y":80,"zoom":1},"selected":[]}`, ""); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"current_view_revision":1`) {
		t.Fatalf("stale view_revision must 409, got %d %s", w.Code, w.Body.String())
	}
	// Identical view content with current revision succeeds without bumping revision.
	reorderedSame := `{"selected":["campaign"],"camera":{"zoom":1,"y":80,"x":120}}`
	sameW := putView(1, reorderedSame, "")
	if sameW.Code != http.StatusOK || !strings.Contains(sameW.Body.String(), `"view_revision":1`) || !strings.Contains(sameW.Body.String(), `"unchanged":true`) {
		t.Fatalf("identical digest save must keep revision=1, got %d %s", sameW.Code, sameW.Body.String())
	}
	// Different view content bumps revision cleanly.
	if w := putView(1, `{"camera":{"x":220,"y":90,"zoom":1.1},"selected":[]}`, "view-2"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"view_revision":2`) {
		t.Fatalf("changed view must bump to revision 2, got %d %s", w.Code, w.Body.String())
	}
}

func TestWorkbenchViewExtractsLegacyCameraWithoutRewritingLayout(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"legacy cam"}`)))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, mk)
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(mw.Body).Decode(&session); err != nil || session.ID == "" {
		t.Fatalf("create session: %v", err)
	}

	legacyDoc := `{"version":2,"session_id":"` + session.ID + `","objects":[{"id":"campaign","kind":"campaign","pinned":true,"position":{"x":42,"y":64},"ref":{"domain":"campaign","id":"` + session.ID + `"}}],"links":[],"areas":[],"camera":{"x":188,"y":240,"zoom":1.25},"selected":["campaign"]}`
	body, _ := json.Marshal(map[string]any{"revision": 0, "document": json.RawMessage(legacyDoc)})
	putReq := httptest.NewRequest(http.MethodPut, "/api/workbench/layout?session_id="+session.ID, bytes.NewReader(body))
	putW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("legacy layout save: %d %s", putW.Code, putW.Body.String())
	}

	viewReq := httptest.NewRequest(http.MethodGet, "/api/workbench/view?session_id="+session.ID, nil)
	viewW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(viewW, viewReq)
	if viewW.Code != http.StatusOK {
		t.Fatalf("view get: %d %s", viewW.Code, viewW.Body.String())
	}
	var got struct {
		ViewRevision int64 `json:"view_revision"`
		View         struct {
			Camera   struct{ X, Y, Zoom float64 } `json:"camera"`
			Selected []string                     `json:"selected"`
		} `json:"view"`
	}
	if err := json.Unmarshal(viewW.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if got.ViewRevision != 0 || got.View.Camera.X != 188 || got.View.Camera.Y != 240 || got.View.Camera.Zoom != 1.25 || len(got.View.Selected) != 1 || got.View.Selected[0] != "campaign" {
		t.Fatalf("unexpected extracted legacy view: %+v", got)
	}

	// Layout revision must remain untouched by reading /api/workbench/view.
	layReq := httptest.NewRequest(http.MethodGet, "/api/workbench/layout?session_id="+session.ID, nil)
	layW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(layW, layReq)
	if layW.Code != http.StatusOK || !strings.Contains(layW.Body.String(), `"revision":1`) {
		t.Fatalf("layout must not be mutated on view read: %d %s", layW.Code, layW.Body.String())
	}
}

func TestLLMModelsEmptyArrayJSON(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	gReq := httptest.NewRequest(http.MethodGet, "/api/llm/settings", nil)
	gW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(gW, gReq)
	if gW.Code != http.StatusOK {
		t.Fatalf("llm settings get: %d %s", gW.Code, gW.Body.String())
	}
	var settings map[string]any
	if err := json.Unmarshal(gW.Body.Bytes(), &settings); err != nil {
		t.Fatalf("decode llm settings: %v", err)
	}
	providers, _ := settings["providers"].(map[string]any)
	omlx, _ := providers["omlx"].(map[string]any)
	omlx["base_url"] = upstream.URL
	body, _ := json.Marshal(settings)

	sReq := httptest.NewRequest(http.MethodPost, "/api/llm/settings", bytes.NewReader(body))
	sReq.Header.Set("Authorization", "Bearer op-token")
	sReq.Header.Set("Content-Type", "application/json")
	sW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sW, sReq)
	if sW.Code != http.StatusOK {
		t.Fatalf("llm settings update: %d %s", sW.Code, sW.Body.String())
	}

	mReq := httptest.NewRequest(http.MethodGet, "/api/llm/models?provider=omlx", nil)
	mW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mW, mReq)
	if mW.Code != http.StatusOK {
		t.Fatalf("llm models get: %d %s", mW.Code, mW.Body.String())
	}
	if !strings.Contains(mW.Body.String(), `"models":[]`) {
		t.Fatalf("expected empty JSON array for models, got %s", mW.Body.String())
	}
}
