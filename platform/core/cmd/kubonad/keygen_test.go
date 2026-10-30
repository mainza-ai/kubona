package main

import (
	"crypto/ed25519"
	"testing"
)

func TestGenerateKeySeedIsUsable(t *testing.T) {
	seed, err := generateKeySeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) != ed25519.SeedSize {
		t.Fatalf("seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(priv))
	}
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		t.Fatal("public key derivation failed")
	}
}

func TestRunKeygenRejectsUnknownKind(t *testing.T) {
	if code := runKeygen([]string{"--kind", "nope"}); code != 2 {
		t.Fatalf("unknown kind must be a usage error, got %d", code)
	}
}
