package cost

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestPaymentManager_Lifecycle(t *testing.T) {
	mgr := NewPaymentManager("0xEscrowAddress")

	// 1. Generate challenge
	c := mgr.GenerateChallenge("USDC", "0.05")
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
	c := mgr.GenerateChallenge("USDC", "0.10")

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
