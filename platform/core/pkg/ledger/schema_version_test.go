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
