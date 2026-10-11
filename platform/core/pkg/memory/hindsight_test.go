package memory

import (
	"database/sql"
	"math"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestHindsightMemoryLifecycle(t *testing.T) {
	store := NewHindsightStore()

	// Retain world fact
	fact := store.Retain(BankWorldFacts, "H2 ground state equilibrium bond distance is 0.7414 Angstroms", 0.95)
	if fact.Bank != BankWorldFacts {
		t.Fatalf("expected world facts bank")
	}

	// Retain experience
	exp := store.Retain(BankExperiences, "Trial run #12 diverged due to vanishing gradients on 6-qubit hardware", 0.8)
	if exp.Bank != BankExperiences {
		t.Fatalf("expected experiences bank")
	}

	// Recall with token budget
	results := store.Recall(BankWorldFacts, "0.7414", 50)
	if len(results) != 1 || results[0].ID != fact.ID {
		t.Fatalf("expected to recall world fact within token budget")
	}

	// Small token budget should prevent over-packing
	tinyRecall := store.Recall("", "", 5)
	if len(tinyRecall) > 1 {
		t.Fatalf("tiny token budget exceeded")
	}

	// Decay test: after 20 hours, experience score should decay
	initialScore := exp.EffectiveScore
	store.Decay(20.0)
	if exp.EffectiveScore >= initialScore {
		t.Fatalf("expected experience score to decay from %f, got %f", initialScore, exp.EffectiveScore)
	}

	// Reflect into mental model
	model := store.Reflect("Avoid deep entangler layers beyond 4 qubits without barren plateau pre-checks")
	if model.Bank != BankMentalModels || model.EffectiveScore != 1.0 {
		t.Fatalf("unexpected reflected model: %+v", model)
	}
}

func TestVectorSerialization(t *testing.T) {
	orig := []float32{0.123, -0.456, 0.789, -0.999}
	blob := EncodeEmbedding(orig)
	decoded, err := DecodeEmbedding(blob)
	if err != nil {
		t.Fatalf("failed to decode embedding: %v", err)
	}
	if len(decoded) != len(orig) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(orig))
	}
	for i := range orig {
		if math.Abs(float64(decoded[i]-orig[i])) > 1e-6 {
			t.Fatalf("value mismatch at %d: got %f, want %f", i, decoded[i], orig[i])
		}
	}
}

func TestHindsightSemanticVectorRecall(t *testing.T) {
	store := NewHindsightStore()

	m1 := store.Retain(BankExperiences, "Barren plateau observed in deep 6-qubit Ry-Rz ansatz on LiH molecule due to vanishing cost gradients", 0.9)
	m2 := store.Retain(BankExperiences, "Hardware calibration drift observed on Quantinuum H1 trapped-ion processor after 2 hours", 0.7)
	m3 := store.Retain(BankExperiences, "Aspirin synthesis ADMET profile passed Lipinski Rule of 5 and solubility checks", 0.6)

	// Semantic query focused on barren plateaus and LiH gradients
	query := "LiH ansatz vanishing gradients and barren plateau issue"
	recalled := store.SemanticRecall(BankExperiences, query, 500, 0.3)

	if len(recalled) == 0 {
		t.Fatalf("expected semantic recall results, got none")
	}

	// Top result must be m1
	if recalled[0].Item.ID != m1.ID {
		t.Fatalf("expected top recalled item to be m1 (Barren plateau on LiH), got %s", recalled[0].Item.Content)
	}

	if recalled[0].Similarity < 0.4 {
		t.Fatalf("expected similarity >= 0.4 for relevant match, got %f", recalled[0].Similarity)
	}

	// Check relative similarity: m1 should be significantly higher than m3 (aspirin)
	var m3Sim float64
	for _, r := range recalled {
		if r.Item.ID == m3.ID {
			m3Sim = r.Similarity
		}
	}
	if recalled[0].Similarity <= m3Sim {
		t.Fatalf("expected m1 similarity (%f) to exceed m3 similarity (%f)", recalled[0].Similarity, m3Sim)
	}

	_ = m2
}

func TestHindsightSQLitePersistence(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite3: %v", err)
	}
	defer db.Close()

	store1 := NewPersistentHindsightStore(db)
	item := store1.Retain(BankMentalModels, "Always compute RHF classical anchor before dispatching to IBM QPU", 0.95)

	// Open a second store on the same DB to test hydration
	store2 := NewPersistentHindsightStore(db)
	recalled := store2.SemanticRecall(BankMentalModels, "RHF classical baseline anchor prior to quantum execution", 500, 0.4)

	if len(recalled) == 0 {
		t.Fatalf("expected semantic recall to find persisted mental model, got none")
	}

	if recalled[0].Item.ID != item.ID {
		t.Fatalf("expected hydrated item ID %s, got %s", item.ID, recalled[0].Item.ID)
	}
}

