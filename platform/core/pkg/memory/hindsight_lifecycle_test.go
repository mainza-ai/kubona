package memory

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func testStore(t *testing.T, cfg StoreConfig) (*HindsightStore, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "m.db")+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewPersistentHindsightStoreWithConfig(db, cfg), db
}

func TestGlobalBudgetBoundsWholeStore(t *testing.T) {
	cfg := DefaultStoreConfig()
	cfg.MaxItemsPerBank = 100
	cfg.MaxTotalItems = 3
	cfg.MaxTotalBytes = 1 << 20
	s, _ := testStore(t, cfg)
	for i := 0; i < 6; i++ {
		s.RetainScoped("lab-a", "lab-a", BankExperiences, string(rune('a'+i))+" distinct experience", 0.5, nil)
	}
	st := s.Stats()
	if st.Items > 3 {
		t.Fatalf("items=%d global cap 3 not enforced", st.Items)
	}
}

func TestDedupeMergesRepeatedContent(t *testing.T) {
	cfg := DefaultStoreConfig()
	s, _ := testStore(t, cfg)
	first := s.RetainScoped("lab-a", "lab-a", BankExperiences, "same turn", 0.4, nil)
	second := s.RetainScoped("lab-a", "lab-a", BankExperiences, "same turn", 0.9, nil)
	if first.ID != second.ID {
		t.Fatalf("duplicate content created a second item: %s vs %s", first.ID, second.ID)
	}
	if second.Importance != 0.9 {
		t.Fatalf("importance=%v want merge bump 0.9", second.Importance)
	}
	if got := s.Stats().Items; got != 1 {
		t.Fatalf("items=%d want 1 after dedupe", got)
	}
}

func TestPurgeRetractedRemovesTombstones(t *testing.T) {
	s, _ := testStore(t, DefaultStoreConfig())
	item := s.RetainScoped("lab-a", "lab-a", BankExperiences, "to retract", 0.5, nil)
	if err := s.RetractScoped("lab-a", item.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if got := s.PurgeRetracted(time.Now().UTC().Add(time.Hour)); got != 1 {
		t.Fatalf("purged=%d want 1", got)
	}
	if got := s.Stats().Items; got != 0 {
		t.Fatalf("items=%d after purge", got)
	}
}

type countingEmbedder struct{ calls int64 }

func (c *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, string, error) {
	atomic.AddInt64(&c.calls, 1)
	vec := make([]float32, EmbeddingDimensions)
	for i := range vec {
		vec[i] = float32((len(text) + i) % 7)
	}
	return vec, "counting-v1", nil
}

func TestEmbeddingCacheAndUnlockedCall(t *testing.T) {
	s, _ := testStore(t, DefaultStoreConfig())
	emb := &countingEmbedder{}
	s.SetEmbedder(emb)
	s.RetainScoped("lab-a", "lab-a", BankExperiences, "embed me once", 0.5, nil)
	s.SemanticRecallScoped(BankExperiences, "embed me once", 100, 0, "lab-a", []string{"lab-a"})
	s.SemanticRecallScoped(BankExperiences, "embed me once", 100, 0, "lab-a", []string{"lab-a"})
	if got := atomic.LoadInt64(&emb.calls); got != 1 {
		t.Fatalf("embedding calls=%d want 1 (retain + 2 cached queries)", got)
	}
}

func TestLoadRespectsGlobalBudget(t *testing.T) {
	cfg := DefaultStoreConfig()
	cfg.MaxTotalItems = 50
	_, db := testStore(t, cfg)
	now := time.Now().UTC()
	for i := 0; i < 200; i++ {
		id := "m" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		if _, err := db.Exec(`INSERT INTO hindsight_memories (id, tenant_id, readership, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, "lab-a", "lab-a", string(BankExperiences), id, 1, 0.5, 0.5, now, now); err != nil {
			t.Fatal(err)
		}
	}
	loaded := NewPersistentHindsightStoreWithConfig(db, cfg)
	if got := len(loaded.items); got > 50 {
		t.Fatalf("loaded=%d want <=50", got)
	}
}

func TestSupersedeHidesOldMemoryFromRecall(t *testing.T) {
	s, db := testStore(t, DefaultStoreConfig())
	old := s.RetainScoped("lab-a", "lab-a", BankExperiences, "the budget is 5 USD", 0.7, nil)
	newer := s.RetainScoped("lab-a", "lab-a", BankExperiences, "the budget is 10 USD", 0.9, nil)
	if err := s.SupersedeScoped("lab-a", old.ID, newer.ID); err != nil {
		t.Fatal(err)
	}
	recalled := s.SemanticRecallScoped(BankExperiences, "the budget is 5 USD", 100, 0, "lab-a", []string{"lab-a"})
	for _, hit := range recalled {
		if hit.Item.ID == old.ID {
			t.Fatal("superseded memory must not be recalled as current")
		}
	}
	listed := s.ListRecent(BankExperiences, "lab-a", 100)
	for _, item := range listed {
		if item.ID == old.ID {
			t.Fatal("superseded memory must not be listed as current")
		}
	}
	var validTo sql.NullTime
	if err := db.QueryRow(`SELECT valid_to FROM hindsight_memories WHERE id = ?`, old.ID).Scan(&validTo); err != nil {
		t.Fatal(err)
	}
	if !validTo.Valid {
		t.Fatal("superseded row lost its validity end")
	}
}

func TestFeedbackCalibratesImportanceAndTemporalGraph(t *testing.T) {
	s, _ := testStore(t, DefaultStoreConfig())
	old := s.RetainScoped("lab-a", "lab-a", BankExperiences, "old fact", 0.5, nil)
	newer := s.RetainScoped("lab-a", "lab-a", BankExperiences, "new fact", 0.5, nil)
	if err := s.SupersedeScoped("lab-a", old.ID, newer.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FeedbackScoped("lab-a", newer.ID, true); err != nil {
		t.Fatal(err)
	}
	updated := s.items[newer.ID]
	if updated.HelpfulCount != 1 || updated.Importance <= 0.5 {
		t.Fatalf("feedback not applied: %+v", updated)
	}
	graph, err := s.TemporalGraph("lab-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Edges[0].Kind != "supersedes" {
		t.Fatalf("temporal graph=%+v", graph)
	}
}
