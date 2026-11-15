package memory

// B10 pins: retraction is a tombstone — content cleared, embedding removed,
// recall/list never return it, and the write is tenant-scoped.

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRetractScopedTombstonesAndHides(t *testing.T) {
	store := NewHindsightStore()
	item := store.RetainScoped("lab-a", "lab-a", BankExperiences, "Trial 12 diverged on LiH due to barren plateau", 0.9, nil)
	if got := store.SemanticRecallScoped(BankExperiences, "LiH barren plateau", 1000, 0, "lab-a", []string{"lab-a"}); len(got) != 1 {
		t.Fatalf("precondition: memory must be recallable, got %d", len(got))
	}

	if err := store.RetractScoped("lab-b", item.ID, "wrong lab"); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("cross-tenant retraction must refuse, got %v", err)
	}
	if err := store.RetractScoped("lab-a", "missing", "x"); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("unknown id must refuse, got %v", err)
	}
	if err := store.RetractScoped("lab-a", item.ID, "operator request"); err != nil {
		t.Fatalf("retract: %v", err)
	}
	if item.Content != "[retracted]" || item.RetractedAt == nil || item.RetractReason != "operator request" {
		t.Fatalf("tombstone malformed: %+v", item)
	}
	if got := store.SemanticRecallScoped(BankExperiences, "LiH barren plateau", 1000, 0, "lab-a", []string{"lab-a"}); len(got) != 0 {
		t.Fatalf("retracted memory must not be recalled, got %d", len(got))
	}
	if got := store.Recall(BankExperiences, "LiH", 1000); len(got) != 0 {
		t.Fatalf("retracted memory must not be recalled by text, got %d", len(got))
	}
	if got := store.ListRecent(BankExperiences, "lab-a", 1000); len(got) != 0 {
		t.Fatalf("retracted memory must not be listed, got %d", len(got))
	}
}

func TestRetractionPersistsAcrossReload(t *testing.T) {
	db := testDB(t)
	store := NewPersistentHindsightStore(db)
	item := store.RetainScoped("lab-a", "lab-a", BankExperiences, "secret-adjacent experience", 0.5, nil)
	if err := store.RetractScoped("lab-a", item.ID, "reload drill"); err != nil {
		t.Fatal(err)
	}
	reloaded := NewPersistentHindsightStore(db)
	recalled := reloaded.SemanticRecallScoped(BankExperiences, "secret-adjacent", 1000, 0, "lab-a", []string{"lab-a"})
	if len(recalled) != 0 {
		t.Fatalf("a tombstone must survive reload, got %d rows", len(recalled))
	}
}