func TestHindsightLRUEviction(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite3: %v", err)
	}
	defer db.Close()

	cfg := StoreConfig{
		MaxItemsPerBank:  3,
		MaxTokensPerBank: 50000,
		DecayInterval:    time.Hour,
		DecayRatePerHour: 0.01,
	}

	store := NewPersistentHindsightStoreWithConfig(db, cfg)

	// Retain 3 items in BankExperiences with different importances
	itemLow := store.Retain(BankExperiences, "Low priority noisy measurement outcome", 0.2)
	itemMid := store.Retain(BankExperiences, "Medium priority convergence slowdown on simulator", 0.5)
	itemHigh := store.Retain(BankExperiences, "High priority barren plateau gradient vanishing warning", 0.9)

	// Verify all 3 present
	if len(store.items) != 3 {
		t.Fatalf("expected 3 items before eviction, got %d", len(store.items))
	}

	// Insert 4th item with high importance
	itemNew := store.Retain(BankExperiences, "Critical circuit depth limit exceeded for H2 ansatz", 0.85)

	// Bank limit is 3, so itemLow (score 0.2) should be evicted
	if len(store.items) != 3 {
		t.Fatalf("expected exactly 3 items after eviction, got %d", len(store.items))
	}

	if _, exists := store.items[itemLow.ID]; exists {
		t.Fatalf("expected lowest score item %s to be evicted from memory", itemLow.ID)
	}

	if _, exists := store.items[itemMid.ID]; !exists {
		t.Fatalf("expected mid item %s to be retained", itemMid.ID)
	}
	if _, exists := store.items[itemHigh.ID]; !exists {
		t.Fatalf("expected high item %s to be retained", itemHigh.ID)
	}
	if _, exists := store.items[itemNew.ID]; !exists {
		t.Fatalf("expected new item %s to be retained", itemNew.ID)
	}

	// Verify SQLite deletion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM hindsight_memories WHERE id = ?", itemLow.ID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected evicted item to be purged from sqlite, found %d rows", count)
	}
}

func TestHindsightIFCScopingAndTenantIsolation(t *testing.T) {
	store := NewHindsightStore()

	// Retain item for tenant A, restricted to Alice and Bob
	itemA := store.RetainScoped(
		"tenant-a",
		"alice, bob",
		BankExperiences,
		"Confidential LiH ansatz parameter configuration for enterprise quantum experiment",
		0.9,
		nil,
	)

	// Retain item for tenant B, restricted to Carol
	itemB := store.RetainScoped(
		"tenant-b",
		"carol",
		BankExperiences,
		"Confidential quantum circuit design with zero noise mitigation",
		0.8,
		nil,
	)

	// Retain public item for tenant A
	itemPub := store.RetainScoped(
		"tenant-a",
		"public",
		BankExperiences,
		"Publicly known quantum variational algorithm guidelines",
		0.7,
		nil,
	)

	// 1. Requester from tenant-a with principal "alice"
	// Destination set is {alice}. Source itemA set is {alice, bob}.
	// Buzz IFC lattice rule: source.CanFlowTo(dest) -> dest readers must be subset of source readers.
	// Since {alice} is subset of {alice, bob}, itemA may flow to {alice}.
	// itemPub is public and can flow to anyone.
	// itemB belongs to tenant-b, so tenant boundary must block it.
	recalledAlice := store.SemanticRecallScoped(
		BankExperiences,
		"quantum",
		1000,
		0.01,
		"tenant-a",
		[]string{"alice"},
	)

	hasItemA := false
	hasItemB := false
	hasItemPub := false
	for _, r := range recalledAlice {
		if r.Item.ID == itemA.ID {
			hasItemA = true
		}
		if r.Item.ID == itemB.ID {
			hasItemB = true
		}
		if r.Item.ID == itemPub.ID {
			hasItemPub = true
		}
	}

	if !hasItemA {
		t.Fatalf("expected alice in tenant-a to access itemA")
	}
	if hasItemB {
		t.Fatalf("tenant isolation failure: tenant-a requester received itemB from tenant-b")
	}
	if !hasItemPub {
		t.Fatalf("expected public itemPub to be visible")
	}

	// 2. Requester from tenant-a with principal "dave" (not in itemA's readership)
	recalledDave := store.SemanticRecallScoped(
		BankExperiences,
		"quantum",
		1000,
		0.01,
		"tenant-a",
		[]string{"dave"},
	)

	for _, r := range recalledDave {
		if r.Item.ID == itemA.ID {
			t.Fatalf("IFC violation: unauthorized principal dave accessed itemA")
		}
	}

	// 3. Unauthenticated/public requester with no principals (Public reader set)
	// Restricted items must NOT flow to public
	recalledPublic := store.SemanticRecallScoped(
		BankExperiences,
		"quantum",
		1000,
		0.01,
		"tenant-a",
		nil,
	)

	for _, r := range recalledPublic {
		if r.Item.ID == itemA.ID || r.Item.ID == itemB.ID {
			t.Fatalf("IFC violation: public unauthenticated recall accessed restricted item %s", r.Item.ID)
		}
	}
}

