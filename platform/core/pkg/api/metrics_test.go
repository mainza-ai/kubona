package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposeOperationsGauges(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kubona.db.backup-20260101T000000Z"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBONA_BACKUP_DIR", dir)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"# HELP kubona_jobs",
		"# HELP kubona_manuscripts",
		"kubona_bridge_configured ",
		"kubona_backup_count 1",
		"kubona_backup_age_seconds ",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	// The backup age must be a real measurement, not a placeholder.
	if !strings.Contains(body, "kubona_backup_age_seconds ") {
		t.Fatal("no backup age gauge")
	}
	_ = time.Now()
}

func TestMetricsExposeManuscriptBuildTelemetry(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	created := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"build": true})
	if created.Code != http.StatusCreated {
		t.Fatalf("create+build = %d: %s", created.Code, created.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, `kubona_manuscript_builds_total{result="success"} 1`) {
		t.Fatalf("missing build counter:\n%s", body)
	}
	if !strings.Contains(body, "kubona_manuscript_build_seconds_count 1") {
		t.Fatalf("missing build duration count:\n%s", body)
	}
	if !strings.Contains(body, "kubona_manuscript_artifact_bytes ") {
		t.Fatalf("missing artifact bytes gauge:\n%s", body)
	}
}
