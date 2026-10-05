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
