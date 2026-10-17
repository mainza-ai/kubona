package agents

import (
	"strings"
	"testing"
)

// P1-4 step 1 (freeze, no behavior change): the CustomAgentRegistry
// Register/List/Delete validation contract. Any future consolidation
// (Go vs Python identity home) must keep these refusals — this file pins
// them first so a migration diff that weakens one fails red.
func TestCustomAgentRegistryValidationContract(t *testing.T) {
	r := NewCustomAgentRegistry()

	// Seeds are present and immutable.
	list := r.List()
	seedIDs := map[string]bool{}
	for _, a := range list {
		if a.IsSystemAgent {
			seedIDs[a.ID] = true
		}
	}
	if len(seedIDs) != 2 {
		t.Fatalf("expected 2 system seeds, got %d", len(seedIDs))
	}

	// Empty name refuses.
	if _, err := r.Register(CustomAgentManifest{ID: "x1", Name: "  "}); err == nil ||
		!strings.Contains(err.Error(), "name is required") {
		t.Fatalf("empty name must refuse, got %v", err)
	}
	// Negative budget refuses.
	if _, err := r.Register(CustomAgentManifest{ID: "x2", Name: "n", SpendBudgetUSD: -1}); err == nil ||
		!strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("negative budget must refuse, got %v", err)
	}
	// IsSystemAgent forgery refuses (reserved for seeds).
	if _, err := r.Register(CustomAgentManifest{ID: "x3", Name: "n", IsSystemAgent: true}); err == nil ||
		!strings.Contains(err.Error(), "reserved for built-in seeds") {
		t.Fatalf("IsSystemAgent forgery must refuse, got %v", err)
	}
	// Seed-ID overwrite refuses, even with a clean manifest.
	for id := range seedIDs {
		if _, err := r.Register(CustomAgentManifest{ID: id, Name: "impostor"}); err == nil ||
			!strings.Contains(err.Error(), "reserved system seed") {
			t.Fatalf("seed overwrite of %q must refuse, got %v", id, err)
		}
		// Seed delete refuses (defense in depth: even a stored row
		// claiming the seed id must not become deletable).
		if err := r.Delete(id); err == nil ||
			!strings.Contains(err.Error(), "cannot delete system agent") {
			t.Fatalf("seed delete of %q must refuse, got %v", id, err)
		}
	}
	// Unknown delete 404s.
	if err := r.Delete("agent_no_such"); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown delete must refuse, got %v", err)
	}

	// Happy path: empty ID mints one, timestamps set, system flag forced.
	got, err := r.Register(CustomAgentManifest{Name: "minted"})
	if err != nil {
		t.Fatalf("valid register refused: %v", err)
	}
	if !strings.HasPrefix(got.ID, "agent_") {
		t.Fatalf("expected minted agent_ id, got %q", got.ID)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatal("timestamps must be set on register")
	}
	if got.IsSystemAgent {
		t.Fatal("IsSystemAgent must be forced false")
	}

	// Round trip through List, then delete removes.
	found := false
	for _, a := range r.List() {
		if a.ID == got.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("registered agent %q missing from List", got.ID)
	}
	if err := r.Delete(got.ID); err != nil {
		t.Fatalf("user delete refused: %v", err)
	}
	for _, a := range r.List() {
		if a.ID == got.ID {
			t.Fatalf("deleted agent %q still listed", got.ID)
		}
	}
}
