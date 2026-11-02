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
