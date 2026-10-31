package ledger_test

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestMigrationBackupCreatedForOlderSchema(t *testing.T) {
	path := t.TempDir() + "/ledger.db"
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.AppendEvent("sess-migrate", "annotation", map[string]any{"x": 1}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Simulate a database from before the migration ledger existed.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(`DELETE FROM schema_migrations`); err != nil {
		raw.Close()
		t.Fatalf("clear version: %v", err)
	}
	raw.Close()

	l2, err := ledger.OpenLedgerWithOptions(path, ledger.Options{BackupBeforeMigration: true})
	if err != nil {
		t.Fatalf("open with migration backup: %v", err)
	}
	defer l2.Close()
	backup := l2.PreMigrationBackupPath()
	if backup == "" {
		t.Fatal("older schema opened without a migration backup")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("migration backup missing: %v", err)
	}
	ro, err := ledger.OpenLedgerReadOnly(backup)
	if err != nil {
		t.Fatalf("open migration backup: %v", err)
	}
	faults, err := ro.VerifyChain("")
	ro.Close()
	if err != nil || len(faults) != 0 {
		t.Fatalf("migration backup chain: faults=%d err=%v", len(faults), err)
	}
}

func TestMigrationBackupNotTakenForFreshOrCurrentSchema(t *testing.T) {
	path := t.TempDir() + "/fresh.db"
	l, err := ledger.OpenLedgerWithOptions(path, ledger.Options{BackupBeforeMigration: true})
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	if got := l.PreMigrationBackupPath(); got != "" {
		t.Fatalf("fresh database took a migration backup: %s", got)
	}
	l.Close()

	l2, err := ledger.OpenLedgerWithOptions(path, ledger.Options{BackupBeforeMigration: true})
	if err != nil {
		t.Fatalf("reopen current: %v", err)
	}
	if got := l2.PreMigrationBackupPath(); got != "" {
		t.Fatalf("current schema took a migration backup: %s", got)
	}
	l2.Close()
}

func TestMigrationBackupRefusesFaultedLedger(t *testing.T) {
	path := t.TempDir() + "/ledger.db"
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.AppendEvent("sess-migrate", "annotation", map[string]any{"x": 1}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(`UPDATE ledger_events SET payload = '{"x":2}' WHERE id = (SELECT MIN(id) FROM ledger_events)`); err != nil {
		raw.Close()
		t.Fatalf("tamper: %v", err)
	}
	if _, err := raw.Exec(`DELETE FROM schema_migrations`); err != nil {
		raw.Close()
		t.Fatalf("clear version: %v", err)
	}
	raw.Close()

	_, err = ledger.OpenLedgerWithOptions(path, ledger.Options{BackupBeforeMigration: true})
	if err == nil {
		t.Fatal("faulted ledger opened with a chain-clean migration backup requirement")
	}
	if !strings.Contains(err.Error(), "pre-migration backup required but failed") {
		t.Fatalf("refusal must name the migration backup, got %v", err)
	}
}
