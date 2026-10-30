package llm

import (
	"strings"
	"testing"
)

func TestSealOpenRoundTripAndRefusals(t *testing.T) {
	key := strings.Repeat("ab", 32)
	t.Setenv("KUBONA_SECRET_KEY", key)
	sealed, err := sealSecret("super-secret")
	if err != nil || !strings.HasPrefix(sealed, sealedPrefix) || strings.Contains(sealed, "super-secret") {
		t.Fatalf("seal failed: %q %v", sealed, err)
	}
	got, err := openSecret(sealed)
	if err != nil || got != "super-secret" {
		t.Fatalf("open failed: %q %v", got, err)
	}
	t.Setenv("KUBONA_SECRET_KEY", strings.Repeat("cd", 32))
	if _, err := openSecret(sealed); err == nil {
		t.Fatal("wrong key must fail authentication")
	}
	t.Setenv("KUBONA_SECRET_KEY", "")
	if _, err := openSecret(sealed); err == nil {
		t.Fatal("sealed secret with no key must refuse")
	}
	if got, err := openSecret("legacy-plain"); err != nil || got != "legacy-plain" {
		t.Fatalf("legacy plaintext must remain readable: %q %v", got, err)
	}
}
