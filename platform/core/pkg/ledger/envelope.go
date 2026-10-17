package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// R1 — canonical evidence envelope: one receipt identity across both
// ledgers. Mirrors kubona/provenance/envelope.py exactly; the checked-in
// vectors (platform/contracts/evidence-vectors.json) pin both rails, so
// any canonicalization drift turns either suite red.
//
// Canonical bytes: sorted keys (encoding/json sorts map keys), compact
// (Encoder with SetEscapeHTML(false) — Go's default HTML escaping would
// fork bytes vs Python), UTF-8, finite floats only (encoding/json already
// refuses NaN/Inf — fail-closed for free).

// canonicalJSON marshals with sorted keys, no HTML escaping, no spaces.
func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Envelope is the sealed receipt. RawResult/MandateRef are nil when absent
// (sealed as JSON null, exactly like Python's None). Note: the seal is
// computed over the body map, never by marshaling this struct — field
// order stays out of the picture entirely.
type Envelope struct {
	EvidenceID       string         `json:"evidence_id"`
	NormalizedResult map[string]any `json:"normalized_result"`
	RawResult        map[string]any `json:"raw_result"`
	AnalysisProfile  map[string]any `json:"analysis_profile"`
	TenantID         string         `json:"tenant_id"`
	MandateRef       map[string]any `json:"mandate_ref"`
}

func envelopeBody(normalized, raw, profile map[string]any, tenant string, mandate map[string]any) map[string]any {
	return map[string]any{
		"analysis_profile":  profile,
		"mandate_ref":       anyMapOrNil(mandate),
		"normalized_result": normalized,
		"raw_result":        anyMapOrNil(raw),
		"tenant_id":         tenant,
	}
}

func anyMapOrNil(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}

// BuildEnvelope seals an envelope, refusing (error) on missing bindings:
// non-empty normalized_result, non-empty analysis_profile with non-empty
// method, non-empty tenant_id. Whole numbers must be ints on both rails
// (25, not 25.0) — see envelope.py for why.
func BuildEnvelope(normalized, raw, profile map[string]any, tenant string, mandate map[string]any) (*Envelope, error) {
	if len(normalized) == 0 {
		return nil, fmt.Errorf("envelope needs a non-empty normalized_result object")
	}
	if len(profile) == 0 {
		return nil, fmt.Errorf("envelope needs a non-empty analysis_profile (REA rule)")
	}
	method, _ := profile["method"].(string)
	if method == "" {
		return nil, fmt.Errorf("analysis_profile.method must be a non-empty string")
	}
	if tenant == "" {
		return nil, fmt.Errorf("envelope needs a non-empty tenant_id")
	}
	if raw != nil && len(raw) == 0 {
		raw = nil
	}
	if mandate != nil && len(mandate) == 0 {
		return nil, fmt.Errorf("mandate_ref must be a non-empty object when present")
	}
	body := envelopeBody(normalized, raw, profile, tenant, mandate)
	sealed, err := canonicalJSON(body)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(sealed)
	return &Envelope{
		EvidenceID:       "ev_" + hex.EncodeToString(sum[:]),
		NormalizedResult: normalized,
		RawResult:        raw,
		AnalysisProfile:  profile,
		TenantID:         tenant,
		MandateRef:       mandate,
	}, nil
}

// VerifyEnvelope recomputes + revalidates. Returns nil on a sealed envelope,
// error on tamper, foreign bytes, or missing bindings.
func VerifyEnvelope(env *Envelope) error {
	if env == nil {
		return fmt.Errorf("envelope must be an object")
	}
	if !strings.HasPrefix(env.EvidenceID, "ev_") || len(env.EvidenceID) != 67 {
		return fmt.Errorf("envelope evidence_id malformed")
	}
	rebuilt, err := BuildEnvelope(env.NormalizedResult, env.RawResult, env.AnalysisProfile, env.TenantID, env.MandateRef)
	if err != nil {
		return err
	}
	if rebuilt.EvidenceID != env.EvidenceID {
		return fmt.Errorf("envelope evidence_id does not match content (tampered or foreign bytes)")
	}
	return nil
}

// (encoding/json refuses NaN/Inf at marshal time with "unsupported value"
// — the non-finite refusal is fail-closed for free on this rail;
// envelope.py raises explicitly on the Python rail.)
