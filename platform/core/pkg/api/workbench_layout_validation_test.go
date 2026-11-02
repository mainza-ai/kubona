package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkbenchLayoutRejectsUnknownRefsLinksAreasAndBodies(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"workbench validation v2"}`)))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, mk)
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(mw.Body).Decode(&session); err != nil || session.ID == "" {
		t.Fatalf("session setup: %v", err)
	}
	put := func(document string) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"revision": 0, "document": json.RawMessage(document)})
		req := httptest.NewRequest(http.MethodPut, "/api/workbench/layout?session_id="+session.ID, bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code
	}
	base := `{"version":2,"session_id":"` + session.ID + `","objects":[{"id":"a","kind":"campaign","position":{"x":0,"y":0},"ref":{"domain":"campaign","id":"` + session.ID + `"}},{"id":"molecule","kind":"molecule","position":{"x":400,"y":0},"ref":{"domain":"campaign","id":"` + session.ID + `"}}],"links":[],"areas":[],"camera":{"x":0,"y":0,"zoom":1}}`
	if code := put(base); code != http.StatusOK {
		t.Fatalf("base layout: %d", code)
	}
	if code := put(strings.Replace(base, `"domain":"campaign"`, `"domain":"secrets"`, 1)); code != http.StatusBadRequest {
		t.Fatalf("unknown ref domain must 400, got %d", code)
	}
	if code := put(strings.Replace(base, `"links":[]`, `"links":[{"id":"l","source":"a","target":"ghost","kind":"supports"}]`, 1)); code != http.StatusBadRequest {
		t.Fatalf("unknown link endpoint must 400, got %d", code)
	}
	if code := put(strings.Replace(base, `"kind":"campaign"`, `"kind":"campaign","content":"secret body"`, 1)); code != http.StatusBadRequest {
		t.Fatalf("object content body must 400, got %d", code)
	}
	if code := put(strings.Replace(base, `"kind":"campaign","position"`, `"kind":"campaign","area":"missing-area","position"`, 1)); code != http.StatusBadRequest {
		t.Fatalf("unknown area assignment must 400, got %d", code)
	}
	if code := put(strings.Replace(base, `"areas":[]`, `"areas":[],"hidden":["ghost"]`, 1)); code != http.StatusBadRequest {
		t.Fatalf("unknown hidden object must 400, got %d", code)
	}
}
