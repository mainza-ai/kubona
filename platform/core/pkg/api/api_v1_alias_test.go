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

func normalizeFleetAgeForParity(t *testing.T, body string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("fleet parity body: %v", err)
	}
	backends, _ := doc["backends"].([]any)
	for _, raw := range backends {
		row, _ := raw.(map[string]any)
		if _, ok := row["calibration_age_hours"]; ok {
			delete(row, "calibration_age_hours")
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("normalize fleet: %v", err)
	}
	return string(out)
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
		legacyBody, versionedBody := lw.Body.String(), vw.Body.String()
		if path == "/api/fleet" {
			// Calibration age is measured from wall-clock now; two sequential
			// calls legitimately differ by microseconds. Compare every other
			// field byte-for-byte and require the volatile field on both.
			legacyBody = normalizeFleetAgeForParity(t, legacyBody)
			versionedBody = normalizeFleetAgeForParity(t, versionedBody)
		}
		if lw.Code != vw.Code || legacyBody != versionedBody {
			t.Errorf("%s parity failed: legacy %d %s / v1 %d %s", path, lw.Code, legacyBody, vw.Code, versionedBody)
		}
	}
}
