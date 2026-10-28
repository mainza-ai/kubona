package cost

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// PaymentChallenge represents an HTTP 402 challenge issued to an agent
type PaymentChallenge struct {
	ChallengeID string    `json:"challenge_id"`
	Token       string    `json:"token"`     // e.g. "USDC"
	Amount      string    `json:"amount"`    // e.g. "0.05"
	Recipient   string    `json:"recipient"` // Escrow address
	ExpiresAt   time.Time `json:"expires_at"`
	Payload     string    `json:"payload"` // Hashable challenge string
}

// PaymentReceipt represents the signed authorization submitted by a paying agent
type PaymentReceipt struct {
	ChallengeID string `json:"challenge_id"`
	Token       string `json:"token"`
	Amount      string `json:"amount"`
	PayerPubHex string `json:"payer_pubkey"`
	Signature   string `json:"signature"`
	Timestamp   int64  `json:"timestamp"`
}

// PaymentManager manages x402 payment challenges and receipt verification.
// Challenge lifecycle is a three-state machine — challenge → authorized →
// settled — so a real payment authorizes once and settles once:
//
//	challenge open:      issued by GenerateChallenge, awaiting a receipt.
//	authorized:           VerifyPayment checked the Ed25519 receipt against
//	                     the challenge (token+amount+recipient binding).
//	                     Authorization is single-use: a second VerifyPayment
//	                     refuses (replay protection).
//	settled:              SettlePayment finalized the authorized challenge
//	                     into a local-only Settlement record. Settling twice
//	                     refuses (already settled).
//
// The old shape marked authorized inside VerifyPayment under the settled
// flag, so SettlePayment (which re-verified) always 400'd "already settled"
// and no real payment could ever settle. The two flags are separate now.
type PaymentManager struct {
	mu         sync.RWMutex
	challenges map[string]*PaymentChallenge
	authorized map[string]bool
	settled    map[string]bool
	recipient  string
}

// NewPaymentManager initializes a PaymentManager with a default escrow recipient
func NewPaymentManager(recipient string) *PaymentManager {
	if recipient == "" {
		recipient = "0x89205A3A3b2A69De6Dbf7f01ED13B2108B2c43e7" // Kubona QPU Escrow
	}
	return &PaymentManager{
		challenges: make(map[string]*PaymentChallenge),
		authorized: make(map[string]bool),
		settled:    make(map[string]bool),
		recipient:  recipient,
	}
}

// maxOpenChallenges bounds the challenge map (DoS cap); challenges live
// 5 minutes, so legitimate use never approaches it.
const maxOpenChallenges = 10000

// parseAmount parses an exact decimal amount (W0.17): float64 would make
// "0.05" vs "0.050" compare unequal-or-fuzzy. big.Rat compares exact values,
// so equivalent spellings match and anything else refuses.
func parseAmount(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("amount %q is not a decimal number", s)
	}
	if r.Sign() <= 0 {
		return nil, fmt.Errorf("amount %q must be positive", s)
	}
	return r, nil
}

