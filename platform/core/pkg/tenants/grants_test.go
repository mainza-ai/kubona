package tenants

// Phase-2 cross-tenant grant tests: the allow/deny matrix.
//
// Contract (mirrored in platform/tests/test_phase2_grants.py — same matrix,
// same fixed vector, byte-identical canonical payloads):
//
//	issue -> use allows | no grant denies (default deny) | revoke -> reuse
//	denies | expired denies | out-of-scope denies | foreign-grantor forgery
//	refuses | revoked-grant reuse refuses | wildcard scope refuses at ISSUE.

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// Fixed cross-language vector (seed -> keypair -> payload -> signature).
// The Python suite asserts the same bytes verify there.
const (
	vectorSeedHex = "9d61b19deffd5a60ba844af492ec2af44449c5697b326919703bac031cae7f60"
	vectorPubHex  = "b9ce0f24f866c0fa02ab7c1ff345f9bb7c73205b4c2ec76a77295ef16592dc0d"
	vectorSource  = "lab-owner"
	vectorTarget  = "lab-guest"
	vectorScope   = "read"
	vectorExpiry  = "2030-01-01T00:00:00Z"
	vectorSigHex  = "e7c204b282be93c2e7727dd71a0d96b9b96564c5b87f15eaf28385a7b31b584dffd3c46d945e8613747883074018bef665a36449b0555fa667d8547ae21abc08"
	vectorID      = "8014c4d29d1218b5000be9fb8b1941b81a869b13918620b5beb14fe1c5989130"
	vectorRevoke  = "a414c227aa1fbea0aa19d4f0aff92c56c5d53a4304ff6293982f422880850c179f207481834f5c8a5ebca424ac634edf4bc613d9c326218cfb9d72926d6d3906"
)

