package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestSealSecretsMigratesPlaintextProviderKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kubona.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.InitSettingsTable(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO llm_settings (key, value_json) VALUES ('active_settings', ?)`,
		`{"schema_version":1,"providers":{"nvidia":{"type":"nvidia","api_key":"plain-secret"}}}`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	t.Setenv("KUBONA_SECRET_KEY", strings.Repeat("ab", 32))
	if code := runSealSecrets([]string{"--db", dbPath}); code != 0 {
		t.Fatalf("seal-secrets exit %d", code)
	}
	check, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	posture, err := llm.AuditSecretsAtRest(check)
	if err != nil || posture.Plaintext != 0 || posture.Sealed != 1 {
		t.Fatalf("posture after seal: %+v err=%v", posture, err)
	}
}
