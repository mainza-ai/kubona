package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad fk-quarantine --db /abs/ledger.db [--apply]
//
// Dry-run by default. With --apply, copies each orphan row into _fk_orphans
// (full row JSON) and deletes it from the live table in one transaction.
// Jobs/loops keep their spend/mandate fields in the JSON; nothing is lost.
// FK enforcement is NOT enabled here — rerun fk-audit after the repair.
func runFKQuarantine(args []string) int {
	fs := flag.NewFlagSet("fk-quarantine", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to SQLite WAL ledger (must be absolute)")
	apply := fs.Bool("apply", false, "write the quarantine (default: dry run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path (relative ledger paths refuse: CWD-dependent repair proves nothing)")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: ledger not readable:", err)
		return 1
	}

	l, err := ledger.OpenLedger(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open for quarantine (is the daemon running?):", err)
		return 1
	}
	defer l.Close()

	orphans, err := foreignKeyOrphans(l.DB())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(orphans) == 0 {
		fmt.Println("fk-quarantine: clean (no orphan rows)")
		return 0
	}
	if !*apply {
		fmt.Printf("fk-quarantine: %d orphan row(s) — dry run, no rows changed\n", len(orphans))
		fmt.Println("  rerun with --apply to copy them to _fk_orphans and remove them from live tables")
		return 1
	}

	if err := quarantineOrphans(l.DB(), orphans); err != nil {
		fmt.Fprintln(os.Stderr, "error: quarantine failed (transaction rolled back):", err)
		return 1
	}
	remaining, err := foreignKeyOrphans(l.DB())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: post-repair audit failed:", err)
		return 1
	}
	if len(remaining) != 0 {
		fmt.Printf("fk-quarantine: %d orphan row(s) remain; investigate before enabling FK enforcement\n", len(remaining))
		return 1
	}
	fmt.Printf("fk-quarantine: quarantined %d orphan row(s); foreign_key_check is clean\n", len(orphans))
	return 0
}

const quarantineDDL = `CREATE TABLE IF NOT EXISTS _fk_orphans (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	table_name TEXT NOT NULL,
	row_id INTEGER NOT NULL,
	parent_table TEXT NOT NULL,
	fk_id INTEGER NOT NULL,
	row_json TEXT NOT NULL,
	detected_at TEXT NOT NULL,
	reason TEXT NOT NULL,
	UNIQUE (table_name, row_id)
)`

var quarantineTables = map[string]bool{
	"chat_messages":     true,
	"jobs":              true,
	"loops":             true,
	"campaign_states":   true,
	"campaign_versions": true,
	"claim_records":     true,
}

func quarantineOrphans(db *sql.DB, orphans []fkOrphan) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(quarantineDDL); err != nil {
		return fmt.Errorf("create _fk_orphans: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, o := range orphans {
		if !quarantineTables[o.Table] {
			return fmt.Errorf("unknown orphan table %q: refusing automatic quarantine", o.Table)
		}
		rowJSON, err := selectRowJSON(tx, o.Table, o.RowID)
		if err != nil {
			return fmt.Errorf("read %s rowid %d: %w", o.Table, o.RowID, err)
		}
		_, recommendation := classifyFKOrphan(o.Table)
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO _fk_orphans (table_name, row_id, parent_table, fk_id, row_json, detected_at, reason) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			o.Table, o.RowID, o.Parent, o.FKID, rowJSON, now, recommendation,
		); err != nil {
			return fmt.Errorf("quarantine %s rowid %d: %w", o.Table, o.RowID, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", o.Table), o.RowID); err != nil {
			return fmt.Errorf("remove %s rowid %d: %w", o.Table, o.RowID, err)
		}
	}
	return tx.Commit()
}

func selectRowJSON(tx *sql.Tx, table string, rowID int64) (string, error) {
	if !quarantineTables[table] {
		return "", fmt.Errorf("unknown table %q", table)
	}
	rows, err := tx.Query(fmt.Sprintf("SELECT * FROM %s WHERE rowid = ?", table), rowID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", fmt.Errorf("row not found")
	}
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return "", err
	}
	obj := make(map[string]any, len(cols))
	for i, col := range cols {
		switch v := vals[i].(type) {
		case []byte:
			obj[col] = string(v)
		default:
			obj[col] = v
		}
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
