package grants

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func testKeys(t *testing.T) (privHex, pubHex, didStr string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	privHex = hex.EncodeToString(priv)
	pubHex = hex.EncodeToString(pub)
	didStr, err = DIDForAdminPubkey(pubHex)
	if err != nil {
		t.Fatalf("did: %v", err)
	}
	return privHex, pubHex, didStr
}

func TestGrantMintAttenuateVerify(t *testing.T) {
	privA, _, didA := testKeys(t)
	privB, _, didB := testKeys(t)
	_, _, didAgent := testKeys(t)

	rootSealed, rootCID, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit, ScopeRead}, 100.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if rootCID == "" || len(rootSealed) == 0 {
		t.Fatalf("empty mint output")
	}

	// Attenuation is signed by the grantee (lab-b) key, narrowing command,
	// budget, and TTL for the agent.
	leafSealed, leafCID, err := AttenuateGrant(rootSealed, privB, didAgent,
		"qpu/submit", []string{ScopeQPUSubmit}, 25.0, 10*time.Minute)
	if err != nil {
		t.Fatalf("attenuate: %v", err)
	}

	vg, err := VerifyChain([][]byte{rootSealed, leafSealed}, VerifyOptions{
		TrustedRoots:  map[string]string{didA: "lab-a"},
		RequiredScope: ScopeQPUSubmit,
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if vg.GrantingTenant != "lab-a" || vg.TargetTenant != "lab-b" || vg.MaxBudgetUSD != 25.0 {
		t.Fatalf("wrong grant terms: %+v", vg)
	}
	if vg.RootCID != rootCID || vg.LeafCID != leafCID {
		t.Fatalf("CID mismatch: %+v", vg)
	}
	if len(vg.Scopes) != 1 || vg.Scopes[0] != ScopeQPUSubmit {
		t.Fatalf("wrong scopes: %+v", vg)
	}
}

func TestGrantRefusals(t *testing.T) {
	privA, _, didA := testKeys(t)
	privB, _, didB := testKeys(t)
	_, _, didEvil := testKeys(t)

	rootSealed, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit}, 100.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	roots := map[string]string{didA: "lab-a"}

	// Untrusted root.
	if _, err := VerifyChain([][]byte{rootSealed}, VerifyOptions{
		TrustedRoots: map[string]string{didEvil: "lab-evil"},
	}); err == nil {
		t.Fatalf("untrusted root must refuse")
	}

	// Widening attenuation refuses at mint time.
	if _, _, err := AttenuateGrant(rootSealed, privB, didB, "admin/wipe", []string{ScopeQPUSubmit}, 10.0, time.Minute); err == nil {
		t.Fatalf("command widening must refuse")
	}
	// Budget raise refuses.
	if _, _, err := AttenuateGrant(rootSealed, privB, didB, "qpu/submit", []string{ScopeQPUSubmit}, 500.0, time.Minute); err == nil {
		t.Fatalf("budget raise must refuse")
	}
	// Scope escape refuses.
	if _, _, err := AttenuateGrant(rootSealed, privB, didB, "qpu/submit", []string{"admin:wipe"}, 10.0, time.Minute); err == nil {
		t.Fatalf("scope escape must refuse")
	}

	// Expired chain refuses.
	shortSealed, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit}, 100.0, time.Millisecond)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := VerifyChain([][]byte{shortSealed}, VerifyOptions{TrustedRoots: roots}); err == nil {
		t.Fatalf("expired grant must refuse")
	}

	// Revoked CID refuses.
	revSealed, revCID, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit}, 100.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, err := VerifyChain([][]byte{revSealed}, VerifyOptions{
		TrustedRoots: roots, Revoked: map[string]bool{revCID: true},
	}); err == nil {
		t.Fatalf("revoked grant must refuse")
	}

	// Tampered bytes refuse.
	tampered := append([]byte(nil), rootSealed...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := VerifyChain([][]byte{tampered}, VerifyOptions{TrustedRoots: roots}); err == nil {
		t.Fatalf("tampered envelope must refuse")
	}

	// Required scope not covered refuses.
	if _, err := VerifyChain([][]byte{rootSealed}, VerifyOptions{
		TrustedRoots: roots, RequiredScope: "admin:wipe",
	}); err == nil {
		t.Fatalf("uncovered scope must refuse")
	}

	// Empty and over-deep chains refuse.
	if _, err := VerifyChain(nil, VerifyOptions{TrustedRoots: roots}); err == nil {
		t.Fatalf("empty chain must refuse")
	}
	deep := make([][]byte, MaxChainDepth+1)
	for i := range deep {
		deep[i] = rootSealed
	}
	if _, err := VerifyChain(deep, VerifyOptions{TrustedRoots: roots}); err == nil {
		t.Fatalf("over-deep chain must refuse")
	}
}

