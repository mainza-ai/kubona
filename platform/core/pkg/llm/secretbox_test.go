package llm

import (
	"database/sql"
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

func TestAuditSecretsAtRestTreatsMissingTableAsNoStoredKeys(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := AuditSecretsAtRest(db)
	if err != nil || got.Plaintext != 0 || got.Sealed != 0 {
		t.Fatalf("missing settings table must audit clean: %+v err=%v", got, err)
	}
}

func TestAuditSecretsAtRestCountsStoredKeys(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := InitSettingsTable(db); err != nil {
		t.Fatal(err)
	}

	plain := `{"schema_version":1,"providers":{"nvidia":{"type":"nvidia","api_key":"plain-secret"}}}`
	if _, err := db.Exec(`INSERT INTO llm_settings (key, value_json) VALUES ('active_settings', ?)`, plain); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBONA_SECRET_KEY", "")
	got, err := AuditSecretsAtRest(db)
	if err != nil || got.KeyConfigured || got.Plaintext != 1 || got.Sealed != 0 {
		t.Fatalf("plaintext posture: %+v err=%v", got, err)
	}

	t.Setenv("KUBONA_SECRET_KEY", strings.Repeat("ab", 32))
	sealed, err := sealSecret("sealed-secret")
	if err != nil {
		t.Fatal(err)
	}
	blob := `{"schema_version":1,"providers":{"nvidia":{"type":"nvidia","api_key":"` + sealed + `"}}}`
	if _, err := db.Exec(`UPDATE llm_settings SET value_json = ? WHERE key = 'active_settings'`, blob); err != nil {
		t.Fatal(err)
	}
	got, err = AuditSecretsAtRest(db)
	if err != nil || !got.KeyConfigured || got.Plaintext != 0 || got.Sealed != 1 {
		t.Fatalf("sealed posture: %+v err=%v", got, err)
	}
}

func TestProductionRefusesPlaintextSecretWrite(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_SECRET_KEY", "")
	if _, err := sealSecret("must-not-land-plaintext"); err == nil {
		t.Fatal("production without KUBONA_SECRET_KEY must refuse a non-empty secret write")
	}
	if got, err := sealSecret(""); err != nil || got != "" {
		t.Fatalf("empty secret must remain a no-op: %q %v", got, err)
	}
}
