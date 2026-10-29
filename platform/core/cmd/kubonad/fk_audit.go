package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad fk-audit --db /abs/ledger.db [--json]
//
// Read-only audit of declared foreign keys. It reports rows whose parent
// is missing; it does not enable enforcement and does not modify the
// database. Enabling PRAGMA foreign_keys without this audit first would
// fail inserts on any pre-existing orphan.
func runFKAudit(args []string) int {
	fs := flag.NewFlagSet("fk-audit", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to SQLite WAL ledger (must be absolute)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path (relative ledger paths refuse: CWD-dependent audits prove nothing)")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: ledger not readable:", err)
		return 1
	}

	l, err := ledger.OpenLedgerReadOnly(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer l.Close()

	orphans, err := foreignKeyOrphans(l.DB())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *asJSON {
		out, _ := json.MarshalIndent(map[string]any{
			"ok":      len(orphans) == 0,
			"orphans": orphans,
		}, "", "  ")
		fmt.Println(string(out))
	} else if len(orphans) == 0 {
		fmt.Println("fk-audit: clean (no orphan rows)")
	} else {
		fmt.Printf("fk-audit: %d orphan row(s)\n", len(orphans))
		for _, o := range orphans {
			fmt.Printf("  table=%s rowid=%d parent=%s fkid=%d\n", o.Table, o.RowID, o.Parent, o.FKID)
		}
	}
	if len(orphans) != 0 {
		return 1
	}
	return 0
}

type fkOrphan struct {
	Table  string `json:"table"`
	RowID  int64  `json:"rowid"`
	Parent string `json:"parent"`
	FKID   int64  `json:"fkid"`
}

func foreignKeyOrphans(db *sql.DB) ([]fkOrphan, error) {
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return nil, fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	var out []fkOrphan
	for rows.Next() {
		var o fkOrphan
		if err := rows.Scan(&o.Table, &o.RowID, &o.Parent, &o.FKID); err != nil {
			return nil, fmt.Errorf("foreign_key_check scan: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("foreign_key_check rows: %w", err)
	}
	return out, nil
}
