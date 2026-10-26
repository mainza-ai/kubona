package api

import (
	"encoding/json"
	"testing"
)

func TestIndexCanaryResultsPerProvider(t *testing.T) {
	raw := `[
		{"provider":"local-simulator","ok":true,"passed":true,"tvd":0.002},
		{"provider":"qiskit-aer-simulator","ok":true,"passed":false,"tvd":0.18}
	]`
	indexed := indexCanaryResults(raw, []string{"local-simulator", "qiskit-aer-simulator", "ibm-quantum"})
	if len(indexed) != 3 {
		t.Fatalf("expected 3 indexed entries, got %d: %v", len(indexed), indexed)
	}
	decode := func(key string) map[string]any {
		t.Helper()
		var out map[string]any
		if err := json.Unmarshal(indexed[key], &out); err != nil {
			t.Fatalf("unmarshal %s: %v", key, err)
		}
		return out
	}
	localMap := decode("local-simulator")
	if localMap["provider"] != "local-simulator" || localMap["passed"] != true {
		t.Fatalf("local-simulator entry must be its own dict, got %v", localMap)
	}
	aerMap := decode("qiskit-aer-simulator")
	if aerMap["provider"] != "qiskit-aer-simulator" || aerMap["passed"] != false {
		t.Fatalf("qiskit-aer-simulator entry must be its own dict, got %v", aerMap)
	}
	ibmMap := decode("ibm-quantum")
	if ibmMap["ok"] != false {
		t.Fatalf("omitted provider must record a fail-closed dict, got %v", ibmMap)
	}
}
