package tenants

// Audited refund primitive tests: consumed spend returns to the books
// only admin-signed, reasoned, and sealed; over-refund/empty/cross-lab
// attempts refuse with their own audit rows.

import (
	"crypto/ed25519"
	"testing"
	"time"
)

func setupRefundLab(t *testing.T) (*Manager, func(), ed25519.PrivateKey, string) {
	t.Helper()
	m, cleanup := setupManager(t)
	if _, err := m.CreateTenant("lab-refund", "Refund Lab"); err != nil {
		t.Fatalf("create: %v", err)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminHex := pubHex(adminPub)
	if _, err := m.SetAdminKey("lab-refund", adminHex, nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// Burn $40 of consumed spend (reservations add to spent_usd).
	if err := m.ReserveSpend("lab-refund", 40); err != nil {
		t.Fatalf("spend: %v", err)
	}
	return m, cleanup, adminPriv, adminHex
}

func refundEnvelope(t *testing.T, priv ed25519.PrivateKey, pubHexStr, tenant string, amount float64, reason string) *AdminAction {
	t.Helper()
	return signed(t, priv, pubHexStr, tenant, "refund_spend",
		map[string]any{"tenant_id": tenant, "amount_usd": amount, "reason": reason}, time.Hour)
}

func TestRefundSpendHappyPath(t *testing.T) {
	m, cleanup, priv, pubHexStr := setupRefundLab(t)
	defer cleanup()
	act := refundEnvelope(t, priv, pubHexStr, "lab-refund", 15.0, "failed QPU job honoured hardware credit")
	got, err := m.RefundSpend(act, "lab-refund", 15.0, "failed QPU job honoured hardware credit")
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got.SpentUSD != 25.0 {
		t.Fatalf("spent must drop 40 -> 25, got %v", got.SpentUSD)
	}
	events, err := m.ledger.GetRecentEvents("lab-refund", 50)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "tenant_spend_refunded" {
			found = true
		}
	}
	if !found {
		t.Fatal("refund must seal tenant_spend_refunded")
	}
}

func TestRefundSpendRefusals(t *testing.T) {
	m, cleanup, priv, pubHexStr := setupRefundLab(t)
	defer cleanup()

	// Over-refund refuses with its own audit row (no negative books).
	act := refundEnvelope(t, priv, pubHexStr, "lab-refund", 1000.0, "too much")
	if _, err := m.RefundSpend(act, "lab-refund", 1000.0, "too much"); err == nil {
		t.Fatal("over-refund must refuse")
	}
	// Zero + empty reason refuse.
	act = refundEnvelope(t, priv, pubHexStr, "lab-refund", 0, "x")
	if _, err := m.RefundSpend(act, "lab-refund", 0, "x"); err == nil {
		t.Fatal("zero refund must refuse")
	}
	act = refundEnvelope(t, priv, pubHexStr, "lab-refund", 5.0, "")
	if _, err := m.RefundSpend(act, "lab-refund", 5.0, ""); err == nil {
		t.Fatal("reasonless refund must refuse")
	}
	// Cross-tenant envelope refuses.
	act = refundEnvelope(t, priv, pubHexStr, "lab-refund", 5.0, "x")
	if _, err := m.RefundSpend(act, "default", 5.0, "x"); err == nil {
		t.Fatal("cross-tenant refund must refuse")
	}
	// Wrong key refuses.
	_, evilPriv, _ := ed25519.GenerateKey(nil)
	evil := signed(t, evilPriv, pubHexStr, "lab-refund", "refund_spend",
		map[string]any{"tenant_id": "lab-refund", "amount_usd": 5.0, "reason": "x"}, time.Hour)
	if _, err := m.RefundSpend(evil, "lab-refund", 5.0, "x"); err == nil {
		t.Fatal("foreign-signed refund must refuse")
	}
	// Balance untouched by all refusals.
	got, _ := m.GetTenant("lab-refund")
	if got.SpentUSD != 40.0 {
		t.Fatalf("refusals must not move books, spent=%v", got.SpentUSD)
	}
}
