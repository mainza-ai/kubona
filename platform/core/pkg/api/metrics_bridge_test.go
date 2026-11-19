package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadBridgeRSSFreshAndStale(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "kubona.db")
	path := filepath.Join(dir, "bridge-rss.json")
	if err := os.WriteFile(path, []byte(`{"rss_bytes":123456}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readBridgeRSS(db); got != 123456 {
		t.Fatalf("rss=%d want 123456", got)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if got := readBridgeRSS(db); got != 0 {
		t.Fatalf("stale rss=%d want 0", got)
	}
}
