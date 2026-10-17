package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvelopeVectors asserts byte-exact agreement with the shared vectors:
// same evidence_id AND same canonical JSON as the Python rail produced.
func TestEnvelopeVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "evidence-vectors.json"))
	if err != nil {
		t.Fatalf("reading evidence vectors: %v", err)
	}
	var doc struct {
		Version  int `json:"version"`
		Fixtures []struct {
			Name          string         `json:"name"`
			Inputs        map[string]any `json:"inputs"`
			EvidenceID    string         `json:"evidence_id"`
			CanonicalJSON string         `json:"canonical_json"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing evidence vectors: %v", err)
	}
	if doc.Version != 1 {
		t.Fatalf("vectors version = %d, this code knows v1", doc.Version)
	}
	asMap := func(v any) map[string]any {
		if v == nil {
			return nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("vector field is not an object: %T", v)
		}
		return m
	}
	for _, fx := range doc.Fixtures {
		in := fx.Inputs
		env, err := BuildEnvelope(
			asMap(in["normalized_result"]),
			asMap(in["raw_result"]),
			asMap(in["analysis_profile"]),
			stringVal(in["tenant_id"]),
			asMap(in["mandate_ref"]),
		)
		if err != nil {
			t.Fatalf("%s: build: %v", fx.Name, err)
		}
		if env.EvidenceID != fx.EvidenceID {
			t.Errorf("%s: evidence_id mismatch:\n got %s\nwant %s", fx.Name, env.EvidenceID, fx.EvidenceID)
		}
		body := envelopeBody(env.NormalizedResult, env.RawResult, env.AnalysisProfile, env.TenantID, env.MandateRef)
		canon, err := canonicalJSON(body)
		if err != nil {
			t.Fatalf("%s: canonicalize: %v", fx.Name, err)
		}
		if string(canon) != fx.CanonicalJSON {
			t.Errorf("%s: canonical bytes mismatch:\n got %s\nwant %s", fx.Name, canon, fx.CanonicalJSON)
		}
		if err := VerifyEnvelope(env); err != nil {
			t.Errorf("%s: sealed envelope must verify: %v", fx.Name, err)
		}
	}
}

func stringVal(v any) string {
	s, _ := v.(string)
	return s
}

func TestEnvelopeRefusals(t *testing.T) {
	if _, err := BuildEnvelope(nil, nil, map[string]any{"method": "m"}, "lab", nil); err == nil {
		t.Error("empty normalized_result must refuse")
	}
	if _, err := BuildEnvelope(map[string]any{"x": 1}, nil, nil, "lab", nil); err == nil {
		t.Error("missing analysis_profile must refuse")
	}
	if _, err := BuildEnvelope(map[string]any{"x": 1}, nil, map[string]any{"backend": "b"}, "lab", nil); err == nil {
		t.Error("profile without method must refuse")
	}
	if _, err := BuildEnvelope(map[string]any{"x": 1}, nil, map[string]any{"method": "m"}, "", nil); err == nil {
		t.Error("empty tenant_id must refuse")
	}
	env, err := BuildEnvelope(map[string]any{"x": 1}, nil, map[string]any{"method": "m"}, "lab", nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	tampered := *env
	tampered.NormalizedResult = map[string]any{"x": 2}
	if err := VerifyEnvelope(&tampered); err == nil {
		t.Error("tampered content must refuse")
	}
	wrongID := *env
	wrongID.EvidenceID = "ev_" + strings.Repeat("0", 64)
	if err := VerifyEnvelope(&wrongID); err == nil {
		t.Error("wrong evidence_id must refuse")
	}
}
