package mandates

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	ErrPQCEncapsulationFailed = errors.New("ML-KEM encapsulation failed")
	ErrPQCDecapsulationFailed = errors.New("ML-KEM decapsulation failed")
	ErrDecryptionFailed       = errors.New("hybrid payload AES-GCM decryption failed")
	ErrRecipientMismatch      = errors.New("recipient key fingerprint mismatch")
)

// HybridSpendMandate wraps a signed SpendMandate in an ML-KEM-768 post-quantum envelope
type HybridSpendMandate struct {
	RecipientFingerprint string `json:"recipient_fingerprint"` // SHA-256 hex of recipient encapsulation key
	EncapsulatedKeyHex   string `json:"encapsulated_key_hex"`  // Hex-encoded ML-KEM-768 ciphertext (1088 bytes)
	NonceHex             string `json:"nonce_hex"`             // Hex-encoded AES-GCM 12-byte nonce
	EncryptedPayloadHex  string `json:"encrypted_payload_hex"` // Hex-encoded AES-GCM ciphertext of canonical JSON SpendMandate
}

// RecipientKeypair holds server-side ML-KEM-768 keys
type RecipientKeypair struct {
	DecapsulationKey *mlkem.DecapsulationKey768
	EncapsulationKey *mlkem.EncapsulationKey768
}

// GenerateRecipientKeypair generates a new ML-KEM-768 keypair
func GenerateRecipientKeypair() (*RecipientKeypair, error) {
	dk, err := mlkem.GenerateKey768()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ML-KEM-768 keypair: %w", err)
	}
	return &RecipientKeypair{
		DecapsulationKey: dk,
		EncapsulationKey: dk.EncapsulationKey(),
	}, nil
}

// ParseEncapsulationKey parses a 1184-byte ML-KEM-768 public key
func ParseEncapsulationKey(keyBytes []byte) (*mlkem.EncapsulationKey768, error) {
	return mlkem.NewEncapsulationKey768(keyBytes)
}

// Fingerprint returns the SHA-256 hex fingerprint of the public encapsulation key
func (rk *RecipientKeypair) Fingerprint() string {
	sum := sha256.Sum256(rk.EncapsulationKey.Bytes())
	return hex.EncodeToString(sum[:])
}

// EncapsulateMandate encrypts a SpendMandate using recipient's ML-KEM-768 encapsulation key
func EncapsulateMandate(ek *mlkem.EncapsulationKey768, mandate *SpendMandate) (*HybridSpendMandate, error) {
	mandateBytes, err := json.Marshal(mandate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mandate: %w", err)
	}

	// 1. ML-KEM-768 encapsulation produces 32-byte shared key and 1088-byte ciphertext
	sharedKey, ciphertext := ek.Encapsulate()

	// 2. Encrypt mandate payload using AES-256-GCM with sharedKey
	block, err := aes.NewCipher(sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM cipher: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encryptedPayload := gcm.Seal(nil, nonce, mandateBytes, nil)

	sum := sha256.Sum256(ek.Bytes())
	return &HybridSpendMandate{
		RecipientFingerprint: hex.EncodeToString(sum[:]),
		EncapsulatedKeyHex:   hex.EncodeToString(ciphertext),
		NonceHex:             hex.EncodeToString(nonce),
		EncryptedPayloadHex:  hex.EncodeToString(encryptedPayload),
	}, nil
}

// DecapsulateAndVerify decapsulates the hybrid envelope, decrypts the mandate, and verifies it
func (rk *RecipientKeypair) DecapsulateAndVerify(hybrid *HybridSpendMandate, expectedTenantID, expectedJobHash string, requestedCostUSD float64) (*SpendMandate, error) {
	if hybrid.RecipientFingerprint != rk.Fingerprint() {
		return nil, ErrRecipientMismatch
	}

	ciphertext, err := hex.DecodeString(hybrid.EncapsulatedKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid encapsulated key hex: %w", err)
	}

	// 1. Decapsulate shared key using recipient private key
	sharedKey, err := rk.DecapsulationKey.Decapsulate(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPQCDecapsulationFailed, err)
	}

	// 2. Decrypt payload
	nonce, err := hex.DecodeString(hybrid.NonceHex)
	if err != nil {
		return nil, fmt.Errorf("invalid nonce hex: %w", err)
	}

	encryptedPayload, err := hex.DecodeString(hybrid.EncryptedPayloadHex)
	if err != nil {
		return nil, fmt.Errorf("invalid encrypted payload hex: %w", err)
	}

	block, err := aes.NewCipher(sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM cipher: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, encryptedPayload, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	// 3. Unmarshal mandate
	var mandate SpendMandate
	if err := json.Unmarshal(plaintext, &mandate); err != nil {
		return nil, fmt.Errorf("failed to decode decrypted mandate: %w", err)
	}

	// 4. Verify cryptographic authentic signature and budget limits
	if err := mandate.Verify(expectedTenantID, expectedJobHash, requestedCostUSD); err != nil {
		return nil, fmt.Errorf("mandate verification failed: %w", err)
	}

	return &mandate, nil
}
