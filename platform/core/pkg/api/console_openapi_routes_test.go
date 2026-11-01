package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// console-openapi.json is generated from the console client. If a generated
// path or method has no registered RoutePolicies row, the client can drift
// into a 404 while its own contract check stays green. This is the reverse
// direction of route_policy_test.go (registered -> classified).
func TestConsoleOpenAPIRoutesExist(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/console-openapi.json")
	if err != nil {
		t.Fatalf("read console OpenAPI: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode console OpenAPI: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("console OpenAPI has no paths")
	}
	for path, operations := range doc.Paths {
		for method := range operations {
			if !routePolicyServes(path, method) {
				t.Errorf("console operation %s %s has no registered route policy", strings.ToUpper(method), path)
			}
		}
	}
}

func routePolicyServes(path, method string) bool {
	for _, policy := range RoutePolicies {
		if !policyServesMethod(policy.Methods, method) {
			continue
		}
		if path == policy.Pattern {
			return true
		}
		if strings.HasSuffix(policy.Pattern, "/") && strings.HasPrefix(path, policy.Pattern) {
			return true
		}
	}
	return false
}

func policyServesMethod(methods, method string) bool {
	for _, candidate := range strings.Split(methods, ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), method) {
			return true
		}
	}
	return false
}
