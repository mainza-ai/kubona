package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// kubonad restore-drill --db /abs/kubona.db [--scratch /abs/tmpdir]
//
// Takes a fresh verified backup, restores it into an empty scratch path, and
// compares the core row counts. It is the local half of the off-host restore
// drill; the operator must copy the verified backup to the off-host destination
// and run the same check there.
func runRestoreDrill(args []string) int {
	fs := flag.NewFlagSet("restore-drill", flag.ContinueOnError)
	dbPath := fs.String("db", "", "live ledger path (absolute)")
	scratch := fs.String("scratch", "", "empty scratch directory (default: temp)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: live ledger:", err)
		return 1
	}
	dir := *scratch
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "kubona-restore-drill-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: create scratch:", err)
			return 1
		}
		defer os.RemoveAll(dir)
	}
	if !filepath.IsAbs(dir) {
		fmt.Fprintln(os.Stderr, "error: --scratch must be absolute")
		return 1
	}
	backup := filepath.Join(dir, "drill-backup.db")
	restored := filepath.Join(dir, "drill-restored.db")
	if code := runBackup([]string{"--db", *dbPath, "--out", backup}); code != 0 {
		return code
	}
	if code := runRestore([]string{"--from", backup, "--db", restored, "--apply"}); code != 0 {
		return code
	}
	live, err := tableCounts(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: live counts:", err)
		return 1
	}
	drill, err := tableCounts(restored)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: restored counts:", err)
		return 1
	}
	for _, table := range []string{"sessions", "jobs", "ledger_events", "claim_records"} {
		if live[table] != drill[table] {
			fmt.Fprintf(os.Stderr, "restore-drill: %s mismatch: live=%d restored=%d\n", table, live[table], drill[table])
			return 1
		}
	}
	fmt.Printf("restore-drill: clean (sessions=%d jobs=%d events=%d claims=%d)\n",
		drill["sessions"], drill["jobs"], drill["ledger_events"], drill["claim_records"])
	return 0
}

func tableCounts(path string) (map[string]int64, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_query_only=1&_busy_timeout=5000", path))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := map[string]int64{}
	for _, table := range []string{"sessions", "jobs", "ledger_events", "claim_records"} {
		var n int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			return nil, fmt.Errorf("%s: %w", table, err)
		}
		out[table] = n
	}
	return out, nil
}
