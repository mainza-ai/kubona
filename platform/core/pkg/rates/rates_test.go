package rates

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmbeddedCardMatchesCanonical(t *testing.T) {
	embeddedBytes, err := os.ReadFile("rate-cards.json")
	if err != nil {
		t.Fatalf("read embedded copy: %v", err)
	}
	canonicalPath := filepath.Join("..", "..", "..", "contracts", "rate-cards.json")
	canonicalBytes, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical card: %v", err)
	}
	if string(embeddedBytes) != string(canonicalBytes) {
		t.Fatalf("embedded rate card drifted from %s; copy the canonical file, never hand-edit the duplicate", canonicalPath)
	}
}

func TestValidateCurrentCard(t *testing.T) {
	if err := Validate(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("current card must validate: %v", err)
	}
}

func TestValidateRefusesStaleCard(t *testing.T) {
	err := Validate(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("stale card must refuse")
	}
	if !errors.Is(err, ErrStale) {
		t.Fatalf("stale refusal must wrap ErrStale, got %v", err)
	}
}

func TestLensAndModelLookup(t *testing.T) {
	if got := MustLensCostUSD("atlas-lens"); got != 0.5 {
		t.Fatalf("atlas lens cost = %v, want 0.5", got)
	}
	r, ok := LookupModelRate("deepseek-v4.1-flash")
	if !ok {
		t.Fatal("deepseek-v4.1-flash missing from card")
	}
	if r.InputPer1KUSD != 0.0001 || r.OutputPer1KUSD != 0.0004 {
		t.Fatalf("deepseek rate = %+v", r)
	}
	if _, ok := LookupModelRate("mystery-model-2099"); ok {
		t.Fatal("unknown model must not be priced")
	}
}

func TestIsStale(t *testing.T) {
	if IsStale(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("current card reported stale")
	}
	if !IsStale(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("expired card not reported stale")
	}
}
