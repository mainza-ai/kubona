package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// kubonad seal-secrets --db /abs/kubona.db
//
// Re-saves provider credentials already stored in llm_settings through the
// AES-256-GCM secretbox. It is the migration for a plaintext ledger: set
// KUBONA_SECRET_KEY, run this while the daemon is stopped, then preflight
// reports the stored keys as sealed. It never prints key material.
func runSealSecrets(args []string) int {
	fs := flag.NewFlagSet("seal-secrets", flag.ContinueOnError)
	dbPath := fs.String("db", "", "ledger path (absolute; default KUBONA_DB_PATH or ~/.kubona/kubona.db)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" {
		*dbPath = os.Getenv("KUBONA_DB_PATH")
	}
	if *dbPath == "" {
		home, _ := os.UserHomeDir()
		*dbPath = filepath.Join(home, ".kubona", "kubona.db")
	}
	if !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw&_busy_timeout=5000", *dbPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger:", err)
		return 1
	}
	defer db.Close()
	posture, err := llm.AuditSecretsAtRest(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: inspect secrets:", err)
		return 1
	}
	if !posture.KeyConfigured {
		fmt.Fprintln(os.Stderr, "error: KUBONA_SECRET_KEY must be a valid 32-byte hex/base64 key before sealing")
		return 1
	}
	settings, err := llm.LoadSettings(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: load settings:", err)
		return 1
	}
	if err := settings.Save(db); err != nil {
		fmt.Fprintln(os.Stderr, "error: save sealed settings:", err)
		return 1
	}
	after, err := llm.AuditSecretsAtRest(db)
	if err != nil || after.Plaintext != 0 {
		fmt.Fprintf(os.Stderr, "error: sealing did not complete (plaintext=%d): %v\n", after.Plaintext, err)
		return 1
	}
	fmt.Printf("seal-secrets: sealed=%d plaintext=0\n", after.Sealed)
	return 0
}