func TestDIDForAdminPubkeyStable(t *testing.T) {
	_, pubHex, didStr := testKeys(t)
	again, err := DIDForAdminPubkey(pubHex)
	if err != nil || again != didStr {
		t.Fatalf("DID derivation unstable: %q vs %q err=%v", again, didStr, err)
	}
	if len(didStr) < len("did:key:z") || didStr[:8] != "did:key:" {
		t.Fatalf("not a did:key DID: %q", didStr)
	}
	if _, err := DIDForAdminPubkey("zzzz"); err == nil {
		t.Fatalf("bad hex must refuse")
	}
}

// W0.8: mint-time validation — empty scopes, slash scopes, and bad TTLs
// refuse at mint, never as known-dead envelopes.
func TestGrantMintValidation(t *testing.T) {
	privA, _, didA := testKeys(t)
	_, _, didB := testKeys(t)
	_ = didA
	if _, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		nil, 100.0, time.Hour); err == nil {
		t.Fatalf("empty scopes must refuse")
	}
	if _, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{""}, 100.0, time.Hour); err == nil {
		t.Fatalf("blank scope must refuse")
	}
	if _, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{"qpu/submit", "a/b"}, 100.0, time.Hour); err == nil {
		t.Fatalf("slash scope must refuse (collides with a:b in scopePath)")
	}
	if _, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit}, 100.0, 0); err == nil {
		t.Fatalf("zero TTL must refuse")
	}
	if _, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit}, 100.0, 91*24*time.Hour); err == nil {
		t.Fatalf("over-90d TTL must refuse")
	}
}

// W0.8: attenuation is the grantee's act — a stranger minting "down" the
// chain refuses at mint time (VerifyChain would refuse it at level i;
// fail-early instead of minting known-dead envelopes).
func TestGrantAttenuateIssuerMustBeAudience(t *testing.T) {
	privA, _, _ := testKeys(t)
	privB, _, didB := testKeys(t)
	privE, _, didEvil := testKeys(t)

	rootSealed, _, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit, ScopeRead}, 100.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// Stranger (not the parent audience) attenuating refuses.
	if _, _, err := AttenuateGrant(rootSealed, privE, didEvil,
		"qpu/submit", []string{ScopeQPUSubmit}, 10.0, time.Minute); err == nil {
		t.Fatalf("non-audience attenuation must refuse")
	}
	// Bad TTL refuses.
	if _, _, err := AttenuateGrant(rootSealed, privB, didB,
		"qpu/submit", []string{ScopeQPUSubmit}, 10.0, 0); err == nil {
		t.Fatalf("zero-TTL attenuation must refuse")
	}
	// Empty scopes refuse (empty is a subset of everything — narrowing to
	// nothing must be explicit refusal, not silent).
	if _, _, err := AttenuateGrant(rootSealed, privB, didB,
		"qpu/submit", nil, 10.0, time.Minute); err == nil {
		t.Fatalf("scope-emptying attenuation must refuse")
	}
}

// W0.8: leaf revocation is surgical — revoking the leaf kills chains
// containing that envelope while the root-only chain still verifies.
func TestGrantLeafRevocationSurgical(t *testing.T) {
	privA, _, didA := testKeys(t)
	privB, _, didB := testKeys(t)
	_, _, didAgent := testKeys(t)

	rootSealed, rootCID, err := MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{ScopeQPUSubmit, ScopeRead}, 100.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	leafSealed, leafCID, err := AttenuateGrant(rootSealed, privB, didAgent,
		"qpu/submit", []string{ScopeQPUSubmit}, 25.0, 10*time.Minute)
	if err != nil {
		t.Fatalf("attenuate: %v", err)
	}
	roots := map[string]string{didA: "lab-a"}
	full := [][]byte{rootSealed, leafSealed}

	// Leaf revoked: full chain refuses, root alone still verifies.
	revoked := map[string]bool{leafCID: true}
	if _, err := VerifyChain(full, VerifyOptions{TrustedRoots: roots, Revoked: revoked}); err == nil {
		t.Fatalf("leaf-revoked chain must refuse")
	}
	if _, err := VerifyChain([][]byte{rootSealed}, VerifyOptions{TrustedRoots: roots, Revoked: revoked}); err != nil {
		t.Fatalf("root-only chain must survive leaf revocation: %v", err)
	}
	// Root revoked: everything refuses (unchanged whole-chain semantics).
	revokedRoot := map[string]bool{rootCID: true}
	if _, err := VerifyChain(full, VerifyOptions{TrustedRoots: roots, Revoked: revokedRoot}); err == nil {
		t.Fatalf("root-revoked chain must refuse")
	}
	_ = rootCID
}

func TestRevokedCIDsFailsClosedOnUnreadableRegistry(t *testing.T) {
	db, err := sql.Open("sqlite3", t.TempDir()+"/revoked.db")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := RevokedCIDs(db); err == nil {
		t.Fatal("closed registry must return an error, not an empty allow-list")
	}
}
