package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad vacuum --db /abs/ledger.db [--snapshot] enables incremental
// auto-vacuum on an existing database (a one-time rebuild) and reclaims free
// pages. It refuses relative paths and refuses to run while the daemon holds
// the state lock; a snapshot is taken first by default.
func runVacuum(args []string) int {
	fs := flag.NewFlagSet("vacuum", flag.ContinueOnError)
	dbPath := fs.String("db", "", "absolute path to the SQLite ledger")
	snapshot := fs.Bool("snapshot", true, "write a verified pre-vacuum snapshot")
	keep := fs.Int("keep-pages", 1000, "incremental_vacuum pages per pass")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: ledger not readable:", err)
		return 1
	}
	l, err := ledger.OpenLedger(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger (is the daemon stopped?):", err)
		return 1
	}
	defer l.Close()

	if *snapshot {
		target := fmt.Sprintf("%s.pre-vacuum-%s", *dbPath, time.Now().UTC().Format("20060102T150405.000000000Z"))
		if err := l.SnapshotTo(target); err != nil {
			fmt.Fprintln(os.Stderr, "error: pre-vacuum snapshot failed:", err)
			return 1
		}
		fmt.Println("snapshot:", target)
	}

	before := 0
	_ = l.DB().QueryRow(`PRAGMA auto_vacuum`).Scan(&before)
	if _, err := l.DB().Exec(`PRAGMA auto_vacuum=INCREMENTAL`); err != nil {
		fmt.Fprintln(os.Stderr, "error: set auto_vacuum:", err)
		return 1
	}
	if _, err := l.DB().Exec(`VACUUM`); err != nil {
		fmt.Fprintln(os.Stderr, "error: vacuum:", err)
		return 1
	}
	rows, err := l.DB().Query(`PRAGMA incremental_vacuum(` + strconv.Itoa(*keep) + `)`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: incremental_vacuum:", err)
		return 1
	}
	for rows.Next() {
	}
	rows.Close()

	after := 0
	_ = l.DB().QueryRow(`PRAGMA auto_vacuum`).Scan(&after)
	var freelist int
	_ = l.DB().QueryRow(`PRAGMA freelist_count`).Scan(&freelist)
	fmt.Printf("auto_vacuum: %d -> %d; freelist=%d\n", before, after, freelist)
	return 0
}
