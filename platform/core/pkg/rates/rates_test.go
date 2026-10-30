package rates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	if got := MustLensCostUSD("atlas-lens"); got != 1.2 {
		t.Fatalf("atlas lens cost = %v, want 1.2", got)
	}
	r, ok := LookupModelRate("opencode", "deepseek-v4.1-flash")
	if !ok {
		t.Fatal("deepseek-v4.1-flash missing from card")
	}
	if r.InputPer1KUSD != 0.0003 || r.OutputPer1KUSD != 0.0012 {
		t.Fatalf("deepseek rate = %+v", r)
	}
	if _, ok := LookupModelRate("opencode", "mystery-model-2099"); ok {
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

func restoreEmbedded(t *testing.T) {
	t.Helper()
	if err := Load(embedded, "embedded"); err != nil {
		t.Fatalf("restore embedded: %v", err)
	}
}

func TestLoadFileSourceAndStaleRefusalKeepsLastGood(t *testing.T) {
	restoreEmbedded(t)
	t.Cleanup(func() { restoreEmbedded(t) })
	good := []byte(`{"version":9,"source":"test","effective_date":"2026-10-30","stale_after_days":30,"models":{"p/m":{"input_per_1k_usd":0.001,"output_per_1k_usd":0.002}}}`)
	path := filepath.Join(t.TempDir(), "rates.json")
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path); err != nil {
		t.Fatalf("load file: %v", err)
	}
	if Source() != "file:"+path {
		t.Fatalf("source = %q", Source())
	}
	if r, ok := LookupModelRate("p", "m"); !ok || r.InputPer1KUSD != 0.001 {
		t.Fatalf("lookup = %+v %v", r, ok)
	}
	stale := []byte(`{"version":10,"source":"test","effective_date":"2020-01-01","stale_after_days":1,"models":{}}`)
	if err := os.WriteFile(path, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path); !errors.Is(err, ErrStale) {
		t.Fatalf("stale file must refuse, got %v", err)
	}
	if Source() != "file:"+path {
		t.Fatalf("failed refresh clobbered the last good card: %q", Source())
	}
}

func TestLoadURLFromLoopback(t *testing.T) {
	restoreEmbedded(t)
	t.Cleanup(func() { restoreEmbedded(t) })
	body := `{"version":11,"source":"http","effective_date":"2026-10-30","stale_after_days":30,"models":{}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	if err := LoadURL(context.Background(), srv.URL); err != nil {
		t.Fatalf("load URL: %v", err)
	}
	if Source() != "url:"+srv.URL {
		t.Fatalf("source = %q", Source())
	}
}

func TestProductionLensRefusesEmbeddedBootstrap(t *testing.T) {
	restoreEmbedded(t)
	t.Cleanup(func() { restoreEmbedded(t) })
	t.Setenv("KUBONA_ENV", "production")
	if _, ok := LensCostUSD("atlas-lens"); ok {
		t.Fatal("production must refuse the embedded lens bootstrap")
	}
}
