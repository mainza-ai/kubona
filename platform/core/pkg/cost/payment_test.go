package cost

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPaymentManager_Lifecycle(t *testing.T) {
	mgr := NewPaymentManager("0xEscrowAddress")

	// 1. Generate challenge
	c, err := mgr.GenerateChallenge("USDC", "0.05")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if c.ChallengeID == "" {
		t.Fatalf("expected non-empty challenge ID")
	}
	if c.Token != "USDC" || c.Amount != "0.05" {
		t.Fatalf("unexpected challenge values: %+v", c)
	}

	header := mgr.FormatWWWAuthenticate(c)
	if header == "" {
		t.Fatalf("expected non-empty WWW-Authenticate header")
	}

	// 2. Client signs the challenge payload with Ed25519
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 keys: %v", err)
	}

	msgHash := sha256.Sum256([]byte(c.Payload))
	sig := ed25519.Sign(priv, msgHash[:])

	receipt := &PaymentReceipt{
		ChallengeID: c.ChallengeID,
		Token:       "USDC",
		Amount:      "0.05",
		PayerPubHex: hex.EncodeToString(pub),
		Signature:   hex.EncodeToString(sig),
		Timestamp:   time.Now().Unix(),
	}

	// 3. Verify valid payment
	valid, err := mgr.VerifyPayment(receipt)
	if err != nil || !valid {
		t.Fatalf("expected valid payment verification, got valid=%v, err=%v", valid, err)
	}

	// 4. Test replay protection (should fail on second attempt)
	replayed, err := mgr.VerifyPayment(receipt)
	if err == nil || replayed {
		t.Fatalf("expected replay rejection, got valid=%v, err=%v", replayed, err)
	}

	// 5. Test header parsing
	authHeader := fmt.Sprintf(`x402 challenge="%s", token="USDC", amount="0.05", payer="%s", signature="%s"`,
		c.ChallengeID, hex.EncodeToString(pub), hex.EncodeToString(sig))
	parsedReceipt, err := ParseAuthorizationHeader(authHeader)
	if err != nil {
		t.Fatalf("failed to parse auth header: %v", err)
	}
	if parsedReceipt.ChallengeID != c.ChallengeID || parsedReceipt.PayerPubHex != hex.EncodeToString(pub) {
		t.Fatalf("parsed receipt mismatch: %+v", parsedReceipt)
	}
}

func TestPaymentManager_TamperedSignature(t *testing.T) {
	mgr := NewPaymentManager("0xEscrowAddress")
	c, err := mgr.GenerateChallenge("USDC", "0.10")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}

	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fakeSig := make([]byte, ed25519.SignatureSize)
	_, _ = rand.Read(fakeSig)

	receipt := &PaymentReceipt{
		ChallengeID: c.ChallengeID,
		Token:       "USDC",
		Amount:      "0.10",
		PayerPubHex: hex.EncodeToString(pub),
		Signature:   hex.EncodeToString(fakeSig),
	}

	valid, err := mgr.VerifyPayment(receipt)
	if valid || err == nil {
		t.Fatalf("expected verification failure for tampered signature, got valid=%v", valid)
	}
}

func TestPaymentManager_ChallengeValidation(t *testing.T) {
	mgr := NewPaymentManager("0xEscrowAddress")
	for _, tc := range [][2]string{{"", "0.05"}, {"USDC", ""}, {"USDC", "0"}, {"USDC", "-1"}, {"USDC", "much"}} {
		if _, err := mgr.GenerateChallenge(tc[0], tc[1]); err == nil {
			t.Fatalf("GenerateChallenge(%q, %q) must refuse", tc[0], tc[1])
		}
	}
}