func vectorPriv(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	seed, err := hex.DecodeString(vectorSeedHex)
	if err != nil {
		t.Fatalf("vector seed: %v", err)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestGrantVectorCanonicalPayload(t *testing.T) {
	exp, err := time.Parse("2006-01-02T15:04:05Z", vectorExpiry)
	if err != nil {
		t.Fatalf("vector expiry: %v", err)
	}
	got := string(GrantCanonicalPayload(vectorSource, vectorTarget, []string{vectorScope}, exp))
	want := "grant|lab-owner|lab-guest|read|2030-01-01T00:00:00Z"
	if got != want {
		t.Fatalf("canonical payload mismatch:\n got %q\nwant %q", got, want)
	}
	sig, err := hex.DecodeString(vectorSigHex)
	if err != nil {
		t.Fatalf("vector sig: %v", err)
	}
	pub, err := hex.DecodeString(vectorPubHex)
	if err != nil {
		t.Fatalf("vector pub: %v", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), []byte(got), sig) {
		t.Fatal("fixed vector signature must verify (cross-language contract)")
	}
	if id := GrantIDFor([]byte(got), sig); id != vectorID {
		t.Fatalf("grant id mismatch: got %s want %s", id, vectorID)
	}
	// The revoke payload for the vector grant verifies too.
	rsig, _ := hex.DecodeString(vectorRevoke)
	if !ed25519.Verify(ed25519.PublicKey(pub), RevokeCanonicalPayload(vectorID, "drill"), rsig) {
		t.Fatal("fixed revoke signature must verify (cross-language contract)")
	}
}

// payloadHas reports whether a ledger payload names field == value
// (distinguishes tenant_read grant events from UCAN spend-grant events
// sharing the registered grant_issued/grant_revoked types).
func payloadHas(raw json.RawMessage, field, value string) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, _ := m[field].(string)
	return v == value
}

// setupGrantLabs registers source + target labs with bootstrapped admins.
func setupGrantLabs(t *testing.T) (*Manager, func(), ed25519.PrivateKey, string) {
	t.Helper()
	m, cleanup := setupManager(t)
	for _, lab := range []string{"lab-owner", "lab-guest"} {
		if _, err := m.CreateTenant(lab, lab+" lab"); err != nil {
			t.Fatalf("create %s: %v", lab, err)
		}
	}
	ownerPub, ownerPriv, _ := ed25519.GenerateKey(nil)
	if _, err := m.SetAdminKey("lab-owner", pubHex(ownerPub), nil); err != nil {
		t.Fatalf("bootstrap owner: %v", err)
	}
	guestPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := m.SetAdminKey("lab-guest", pubHex(guestPub), nil); err != nil {
		t.Fatalf("bootstrap guest: %v", err)
	}
	return m, cleanup, ownerPriv, pubHex(ownerPub)
}

func issueFor(t *testing.T, m *Manager, priv ed25519.PrivateKey, pubHexStr, source, target string, scopes []string, expiry time.Time) *Grant {
	t.Helper()
	canonical := GrantCanonicalPayload(source, target, scopes, expiry)
	sig := ed25519.Sign(priv, canonical)
	g, err := m.IssueGrant(source, target, scopes, expiry, pubHexStr, hex.EncodeToString(sig))
	if err != nil {
		t.Fatalf("issue %s -> %s %v: %v", source, target, scopes, err)
	}
	return g
}

func TestGrantIssueUseMatrix(t *testing.T) {
	m, cleanup, ownerPriv, ownerPubHex := setupGrantLabs(t)
	defer cleanup()

	// No grant = no access (default deny preserved).
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err == nil {
		t.Fatal("cross-tenant read without a grant must refuse")
	}

	// Issue -> use allows.
	g := issueFor(t, m, ownerPriv, ownerPubHex, "lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(time.Hour))
	if g.SourceTenant != "lab-owner" || g.TargetTenant != "lab-guest" {
		t.Fatalf("grant parties wrong: %+v", g)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err != nil {
		t.Fatalf("granted read must allow: %v", err)
	}
	// Out-of-scope refuses (grant covers read, not write).
	if err := m.CheckGrant("lab-owner", "lab-guest", "", "write"); err == nil {
		t.Fatal("out-of-scope use must refuse")
	}
	// Wrong grantee refuses.
	if err := m.CheckGrant("lab-owner", "lab-stranger", "", ReadScope); err == nil {
		t.Fatal("grant for another grantee must refuse")
	}
	// Wrong direction refuses (guest cannot read owner under owner's grant reversed).
	if err := m.CheckGrant("lab-guest", "lab-owner", "", ReadScope); err == nil {
		t.Fatal("reversed direction must refuse")
	}
	// Revoke -> reuse refuses (immediately, next check).
	revSig := ed25519.Sign(ownerPriv, RevokeCanonicalPayload(g.GrantID, "drill"))
	if err := m.RevokeGrant("lab-owner", g.GrantID, "drill", ownerPubHex, hex.EncodeToString(revSig)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err == nil {
		t.Fatal("revoked-grant reuse must refuse")
	}
	// Re-issue after revoke allows again (fresh lifecycle = fresh bytes:
	// revocation is sticky — re-presenting the IDENTICAL blob stays dead,
	// so the new grant carries a new expiry).
	replayed := g
	if _, err := m.IssueGrant(replayed.SourceTenant, replayed.TargetTenant, replayed.Scopes, replayed.Expiry, replayed.GrantorPubkeyHex, replayed.SignatureHex); err != nil {
		t.Fatalf("replay issue: %v", err)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err == nil {
		t.Fatal("revocation must be sticky: replaying the revoked blob must stay dead")
	}
	issueFor(t, m, ownerPriv, ownerPubHex, "lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(2*time.Hour))
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err != nil {
		t.Fatalf("fresh grant after revoke must allow: %v", err)
	}
}

func TestGrantExpiryEnforced(t *testing.T) {
	m, cleanup, ownerPriv, ownerPubHex := setupGrantLabs(t)
	defer cleanup()

	// Issuing an already-expired grant refuses at issue time.
	canonical := GrantCanonicalPayload("lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(-time.Minute))
	sig := ed25519.Sign(ownerPriv, canonical)
	if _, err := m.IssueGrant("lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(-time.Minute), ownerPubHex, hex.EncodeToString(sig)); err == nil {
		t.Fatal("already-expired grant must refuse at issue")
	}

	// A short-lived grant stops working after expiry (live re-check, no TTL cache).
	g := issueFor(t, m, ownerPriv, ownerPubHex, "lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(2*time.Second))
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err != nil {
		t.Fatalf("fresh grant must allow: %v", err)
	}
	// Force expiry by rewriting the row (deterministic, no sleeping).
	if _, err := m.ledger.DB().Exec(`UPDATE tenant_grants SET expiry = ? WHERE grant_id = ?`,
		time.Now().UTC().Add(-time.Second).Format("2006-01-02T15:04:05Z"), g.GrantID); err != nil {
		t.Fatalf("force expiry: %v", err)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err == nil {
		t.Fatal("expired grant must refuse")
	}
}

func TestGrantForeignGrantorForgery(t *testing.T) {
	m, cleanup, _, _ := setupGrantLabs(t)
	defer cleanup()

	// Attacker self-signs a grant claiming to be lab-owner. The signature
	// verifies against the ATTACKER key, but the source admin key is someone
	// else's: issue refuses (ErrNotAdmin), and even a planted row would fail
	// the live check (signature re-verified against the current admin key).
	evilPub, evilPriv, _ := ed25519.GenerateKey(nil)
	expiry := time.Now().UTC().Add(time.Hour)
	canonical := GrantCanonicalPayload("lab-owner", "lab-guest", []string{ReadScope}, expiry)
	sig := ed25519.Sign(evilPriv, canonical)
	if _, err := m.IssueGrant("lab-owner", "lab-guest", []string{ReadScope}, expiry, pubHex(evilPub), hex.EncodeToString(sig)); err == nil {
		t.Fatal("foreign-grantor forgery must refuse at issue")
	} else if err != ErrNotAdmin && err != ErrGrantSignature {
		t.Fatalf("forgery must refuse as not-admin/signature, got: %v", err)
	}

	// Planted-row defense in depth: a row smuggled past issue still fails
	// the live check, which re-verifies against the CURRENT source admin key.
	owner, err := m.GetTenant("lab-owner")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	_ = owner
	fakeID := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if _, err := m.ledger.DB().Exec(
		`INSERT INTO tenant_grants (grant_id, source_tenant, target_tenant, scopes_csv, expiry, grantor_pubkey_hex, signature_hex, revoked, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		fakeID, "lab-owner", "lab-guest", ReadScope,
		expiry.Format("2006-01-02T15:04:05Z"), pubHex(evilPub), hex.EncodeToString(sig), time.Now().UTC()); err != nil {
		t.Fatalf("plant row: %v", err)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err == nil {
		t.Fatal("planted forged row must fail the live signature check")
	}
}

func TestGrantWildcardScopeRefusedAtIssue(t *testing.T) {
	m, cleanup, ownerPriv, ownerPubHex := setupGrantLabs(t)
	defer cleanup()
	expiry := time.Now().UTC().Add(time.Hour)

	for _, scopes := range [][]string{{"*"}, {ReadScope, "*"}, {"read:*"}, {"*write"}} {
		canonical := GrantCanonicalPayload("lab-owner", "lab-guest", scopes, expiry)
		sig := ed25519.Sign(ownerPriv, canonical)
		if _, err := m.IssueGrant("lab-owner", "lab-guest", scopes, expiry, ownerPubHex, hex.EncodeToString(sig)); err == nil {
			t.Fatalf("wildcard scopes %v must refuse at issue", scopes)
		}
	}
	// Empty scope list refuses too (an empty allowlist authorizes nothing-checkable).
	canonical := GrantCanonicalPayload("lab-owner", "lab-guest", nil, expiry)
	sig := ed25519.Sign(ownerPriv, canonical)
	if _, err := m.IssueGrant("lab-owner", "lab-guest", nil, expiry, ownerPubHex, hex.EncodeToString(sig)); err == nil {
		t.Fatal("empty scope list must refuse at issue")
	}
	// Grants to ghost tenants refuse (no grants into the void).
	canonical = GrantCanonicalPayload("lab-owner", "lab-ghost", []string{ReadScope}, expiry)
	sig = ed25519.Sign(ownerPriv, canonical)
	if _, err := m.IssueGrant("lab-owner", "lab-ghost", []string{ReadScope}, expiry, ownerPubHex, hex.EncodeToString(sig)); err == nil {
		t.Fatal("grant to unknown tenant must refuse")
	}
}

func TestGrantRevokeAuthorization(t *testing.T) {
	m, cleanup, ownerPriv, ownerPubHex := setupGrantLabs(t)
	defer cleanup()
	g := issueFor(t, m, ownerPriv, ownerPubHex, "lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(time.Hour))

	// Only the ISSUING lab revokes: the grantee's admin cannot revoke.
	guest, err := m.GetTenant("lab-guest")
	if err != nil {
		t.Fatalf("guest: %v", err)
	}
	_ = guest
	evilPub, evilPriv, _ := ed25519.GenerateKey(nil)
	_ = evilPub
	revSig := ed25519.Sign(evilPriv, RevokeCanonicalPayload(g.GrantID, "spoof"))
	if err := m.RevokeGrant("lab-owner", g.GrantID, "spoof", pubHex(evilPub), hex.EncodeToString(revSig)); err == nil {
		t.Fatal("foreign revocation must refuse")
	}
	// Revoking an unknown grant refuses (no silent success).
	revSig = ed25519.Sign(ownerPriv, RevokeCanonicalPayload("no-such-grant", "x"))
	if err := m.RevokeGrant("lab-owner", "no-such-grant", "x", ownerPubHex, hex.EncodeToString(revSig)); err == nil {
		t.Fatal("revoking an unknown grant must refuse")
	}
	// The grant still works after refused revocations.
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err != nil {
		t.Fatalf("grant must survive refused revocations: %v", err)
	}
}

func TestGrantPrincipalTarget(t *testing.T) {
	m, cleanup, ownerPriv, ownerPubHex := setupGrantLabs(t)
	defer cleanup()

	// A principal-bound grant covers exactly that principal, not the lab.
	g := issueFor(t, m, ownerPriv, ownerPubHex, "lab-owner", "sub:alice", []string{ReadScope}, time.Now().UTC().Add(time.Hour))
	if g.TargetTenant != "sub:alice" {
		t.Fatalf("principal target not stored: %+v", g)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "alice", ReadScope); err != nil {
		t.Fatalf("principal-bound grant must allow that principal: %v", err)
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "bob", ReadScope); err == nil {
		t.Fatal("principal-bound grant must not cover another principal")
	}
	if err := m.CheckGrant("lab-owner", "lab-guest", "", ReadScope); err == nil {
		t.Fatal("principal-bound grant must not cover the bare tenant")
	}
}

func TestGrantIssueSealedInLedger(t *testing.T) {
	m, cleanup, ownerPriv, ownerPubHex := setupGrantLabs(t)
	defer cleanup()
	g := issueFor(t, m, ownerPriv, ownerPubHex, "lab-owner", "lab-guest", []string{ReadScope}, time.Now().UTC().Add(time.Hour))

	events, err := m.ledger.GetRecentEvents("lab-owner", 20)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	foundIssue := false
	for _, e := range events {
		if e.EventType == "grant_issued" && payloadHas(e.Payload, "grant_id", g.GrantID) {
			foundIssue = true
		}
	}
	if !foundIssue {
		t.Fatal("issue must seal grant_issued (kind tenant_read) in the ledger")
	}
	revSig := ed25519.Sign(ownerPriv, RevokeCanonicalPayload(g.GrantID, "done"))
	if err := m.RevokeGrant("lab-owner", g.GrantID, "done", ownerPubHex, hex.EncodeToString(revSig)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	events, err = m.ledger.GetRecentEvents("lab-owner", 20)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	foundRevoke := false
	for _, e := range events {
		if e.EventType == "grant_revoked" && payloadHas(e.Payload, "grant_id", g.GrantID) {
			foundRevoke = true
		}
	}
	if !foundRevoke {
		t.Fatal("revoke must seal grant_revoked in the ledger")
	}
}
