package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestPreflightFreshStateIsClean(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBONA_PREFLIGHT_REQUIRE_BACKUP", "0")
	if code := runPreflight([]string{
		"--db", filepath.Join(dir, "fresh.db"),
		"--backup-dir", filepath.Join(dir, "backups"),
		"--rate-card=false",
	}); code != 0 {
		t.Fatalf("fresh preflight exit %d, want 0", code)
	}
}

func TestPreflightFailsOnFaultedChain(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "kubona.db")
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.AppendEvent("sess-preflight", "annotation", map[string]any{"x": 1}); err != nil {
		t.Fatalf("append: %v", err)
	}
	l.Close()
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := raw.Exec(`UPDATE ledger_events SET payload = '{"x":2}' WHERE id = (SELECT MIN(id) FROM ledger_events)`); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	raw.Close()
	if code := runPreflight([]string{
		"--db", dbPath,
		"--backup-dir", filepath.Join(dir, "backups"),
		"--rate-card=false",
	}); code == 0 {
		t.Fatal("preflight accepted a faulted chain")
	}
}

func TestPreflightRequiresFreshBackupWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	if err := os.Mkdir(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBONA_PREFLIGHT_REQUIRE_BACKUP", "1")
	if code := runPreflight([]string{
		"--db", filepath.Join(dir, "fresh.db"),
		"--backup-dir", backups,
		"--rate-card=false",
	}); code == 0 {
		t.Fatal("preflight accepted an empty required backup directory")
	}
}

func TestPreflightNotifiesWebhookOnFailure(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("webhook content-type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	if err := os.Mkdir(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBONA_PREFLIGHT_REQUIRE_BACKUP", "1")
	t.Setenv("KUBONA_ALERT_WEBHOOK_URL", srv.URL)
	if code := runPreflight([]string{
		"--db", filepath.Join(dir, "fresh.db"),
		"--backup-dir", backups,
		"--rate-card=false",
	}); code == 0 {
		t.Fatal("preflight did not fail")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits)
	}
}