func TestPaymentManager_ReceiptMustMatchChallenge(t *testing.T) {
	mgr := NewPaymentManager("0xEscrowAddress")
	c, err := mgr.GenerateChallenge("USDC", "0.05")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	msgHash := sha256.Sum256([]byte(c.Payload))
	sig := ed25519.Sign(priv, msgHash[:])
	mk := func(token, amount string) *PaymentReceipt {
		return &PaymentReceipt{
			ChallengeID: c.ChallengeID,
			Token:       token,
			Amount:      amount,
			PayerPubHex: hex.EncodeToString(pub),
			Signature:   hex.EncodeToString(sig),
			Timestamp:   time.Now().Unix(),
		}
	}
	// Wrong token refuses even with a valid signature over the challenge.
	if ok, err := mgr.VerifyPayment(mk("FAKE", "0.05")); ok || err == nil {
		t.Fatalf("token-mismatched receipt must refuse, got %v %v", ok, err)
	}
	// Wrong amount refuses (pay $0.05, settle as $100 hole closed).
	if ok, err := mgr.VerifyPayment(mk("USDC", "100")); ok || err == nil {
		t.Fatalf("amount-mismatched receipt must refuse, got %v %v", ok, err)
	}
	if ok, err := mgr.VerifyPayment(mk("USDC", "0.01")); ok || err == nil {
		t.Fatalf("under-amount receipt must refuse, got %v %v", ok, err)
	}
	// Equivalent spelling matches exactly ("0.050" == "0.05", rational compare).
	if ok, err := mgr.VerifyPayment(mk("USDC", "0.050")); !ok || err != nil {
		t.Fatalf("equivalent-spelling amount must verify, got %v %v", ok, err)
	}
}

func TestParseAuthorizationHeaderRequiresAllFields(t *testing.T) {
	base := `x402 challenge="c", token="USDC", amount="0.05", payer="p", signature="s"`
	if _, err := ParseAuthorizationHeader(base); err != nil {
		t.Fatalf("full header must parse: %v", err)
	}
	for _, drop := range []string{"token", "amount"} {
		fields := map[string]string{"challenge": "c", "token": "USDC", "amount": "0.05", "payer": "p", "signature": "s"}
		delete(fields, drop)
		var b strings.Builder
		b.WriteString("x402 ")
		i := 0
		for k, v := range fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k + `="` + v + `"`)
			i++
		}
		if _, err := ParseAuthorizationHeader(b.String()); err == nil {
			t.Fatalf("header without %s must refuse", drop)
		}
	}
}

// X402 reboot-safety proof (residual investigation 2026-10-17): the
// settled map is in-memory, but single-use does NOT depend on it across
// restarts — verification requires a LIVE challenge, and challenges die
// with the process. A rebooted manager holds no challenges, so every
// pre-restart receipt refuses at lookup (fail-closed), settled or not.
// This test pins that property: if verification ever stops requiring
// challenge liveness, it fails and the ledger-backing residual reopens.
func TestPaymentManager_RebootRefusesOldReceipts(t *testing.T) {
	mgr := NewPaymentManager("0xEscrowAddress")
	c, err := mgr.GenerateChallenge("USDC", "0.05")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	msgHash := sha256.Sum256([]byte(c.Payload))
	sig := ed25519.Sign(priv, msgHash[:])
	receipt := &PaymentReceipt{
		ChallengeID: c.ChallengeID,
		Token:       "USDC",
		Amount:      "0.05",
		PayerPubHex: hex.EncodeToString(pub),
		Signature:   hex.EncodeToString(sig),
		Timestamp:   time.Now().Unix(),
	}
	if _, err := mgr.SettlePayment(receipt, "base-sepolia"); err != nil {
		t.Fatalf("settle: %v", err)
	}

	// Simulate restart: brand-new manager, zero carried state.
	restarted := NewPaymentManager("0xEscrowAddress")
	if valid, err := restarted.VerifyPayment(receipt); valid || err == nil {
		t.Fatalf("post-restart verify must refuse, got valid=%v err=%v", valid, err)
	}
	if _, err := restarted.SettlePayment(receipt, "base-sepolia"); err == nil {
		t.Fatal("post-restart settle must refuse")
	}
	// And the pre-restart manager still refuses the replay in-window.
	if valid, err := mgr.VerifyPayment(receipt); valid || err == nil {
		t.Fatalf("in-window replay must refuse, got valid=%v err=%v", valid, err)
	}
}