func TestHindsightDecayPersistence(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite3: %v", err)
	}
	defer db.Close()

	store := NewPersistentHindsightStore(db)
	item := store.Retain(BankExperiences, "Temporary experimental result with fast decay", 1.0)

	initialScore := item.EffectiveScore

	// Apply decay for 24 hours
	store.Decay(24.0)

	if item.EffectiveScore >= initialScore {
		t.Fatalf("expected effective score to decay, got %f vs initial %f", item.EffectiveScore, initialScore)
	}

	// Verify persistence in SQLite
	var persistedScore float64
	err = db.QueryRow("SELECT effective_score FROM hindsight_memories WHERE id = ?", item.ID).Scan(&persistedScore)
	if err != nil {
		t.Fatalf("failed to read persisted score: %v", err)
	}

	if math.Abs(persistedScore-item.EffectiveScore) > 1e-4 {
		t.Fatalf("persisted score %f does not match memory score %f", persistedScore, item.EffectiveScore)
	}
}

func TestTenantIsolationHasNoDefaultBypass(t *testing.T) {
	store := NewHindsightStore()

	// Same content retained in two lab tenants.
	store.RetainScoped("lab-a", "public", BankExperiences, "Acme proprietary ansatz calibration secret", 0.9, nil)
	store.RetainScoped("lab-b", "public", BankExperiences, "Acme proprietary ansatz calibration secret", 0.9, nil)

	// Each lab sees exactly its own copy — no cross-tenant leak.
	gotA := store.SemanticRecallScoped(BankExperiences, "Acme proprietary ansatz calibration", 10000, 0.0, "lab-a", []string{"lab-a"})
	if len(gotA) != 1 || gotA[0].Item.TenantID != "lab-a" {
		t.Fatalf("lab-a must see exactly its own item: %+v", gotA)
	}
	gotB := store.SemanticRecallScoped(BankExperiences, "Acme proprietary ansatz calibration", 10000, 0.0, "lab-b", []string{"lab-b"})
	if len(gotB) != 1 || gotB[0].Item.TenantID != "lab-b" {
		t.Fatalf("lab-b must see exactly its own item: %+v", gotB)
	}

	// A third lab sees nothing; the old 'default' bypass would have leaked
	// items tenants marked default, so also assert a default-tenant item is
	// invisible to other labs.
	store.Retain(BankExperiences, "Default-bank shared calibration note", 0.5)
	gotC := store.SemanticRecallScoped(BankExperiences, "calibration", 10000, 0.0, "lab-c", []string{"lab-c"})
	for _, it := range gotC {
		if it.Item.TenantID != "lab-c" {
			t.Fatalf("lab-c must not see foreign items: %+v", it.Item)
		}
	}
}

func TestListRecentInventory(t *testing.T) {
	store := NewHindsightStore()
	store.RetainScoped("lab-a", "lab-a", BankExperiences, "first memory", 0.5, nil)
	time.Sleep(5 * time.Millisecond)
	store.RetainScoped("lab-a", "lab-a", BankWorldFacts, "second memory", 0.5, nil)
	time.Sleep(5 * time.Millisecond)
	store.RetainScoped("lab-b", "lab-b", BankExperiences, "other tenant", 0.5, nil)

	listed := store.ListRecent("", "lab-a", 1000)
	if len(listed) != 2 {
		t.Fatalf("expected 2 lab-a memories, got %d", len(listed))
	}
	if listed[0].Content != "second memory" || listed[1].Content != "first memory" {
		t.Errorf("newest-first ordering broken: %q, %q", listed[0].Content, listed[1].Content)
	}
	// Bank filter narrows; tenant boundary holds.
	if got := store.ListRecent(BankExperiences, "lab-a", 1000); len(got) != 1 || got[0].Content != "first memory" {
		t.Errorf("bank filter wrong: %+v", got)
	}
	if got := store.ListRecent("", "lab-b", 1000); len(got) != 1 {
		t.Errorf("tenant boundary wrong: %+v", got)
	}
	// Token budget binds the listing.
	if got := store.ListRecent("", "lab-a", 1); len(got) != 1 {
		t.Errorf("budget must bound listing, got %d", len(got))
	}
}
