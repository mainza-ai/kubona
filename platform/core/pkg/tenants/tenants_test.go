package tenants

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func setupManager(t *testing.T) (*Manager, func()) {
	t.Helper()
	dbPath := "test_tenants.db"
	os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	m := NewManager(l)
	cleanup := func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return m, cleanup
}

func TestTenantLifecycle(t *testing.T) {
	m, cleanup := setupManager(t)
	defer cleanup()

	// Seeded default lab tenant: L1, default ceiling, no admin yet.
	def, err := m.GetTenant(DefaultTenantID)
	if err != nil {
		t.Fatalf("default tenant must be seeded: %v", err)
	}
	if def.AutonomyLevel != LevelL1 || def.SpendCeilingUSD != DefaultCeilingUSD {
		t.Fatalf("default tenant wrong: %+v", def)
	}

	// Register a lab.
	lab, err := m.CreateTenant("lab-acme", "Acme Lab")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if lab.AutonomyLevel != LevelL1 {
		t.Fatalf("new tenants default L1, got %s", lab.AutonomyLevel)
	}

	// Bootstrap admin key (first write wins).
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminHex := pubHex(adminPub)
	lab, err = m.SetAdminKey("lab-acme", adminHex, nil)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if lab.AdminPubkeyHex != adminHex {
		t.Fatalf("admin key not stored: %+v", lab)
	}

	// Second bootstrap without signature must refuse (rotation needs admin).
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := m.SetAdminKey("lab-acme", pubHex(otherPub), nil); err != ErrAdminKeySet {
		t.Fatalf("expected ErrAdminKeySet, got %v", err)
	}

	// Grant L2 with a signed, bound envelope.
	grantParams := map[string]string{"level": LevelL2}
	act := signed(t, adminPriv, adminHex, "lab-acme", "grant_autonomy", grantParams, 5*time.Minute)
	lab, err = m.GrantAutonomy(act, LevelL2)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if lab.AutonomyLevel != LevelL2 {
		t.Fatalf("expected L2, got %s", lab.AutonomyLevel)
	}

	// Expired envelope must refuse (replay bound).
	stale := signed(t, adminPriv, adminHex, "lab-acme", "grant_autonomy", map[string]string{"level": LevelL3}, -time.Minute)
	if _, err := m.GrantAutonomy(stale, LevelL3); err != ErrActionExpired {
		t.Fatalf("expected ErrActionExpired, got %v", err)
	}

	// Wrong key must refuse.
	evilPub, evilPriv, _ := ed25519.GenerateKey(nil)
	_ = evilPub
	forged := signed(t, evilPriv, pubHex(evilPub), "lab-acme", "grant_autonomy", map[string]string{"level": LevelL3}, 5*time.Minute)
	if _, err := m.GrantAutonomy(forged, LevelL3); err != ErrNotAdmin {
		t.Fatalf("expected ErrNotAdmin, got %v", err)
	}

	// Spend checks against the ceiling.
	if err := m.CheckSpend("lab-acme", 50.0); err != nil {
		t.Fatalf("within ceiling must pass: %v", err)
	}
	if err := m.CheckSpend("lab-acme", 10000.0); err != ErrCeilingExceeded {
		t.Fatalf("expected ErrCeilingExceeded, got %v", err)
	}

	// Revoke drops to L0.
	revParams := map[string]string{"reason": "key compromise drill"}
	rev := signed(t, adminPriv, adminHex, "lab-acme", "revoke_autonomy", revParams, 5*time.Minute)
	lab, err = m.RevokeAutonomy(rev, "key compromise drill")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if lab.AutonomyLevel != LevelL0 {
		t.Fatalf("revoke must drop to L0, got %s", lab.AutonomyLevel)
	}
}

func TestTenantCeilingChange(t *testing.T) {
	m, cleanup := setupManager(t)
	defer cleanup()

	if _, err := m.CreateTenant("lab-beta", "Beta"); err != nil {
		t.Fatalf("create: %v", err)
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	if _, err := m.SetAdminKey("lab-beta", pubHex(pub), nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	act := signed(t, priv, pubHex(pub), "lab-beta", "set_ceiling", map[string]float64{"ceiling_usd": 500}, 5*time.Minute)
	lab, err := m.SetCeiling(act, 500)
	if err != nil {
		t.Fatalf("set ceiling: %v", err)
	}
	if lab.SpendCeilingUSD != 500 {
		t.Fatalf("ceiling not stored: %+v", lab)
	}
}

func pubHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}

func signed(t *testing.T, priv ed25519.PrivateKey, pubHexStr, tenantID, action string, params any, ttl time.Duration) *AdminAction {
	t.Helper()
	hash, err := ParamsHashOf(params)
	if err != nil {
		t.Fatalf("params hash: %v", err)
	}
	a := &AdminAction{
		TenantID:   tenantID,
		Action:     action,
		ParamsHash: hash,
		Expiry:     time.Now().UTC().Add(ttl),
		PubkeyHex:  pubHexStr,
	}
	sig := ed25519.Sign(priv, a.CanonicalPayload())
	a.SignatureHex = hex.EncodeToString(sig)
	return a
}

func TestReserveSpendCumulativeCeiling(t *testing.T) {
	m, cleanup := setupManager(t)
	defer cleanup()

	// Default ceiling is 150: two $100 reservations must not both succeed.
	if err := m.ReserveSpend("default", 100); err != nil {
		t.Fatalf("first reservation must succeed: %v", err)
	}
	if err := m.ReserveSpend("default", 100); err == nil {
		t.Fatal("second $100 reservation on a $150 ceiling must refuse (cumulative)")
	} else if err != ErrCeilingExceeded {
		t.Fatalf("refusal must be the ceiling error, got: %v", err)
	}
	got, err := m.GetTenant("default")
	if err != nil {
		t.Fatal(err)
	}
	if got.SpentUSD != 100 {
		t.Fatalf("spent must be 100 after one kept reservation, got %v", got.SpentUSD)
	}
	// Release returns the budget: a new $100 reservation passes again.
	if err := m.ReleaseSpend("default", 100); err != nil {
		t.Fatal(err)
	}
	if err := m.ReserveSpend("default", 100); err != nil {
		t.Fatalf("reservation after release must succeed: %v", err)
	}
	// Release floors at zero: double-release never mints negative spend.
	if err := m.ReleaseSpend("default", 100); err != nil {
		t.Fatal(err)
	}
	if err := m.ReleaseSpend("default", 100); err != nil {
		t.Fatal(err)
	}
	got, _ = m.GetTenant("default")
	if got.SpentUSD != 0 {
		t.Fatalf("spent must floor at 0, got %v", got.SpentUSD)
	}
}

func TestReserveSpendConcurrentNoOverspend(t *testing.T) {
	m, cleanup := setupManager(t)
	defer cleanup()

	// 10 concurrent $20 reservations on a $150 ceiling: exactly 7 win (140),
	// 3 refuse. The atomic compare-and-set admits no joint overspend.
	const workers, amount = 10, 20.0
	won := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		go func() {
			won <- (m.ReserveSpend("default", amount) == nil)
		}()
	}
	succeeded := 0
	for i := 0; i < workers; i++ {
		if <-won {
			succeeded++
		}
	}
	if succeeded != 7 {
		t.Fatalf("exactly 7 of 10 concurrent $20 reservations must win, got %d", succeeded)
	}
	got, _ := m.GetTenant("default")
	if got.SpentUSD != 140 {
		t.Fatalf("spent must be exactly 140, got %v", got.SpentUSD)
	}
}