// GenerateChallenge creates a new cryptographically unique 402 challenge with 5-minute TTL.
// Empty tokens and non-positive/unparseable amounts refuse (W0.17): an
// empty-amount challenge used to settle free.
func (m *PaymentManager) GenerateChallenge(token, amount string) (*PaymentChallenge, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("challenge token is required: refusing")
	}
	if _, err := parseAmount(amount); err != nil {
		return nil, fmt.Errorf("challenge amount invalid: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Janitor sweep (W0.17): expired challenges used to pile up until a
	// verify attempt touched them. Sweep on every issue; refuse past the cap
	// rather than growing unbounded.
	now := time.Now().UTC()
	for id, ch := range m.challenges {
		if now.After(ch.ExpiresAt) {
			delete(m.challenges, id)
		}
	}
	for id, done := range m.settled {
		if done {
			if _, ok := m.challenges[id]; !ok {
				delete(m.settled, id)
			}
		}
	}
	for id, done := range m.authorized {
		if done {
			if _, ok := m.challenges[id]; !ok {
				delete(m.authorized, id)
			}
		}
	}
	if len(m.challenges) >= maxOpenChallenges {
		return nil, fmt.Errorf("challenge registry full: refusing (never grow unbounded)")
	}

	rawID := make([]byte, 16)
	_, _ = rand.Read(rawID)
	challengeID := hex.EncodeToString(rawID)

	expiresAt := now.Add(5 * time.Minute)
	payload := fmt.Sprintf("x402:%s:%s:%s:%s:%d", challengeID, token, amount, m.recipient, expiresAt.Unix())

	challenge := &PaymentChallenge{
		ChallengeID: challengeID,
		Token:       token,
		Amount:      amount,
		Recipient:   m.recipient,
		ExpiresAt:   expiresAt,
		Payload:     payload,
	}

	m.challenges[challengeID] = challenge
	return challenge, nil
}

// FormatWWWAuthenticate formats the HTTP 402 challenge header
func (m *PaymentManager) FormatWWWAuthenticate(c *PaymentChallenge) string {
	return fmt.Sprintf(`x402 token="%s", amount="%s", address="%s", challenge="%s", expires="%d"`,
		c.Token, c.Amount, c.Recipient, c.ChallengeID, c.ExpiresAt.Unix())
}

// VerifyPayment validates a signed payment receipt against the open challenge.
// Success moves the challenge open → authorized (single-use: a second verify
// refuses, settled or not). Settlement is a separate step — see SettlePayment.
func (m *PaymentManager) VerifyPayment(receipt *PaymentReceipt) (bool, error) {
	if receipt == nil {
		return false, fmt.Errorf("receipt is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	challenge, exists := m.challenges[receipt.ChallengeID]
	if !exists {
		return false, fmt.Errorf("challenge %s not found or expired", receipt.ChallengeID)
	}

	if time.Now().UTC().After(challenge.ExpiresAt) {
		delete(m.challenges, receipt.ChallengeID)
		return false, fmt.Errorf("challenge %s has expired", receipt.ChallengeID)
	}

	if m.settled[receipt.ChallengeID] {
		return false, fmt.Errorf("challenge %s already settled (replay protection)", receipt.ChallengeID)
	}
	if m.authorized[receipt.ChallengeID] {
		return false, fmt.Errorf("challenge %s already authorized (replay protection)", receipt.ChallengeID)
	}

	if err := checkReceiptAgainstChallenge(receipt, challenge); err != nil {
		return false, err
	}

	m.authorized[receipt.ChallengeID] = true
	return true, nil
}

// checkReceiptAgainstChallenge enforces the exact challenge match plus the
// Ed25519 payer authorization over the challenge payload. It is the single
// crypto core shared by VerifyPayment (authorize) and SettlePayment
// (finalize): one verifier, never two that can rot apart.
func checkReceiptAgainstChallenge(receipt *PaymentReceipt, challenge *PaymentChallenge) error { // W0.17 exact challenge match: the receipt's token and amount must equal
	// the challenged ones. The signature authorizes the CHALLENGE payload —
	// without this comparison a $0.05 authorization settled with receipt
	// fields claiming $100 in another token (settlement records use the
	// receipt's fields). Amounts compare as exact rationals, so "0.05" and
	// "0.050" match while anything else refuses.
	if receipt.Token != challenge.Token {
		return fmt.Errorf("receipt token %q does not match challenged %q: refusing", receipt.Token, challenge.Token)
	}
	wantAmt, err := parseAmount(challenge.Amount)
	if err != nil {
		return fmt.Errorf("stored challenge amount corrupt: %w", err)
	}
	gotAmt, err := parseAmount(receipt.Amount)
	if err != nil {
		return fmt.Errorf("receipt amount invalid: %w", err)
	}
	if wantAmt.Cmp(gotAmt) != 0 {
		return fmt.Errorf("receipt amount %q does not match challenged %q: refusing", receipt.Amount, challenge.Amount)
	}

	// Verify public key format
	pubBytes, err := hex.DecodeString(receipt.PayerPubHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 payer public key: %v", err)
	}

	// Verify signature format
	sigBytes, err := hex.DecodeString(receipt.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid ed25519 signature format: %v", err)
	}

	// Message is hash of challenge payload
	msgHash := sha256.Sum256([]byte(challenge.Payload))
	if !ed25519.Verify(pubBytes, msgHash[:], sigBytes) {
		return fmt.Errorf("cryptographic signature verification failed for x402 payment")
	}
	return nil
}

// ParseAuthorizationHeader extracts a PaymentReceipt from "Authorization: x402 ..." header
func ParseAuthorizationHeader(header string) (*PaymentReceipt, error) {
	if !strings.HasPrefix(header, "x402 ") {
		return nil, fmt.Errorf("invalid authorization scheme, expected 'x402'")
	}

	parts := strings.Split(strings.TrimPrefix(header, "x402 "), ",")
	params := make(map[string]string)
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			params[k] = v
		}
	}

	receipt := &PaymentReceipt{
		ChallengeID: params["challenge"],
		Token:       params["token"],
		Amount:      params["amount"],
		PayerPubHex: params["payer"],
		Signature:   params["signature"],
		Timestamp:   time.Now().Unix(),
	}

	if receipt.ChallengeID == "" || receipt.Token == "" || receipt.Amount == "" ||
		receipt.PayerPubHex == "" || receipt.Signature == "" {
		return nil, fmt.Errorf("missing required fields in x402 authorization header (challenge, token, amount, payer, signature)")
	}

	return receipt, nil
}

// Settlement records the outcome of finalizing a verified payment. Until a
// live chain broadcaster is injected (Phase 5 x402 rail), settlements are
// local-only: Simulated is always true and Reference is a content hash, NOT a
// chain transaction hash. The flag exists so no consumer can mistake a local
// receipt for an on-chain settlement.
//
// Authorization vs settlement (W0.17): dispatch gates (handleA2A) rest on
// VerifyPayment — a REAL Ed25519 payer authorization over the challenge
// payload binding token+amount+recipient — not on chain broadcast. A verified
// receipt is genuine payer consent; only the broadcast is stubbed, and every
// settlement record carries simulated:true to say so. Check Simulated before
// treating any settlement as on-chain proof.
type Settlement struct {
	Reference string `json:"reference"`
	Network   string `json:"network"`
	Simulated bool   `json:"simulated"`
	Settled   bool   `json:"settled"`
}

// SettlePayment finalizes payment against an escrow ledger or payment network.
// It moves the challenge authorized → settled (or open → settled when the
// payer settles without a prior VerifyPayment call — the crypto verifies
// either way). A real payment settles exactly once: settling an already
// settled challenge refuses.
func (m *PaymentManager) SettlePayment(receipt *PaymentReceipt, network string) (*Settlement, error) {
	if receipt == nil {
		return nil, fmt.Errorf("payment verification failed: receipt is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	challenge, exists := m.challenges[receipt.ChallengeID]
	if !exists {
		return nil, fmt.Errorf("payment verification failed: challenge %s not found or expired", receipt.ChallengeID)
	}
	if time.Now().UTC().After(challenge.ExpiresAt) {
		delete(m.challenges, receipt.ChallengeID)
		return nil, fmt.Errorf("payment verification failed: challenge %s has expired", receipt.ChallengeID)
	}
	if m.settled[receipt.ChallengeID] {
		return nil, fmt.Errorf("payment verification failed: challenge %s already settled (settlement is single-use)", receipt.ChallengeID)
	}
	if err := checkReceiptAgainstChallenge(receipt, challenge); err != nil {
		return nil, fmt.Errorf("payment verification failed: %w", err)
	}

	if network == "" {
		network = "base-sepolia"
	}

	txData := fmt.Sprintf("%s:%s:%s:%s:%s:%d", network, receipt.ChallengeID, receipt.Token, receipt.Amount, receipt.PayerPubHex, receipt.Timestamp)
	txHash := sha256.Sum256([]byte(txData))
	ref := "local:" + hex.EncodeToString(txHash[:])

	m.authorized[receipt.ChallengeID] = true
	m.settled[receipt.ChallengeID] = true

	return &Settlement{Reference: ref, Network: network, Simulated: true, Settled: true}, nil
}
