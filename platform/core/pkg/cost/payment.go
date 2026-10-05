package cost

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PaymentChallenge represents an HTTP 402 challenge issued to an agent
type PaymentChallenge struct {
	ChallengeID string    `json:"challenge_id"`
	Token       string    `json:"token"`       // e.g. "USDC"
	Amount      string    `json:"amount"`      // e.g. "0.05"
	Recipient   string    `json:"recipient"`   // Escrow address
	ExpiresAt   time.Time `json:"expires_at"`
	Payload     string    `json:"payload"`     // Hashable challenge string
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

// PaymentManager manages x402 payment challenges and receipt verification
type PaymentManager struct {
	mu         sync.RWMutex
	challenges map[string]*PaymentChallenge
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
		settled:    make(map[string]bool),
		recipient:  recipient,
	}
}

// GenerateChallenge creates a new cryptographically unique 402 challenge with 5-minute TTL
func (m *PaymentManager) GenerateChallenge(token, amount string) *PaymentChallenge {
	m.mu.Lock()
	defer m.mu.Unlock()

	rawID := make([]byte, 16)
	_, _ = rand.Read(rawID)
	challengeID := hex.EncodeToString(rawID)

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
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
	return challenge
}

// FormatWWWAuthenticate formats the HTTP 402 challenge header
func (m *PaymentManager) FormatWWWAuthenticate(c *PaymentChallenge) string {
	return fmt.Sprintf(`x402 token="%s", amount="%s", address="%s", challenge="%s", expires="%d"`,
		c.Token, c.Amount, c.Recipient, c.ChallengeID, c.ExpiresAt.Unix())
}

// VerifyPayment validates a signed payment receipt against the open challenge
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

	// Verify public key format
	pubBytes, err := hex.DecodeString(receipt.PayerPubHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid ed25519 payer public key: %v", err)
	}

	// Verify signature format
	sigBytes, err := hex.DecodeString(receipt.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid ed25519 signature format: %v", err)
	}

	// Message is hash of challenge payload
	msgHash := sha256.Sum256([]byte(challenge.Payload))
	if !ed25519.Verify(pubBytes, msgHash[:], sigBytes) {
		return false, fmt.Errorf("cryptographic signature verification failed for x402 payment")
	}

	m.settled[receipt.ChallengeID] = true
	return true, nil
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

	if receipt.ChallengeID == "" || receipt.PayerPubHex == "" || receipt.Signature == "" {
		return nil, fmt.Errorf("missing required fields in x402 authorization header")
	}

	return receipt, nil
}
