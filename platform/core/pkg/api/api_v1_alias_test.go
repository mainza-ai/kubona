package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIV1AliasMatchesLegacy(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	for _, path := range []string{"/api/health", "/api/v1/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if got := w.Header().Get("Kubona-API-Version"); got != "1" {
		t.Fatalf("version header: %q", got)
	}
}

func TestAPIV1AliasMatchesLegacyPayloads(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"v1 parity"}`)))
	mk.Header.Set("Authorization", "Bearer op-token")
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
	paths := []string{
		"/api/sessions",
		"/api/sessions/" + session.ID + "/messages",
		"/api/jobs?session_id=" + session.ID,
		"/api/events?session_id=" + session.ID,
		"/api/tasks?session_id=" + session.ID,
		"/api/workbench/layout?session_id=" + session.ID,
		"/api/cost/summary",
		"/api/quantum/backends",
		"/api/quantum/settings",
		"/api/capabilities",
		"/api/fleet",
		"/api/radar",
	}
	for _, path := range paths {
		legacy := httptest.NewRequest(http.MethodGet, path, nil)
		legacy.Header.Set("Authorization", "Bearer op-token")
		lw := httptest.NewRecorder()
		srv.Handler().ServeHTTP(lw, legacy)
		versioned := httptest.NewRequest(http.MethodGet, strings.Replace(path, "/api/", "/api/v1/", 1), nil)
		versioned.Header.Set("Authorization", "Bearer op-token")
		vw := httptest.NewRecorder()
		srv.Handler().ServeHTTP(vw, versioned)
		if lw.Code != vw.Code || lw.Body.String() != vw.Body.String() {
			t.Errorf("%s parity failed: legacy %d %s / v1 %d %s", path, lw.Code, lw.Body.String(), vw.Code, vw.Body.String())
		}
	}
}
