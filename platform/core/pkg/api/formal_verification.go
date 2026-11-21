package api

// Signed comparator attestation ingestion (A3).
//
// The Lean comparator runs in the pinned Docker sandbox on the operator's
// host (platform/kubona/eval/comparator_cli.py). It signs the canonical
// payload with a dedicated comparator key; this endpoint verifies that
// signature and appends a `formal_verification` event. Neither the console
// nor an API caller can forge a kernel-proof verdict: without the configured
// public key the endpoint refuses.
//
// The daemon does not run Lean itself — it has no Docker socket by design.
// The attestation is the boundary between the sandbox and the ledger.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const maxFormalVerificationBody = 256 * 1024

// permittedAxioms is the standard Lean whitelist. The comparator already
// enforces it; this is defense in depth at the trust boundary.
var permittedAxioms = map[string]bool{
	"propext":          true,
	"Quot.sound":       true,
	"Classical.choice": true,
}

func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

func (s *Server) handleFormalVerification(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "formal verification attestations are POST-only", nil)
		return
	}
	if !s.verifyLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	configured := strings.ToLower(strings.TrimSpace(os.Getenv("KUBONA_COMPARATOR_PUBKEY_HEX")))
	if configured == "" {
		WriteAPIError(w, "comparator_unconfigured",
			"no comparator public key is configured; a kernel proof cannot be attested", nil)
		return
	}
	if !s.reviewerKeyConfigured && !ephemeralAllowed() {
		WriteAPIError(w, "server_misconfigured", "reviewer signing key is required to seal formal verifications in production", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormalVerificationBody)
	var body struct {
		Payload      map[string]any `json:"payload"`
		SignatureHex string         `json:"signature_hex"`
		PubkeyHex    string         `json:"pubkey_hex"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid attestation: %v", err), nil)
		return
	}
	if len(body.Payload) == 0 {
		WriteAPIError(w, "bad_request", "attestation payload is required", nil)
		return
	}
	pubHex := strings.ToLower(strings.TrimSpace(body.PubkeyHex))
	if subtle.ConstantTimeCompare([]byte(pubHex), []byte(configured)) != 1 {
		WriteAPIError(w, "unauthorized", "attestation public key does not match the configured comparator key", nil)
		return
	}
	pub, err := hex.DecodeString(pubHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		WriteAPIError(w, "bad_request", "attestation public key is malformed", nil)
		return
	}
	sig, err := hex.DecodeString(strings.TrimSpace(body.SignatureHex))
	if err != nil || len(sig) != ed25519.SignatureSize {
		WriteAPIError(w, "bad_request", "attestation signature is malformed", nil)
		return
	}
	canonical, err := canonicalJSON(body.Payload)
	if err != nil {
		WriteAPIError(w, "bad_request", "attestation payload is not canonicalizable", nil)
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), canonical, sig) {
		WriteAPIError(w, "unauthorized", "attestation signature does not verify", nil)
		return
	}
	if err := validateFormalPayload(body.Payload, sessionID); err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}

	sigTrimmed := strings.ToLower(strings.TrimSpace(body.SignatureHex))
	if prior, perr := s.ledger.GetRecentEvents(sessionID, 500); perr == nil {
		for _, ev := range prior {
			if ev.EventType != "formal_verification" {
				continue
			}
			var prev map[string]any
			if json.Unmarshal([]byte(ev.Payload), &prev) == nil {
				if ps, _ := prev["comparator_signature_hex"].(string); strings.EqualFold(strings.TrimSpace(ps), sigTrimmed) {
					WriteAPIError(w, "conflict", "formal verification attestation already recorded (replay refused)", nil)
					return
				}
			}
		}
	}

	eventPayload := map[string]any{}
	for k, v := range body.Payload {
		eventPayload[k] = v
	}
	eventPayload["comparator_pubkey_hex"] = pubHex
	eventPayload["comparator_signature_hex"] = sigTrimmed
	rec, err := s.ledger.AppendSignedEvent(sessionID, "formal_verification", eventPayload, s.signerPrivKey)
	if err != nil {
		WriteAPIError(w, "internal", "formal verification event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "proof_hash": body.Payload["proof_hash"],
		"event_id": rec.ID, "entry_hash": rec.EntryHash,
	})
}

func validateFormalPayload(payload map[string]any, sessionID string) error {
	if kind, _ := payload["kind"].(string); kind != "formal_verification" {
		return fmt.Errorf("payload kind must be formal_verification")
	}
	if sid, _ := payload["session_id"].(string); sid != sessionID {
		return fmt.Errorf("payload session_id does not match the request session")
	}
	challenge, _ := payload["challenge"].(string)
	if strings.TrimSpace(challenge) == "" {
		return fmt.Errorf("payload challenge is required")
	}
	proofHash, _ := payload["proof_hash"].(string)
	if len(proofHash) != 64 {
		return fmt.Errorf("proof_hash must be a 64-character SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(proofHash); err != nil {
		return fmt.Errorf("proof_hash is not hex")
	}
	if verdict, _ := payload["verdict"].(string); verdict != "live_kernel_proven" {
		return fmt.Errorf("verdict must be live_kernel_proven")
	}
	if n, ok := payload["sorry_ax_count"].(float64); !ok || n != 0 {
		return fmt.Errorf("sorry_ax_count must be exactly 0")
	}
	if clean, ok := payload["axioms_clean"].(bool); !ok || !clean {
		return fmt.Errorf("axioms_clean must be true")
	}
	axioms, ok := payload["axioms_used"].([]any)
	if !ok {
		return fmt.Errorf("axioms_used must be an array of axiom names")
	}
	for _, raw := range axioms {
		name, ok := raw.(string)
		if !ok || !permittedAxioms[name] {
			return fmt.Errorf("axiom %q is outside the permitted whitelist", name)
		}
	}
	theorems, ok := payload["theorem_names"].([]any)
	if !ok || len(theorems) == 0 {
		return fmt.Errorf("theorem_names must name at least one checked theorem")
	}
	for _, raw := range theorems {
		name, ok := raw.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("theorem_names entries must be non-empty strings")
		}
	}
	return nil
}
