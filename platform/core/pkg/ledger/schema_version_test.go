package ledger_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestSchemaVersionRecordedAndDowngradeRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	version, err := l.SchemaVersion()
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != ledger.CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, ledger.CurrentSchemaVersion)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}

	// Simulate a database written by a newer binary. The old binary must
	// refuse before it mutates the schema.
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, ledger.CurrentSchemaVersion+1, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert future version: %v", err)
	}
	db.Close()

	if _, err := ledger.OpenLedger(path); err == nil {
		t.Fatal("newer schema must refuse to open")
	} else if !strings.Contains(err.Error(), "newer than this binary supports") {
		t.Fatalf("refusal must name downgrade protection, got %v", err)
	}
}

func TestLegacyRealColumnsBackfilledAndDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, spend_limit_usd REAL, current_spend_usd REAL)`); err != nil {
		t.Fatalf("create legacy sessions: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (id, spend_limit_usd, current_spend_usd) VALUES ('legacy', 12.5, 3.25)`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	raw.Close()

	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open legacy ledger: %v", err)
	}
	var limit, spent int64
	if err := l.DB().QueryRow(`SELECT spend_limit_micro_usd, current_spend_micro_usd FROM sessions WHERE id = 'legacy'`).Scan(&limit, &spent); err != nil {
		t.Fatalf("read backfilled micro: %v", err)
	}
	if limit != 12500000 || spent != 3250000 {
		t.Fatalf("backfill = %d/%d micro, want 12500000/3250000", limit, spent)
	}
	for _, col := range []string{"spend_limit_usd", "current_spend_usd"} {
		ok, err := legacyColumnExists(l.DB(), "sessions", col)
		if err != nil {
			t.Fatalf("column check %s: %v", col, err)
		}
		if ok {
			t.Fatalf("legacy column %s still present after migration", col)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close legacy ledger: %v", err)
	}

	// Reopen: the same compatibility migrations must be idempotent now that
	// the legacy columns are gone.
	l2, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen migrated ledger: %v", err)
	}
	l2.Close()
}

func legacyColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}
