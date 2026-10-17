package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// R1a — the Go taxonomy map must be exactly the go-emitter rows of the
// shared contract. Anyone hand-editing this map instead of the JSON (or
// vice versa) turns this test red.
func TestEventTypesMatchContract(t *testing.T) {
	contractPath := filepath.Join("..", "..", "..", "contracts", "event-types.json")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("reading shared taxonomy contract: %v", err)
	}
	var contract struct {
		Version int `json:"version"`
		Types   map[string]struct {
			Emitters []string `json:"emitters"`
		} `json:"types"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("parsing shared taxonomy contract: %v", err)
	}
	if contract.Version != 1 {
		t.Fatalf("contract version = %d, this code knows v1 — refusing to validate against an unknown major", contract.Version)
	}
	want := map[string]bool{}
	for name, row := range contract.Types {
		for _, e := range row.Emitters {
			if e == "go" {
				want[name] = true
			}
		}
	}
	for name := range KnownEventTypes {
		if !want[name] {
			t.Errorf("KnownEventTypes[%q] has no go-emitter row in the shared contract — add it there first", name)
		}
	}
	for name := range want {
		if !KnownEventTypes[name] {
			t.Errorf("contract go-emitter row %q missing from KnownEventTypes — mirror it here", name)
		}
	}
}
