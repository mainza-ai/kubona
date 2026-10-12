package memory

import (
	"testing"
)

func TestFormatVectorString(t *testing.T) {
	vec := []float32{0.1, -0.25, 0.999}
	str := FormatVectorString(vec)
	expectedPrefix := "[0.100000,-0.250000,0.999000]"
	if str != expectedPrefix {
		t.Fatalf("unexpected vector string format: got %s, want %s", str, expectedPrefix)
	}

	emptyStr := FormatVectorString(nil)
	if emptyStr != "[]" {
		t.Fatalf("unexpected empty vector string: got %s, want []", emptyStr)
	}
}

func TestPostgresVectorStoreNilGuards(t *testing.T) {
	store := NewPostgresVectorStore(nil, DefaultPostgresStoreConfig())

	_, err := store.Retain(nil, "tenant-1", "public", BankWorldFacts, "test", 0.5, []float32{0.1})
	if err == nil {
		t.Fatalf("expected error on nil db connection for Retain")
	}

	_, err = store.SemanticRecall(nil, BankWorldFacts, []float32{0.1}, 100, 0.5, "tenant-1", nil, 10)
	if err == nil {
		t.Fatalf("expected error on nil db connection for SemanticRecall")
	}

	_, err = store.AcquireDistributedLock(nil, "key-1", "node-1", 10)
	if err == nil {
		t.Fatalf("expected error on nil db connection for AcquireDistributedLock")
	}

	err = store.ReleaseDistributedLock(nil, "key-1", "node-1")
	if err == nil {
		t.Fatalf("expected error on nil db connection for ReleaseDistributedLock")
	}
}

// Q-34 dormant contract: without an operator-provisioned DSN the store is
// unconfigured, and nothing in the live path may depend on it being wired.
// (Verified by grep at decision time: zero non-test references to
// NewPostgresVectorStore or Postgres* symbols outside this file.)
func TestPostgresStoreDormantContract(t *testing.T) {
	t.Setenv("KUBONA_PG_DSN", "")
	cfg := DefaultPostgresStoreConfig()
	if cfg.DSN != "" {
		t.Fatalf("dormant store must be unconfigured without KUBONA_PG_DSN, got DSN set")
	}
	// Even constructed, every method refuses a nil DB — fail-closed, never
	// a silent no-op that callers could mistake for a working sidecar.
	store := NewPostgresVectorStore(nil, cfg)
	if _, err := store.Retain(nil, "t", "public", BankWorldFacts, "x", 0.5, []float32{0.1}); err == nil {
		t.Fatalf("dormant store Retain must refuse nil DB")
	}
}
