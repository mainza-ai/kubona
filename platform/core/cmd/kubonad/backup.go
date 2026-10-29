package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad backup --db /abs/ledger.db [--out /abs/backup.db]
//
// Copies a live SQLite ledger through VACUUM INTO (a consistent snapshot
// that includes WAL content) and verifies the copy's hash chain before it
// is accepted. The source is opened read-only, so the running daemon keeps
// its exclusive writer lock. The output is never overwritten.
//
// A backup that has not been verified is not a backup. This command
// verifies the copy with the same read-only VerifyChain path as
// `kubonad verify`, then prints the sha256 so the caller can pin it.
func runBackup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to SQLite WAL ledger (must be absolute)")
	outPath := fs.String("out", "", "output path (absolute; default: <db>.backup-<UTC>)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path (relative ledger paths refuse: CWD-dependent backups prove nothing)")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: ledger not readable:", err)
		return 1
	}

	target := *outPath
	if target == "" {
		target = fmt.Sprintf("%s.backup-%s", *dbPath, time.Now().UTC().Format("20060102T150405Z"))
	}
	if !filepath.IsAbs(target) {
		fmt.Fprintln(os.Stderr, "error: --out must be an absolute path")
		return 1
	}
	if _, err := os.Stat(target); err == nil {
		fmt.Fprintf(os.Stderr, "error: refusing to overwrite existing %s\n", target)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot create backup directory:", err)
		return 1
	}

	// VACUUM INTO needs a connection that may write the destination; a
	// mode=ro connection refuses it. This connection is opened mode=rw but
	// only reads the source and writes a new file. The daemon's single-writer
	// guard is a Go-level flock on <db>.lock, not a SQLite lock, so this
	// operator path does not fight the live writer; WAL readers are allowed.
	src, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw&_busy_timeout=5000", *dbPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger for snapshot:", err)
		return 1
	}
	if err := src.Ping(); err != nil {
		src.Close()
		fmt.Fprintln(os.Stderr, "error: ledger ping:", err)
		return 1
	}
	// VACUUM INTO takes a SQL string literal, not a bind parameter.
	quoted := "'" + strings.ReplaceAll(target, "'", "''") + "'"
	_, execErr := src.Exec("VACUUM INTO " + quoted)
	src.Close()
	if execErr != nil {
		fmt.Fprintln(os.Stderr, "error: backup snapshot failed:", execErr)
		_ = os.Remove(target)
		return 1
	}

	copyLedger, err := ledger.OpenLedgerReadOnly(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: backup is unreadable:", err)
		return 1
	}
	faults, verifyErr := copyLedger.VerifyChain("")
	copyLedger.Close()
	if verifyErr != nil {
		fmt.Fprintln(os.Stderr, "error: backup verification failed:", verifyErr)
		return 1
	}
	if len(faults) != 0 {
		fmt.Fprintf(os.Stderr, "error: backup has %d chain fault(s); refusing it\n", len(faults))
		return 1
	}

	sum, size, err := fileSHA256(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot hash backup:", err)
		return 1
	}
	fmt.Printf("backup: %s\nsize: %d\nsha256: %s\nchain: clean\n", target, size, sum)
	return 0
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
