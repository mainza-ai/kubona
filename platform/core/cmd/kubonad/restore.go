package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad restore --from /abs/verified.db --db /abs/kubona.db [--apply]
//
// Restore is deliberately two-step: the default is a dry run that proves the
// source chain and reports exactly what would move. --apply acquires the
// daemon's state lock, preserves the current database and WAL sidecars under
// a timestamped pre-restore name, atomically installs the verified source,
// then verifies the installed copy. Any failure rolls the preserved state
// back. A source that does not verify is never installed.
func runRestore(args []string) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	from := fs.String("from", "", "verified backup path (absolute)")
	dbPath := fs.String("db", "", "live ledger path (absolute)")
	apply := fs.Bool("apply", false, "install the backup (default: dry run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *from == "" || !filepath.IsAbs(*from) {
		fmt.Fprintln(os.Stderr, "error: --from must be an absolute path")
		return 1
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	if filepath.Clean(*from) == filepath.Clean(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --from and --db must differ")
		return 1
	}
	if info, err := os.Lstat(*from); err != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(os.Stderr, "error: backup is not a regular readable file: %v\n", err)
		return 1
	}
	if err := verifyRestoreSource(*from); err != nil {
		fmt.Fprintln(os.Stderr, "error: refusing restore:", err)
		return 1
	}
	sum, size, err := fileSHA256(*from)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot hash backup:", err)
		return 1
	}

	preRestore := fmt.Sprintf("%s.pre-restore-%s", *dbPath, time.Now().UTC().Format("20060102T150405.000000000Z"))
	targetExists := false
	if _, err := os.Stat(*dbPath); err == nil {
		targetExists = true
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "error: cannot stat live ledger:", err)
		return 1
	}

	if !*apply {
		fmt.Printf("restore: dry run (no writes)\nsource: %s\nsize: %d\nsha256: %s\nchain: clean\ntarget: %s\n", *from, size, sum, *dbPath)
		if targetExists {
			fmt.Printf("preserve: %s\n", preRestore)
		} else {
			fmt.Println("preserve: none (target does not exist)")
		}
		fmt.Println("next: re-run with --apply only while the daemon is stopped")
		return 0
	}

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "error: create state directory:", err)
		return 1
	}
	lock, err := acquireStateLock(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer releaseStateLock(lock)

	// Stage the source in the target directory so the final rename is atomic
	// and cannot cross filesystems.
	tmp, err := os.CreateTemp(filepath.Dir(*dbPath), ".kubona-restore-*.tmp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: create restore staging file:", err)
		return 1
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		fmt.Fprintln(os.Stderr, "error: chmod staging file:", err)
		return 1
	}
	src, err := os.Open(*from)
	if err != nil {
		tmp.Close()
		fmt.Fprintln(os.Stderr, "error: open backup:", err)
		return 1
	}
	_, copyErr := io.Copy(tmp, src)
	src.Close()
	if copyErr != nil {
		tmp.Close()
		fmt.Fprintln(os.Stderr, "error: stage backup:", copyErr)
		return 1
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		fmt.Fprintln(os.Stderr, "error: sync staged backup:", err)
		return 1
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "error: close staged backup:", err)
		return 1
	}

	preserved, err := preserveLiveLedger(*dbPath, preRestore, targetExists)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: preserve live ledger:", err)
		return 1
	}
	if err := os.Rename(tmpName, *dbPath); err != nil {
		_ = restorePreserved(*dbPath, preserved)
		fmt.Fprintln(os.Stderr, "error: install backup:", err)
		return 1
	}
	if err := syncDirectory(filepath.Dir(*dbPath)); err != nil {
		fmt.Fprintln(os.Stderr, "warning: sync state directory:", err)
	}

	if err := verifyRestoreSource(*dbPath); err != nil {
		if rollbackErr := restorePreserved(*dbPath, preserved); rollbackErr != nil {
			fmt.Fprintf(os.Stderr, "error: installed copy failed verification (%v) and rollback failed: %v\n", err, rollbackErr)
			return 1
		}
		fmt.Fprintf(os.Stderr, "error: installed copy failed verification (%v); previous state restored\n", err)
		return 1
	}

	installedSum, installedSize, err := fileSHA256(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: hash installed ledger:", err)
		return 1
	}
	fmt.Printf("restore: applied\nsource: %s\ninstalled: %s\nsize: %d\nsha256: %s\nchain: clean\n", *from, *dbPath, installedSize, installedSum)
	if preserved.dir != "" {
		fmt.Printf("preserved: %s\n", preserved.dir)
	} else {
		fmt.Println("preserved: none (target did not exist)")
	}
	return 0
}

type preservedLedger struct {
	dir    string
	hadWAL bool
	hadSHM bool
}

func acquireStateLock(dbPath string) (*os.File, error) {
	lock, err := os.OpenFile(dbPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("state is locked by a running daemon (%s): stop it before restoring", dbPath)
	}
	return lock, nil
}

func releaseStateLock(lock *os.File) {
	if lock == nil {
		return
	}
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}

// preserveLiveLedger renames the live DB and sidecars to a timestamped
// directory. Renaming (not copying) keeps the operation fast and leaves the
// WAL content intact for rollback or forensic inspection.
func preserveLiveLedger(dbPath, stamp string, exists bool) (preservedLedger, error) {
	if !exists {
		return preservedLedger{}, nil
	}
	if _, err := os.Stat(stamp); err == nil {
		return preservedLedger{}, fmt.Errorf("refusing to overwrite existing preservation path %s", stamp)
	}
	if err := os.Mkdir(stamp, 0o700); err != nil {
		return preservedLedger{}, err
	}
	p := preservedLedger{dir: stamp}
	if err := os.Rename(dbPath, filepath.Join(stamp, "ledger.db")); err != nil {
		return preservedLedger{}, err
	}
	rollback := func() {
		_ = os.Rename(filepath.Join(stamp, "ledger.db"), dbPath)
		if p.hadWAL {
			_ = os.Rename(filepath.Join(stamp, "ledger.db-wal"), dbPath+"-wal")
		}
		if p.hadSHM {
			_ = os.Rename(filepath.Join(stamp, "ledger.db-shm"), dbPath+"-shm")
		}
		_ = os.Remove(stamp)
	}
	if _, err := os.Stat(dbPath + "-wal"); err == nil {
		if err := os.Rename(dbPath+"-wal", filepath.Join(stamp, "ledger.db-wal")); err != nil {
			rollback()
			return preservedLedger{}, err
		}
		p.hadWAL = true
	}
	if _, err := os.Stat(dbPath + "-shm"); err == nil {
		if err := os.Rename(dbPath+"-shm", filepath.Join(stamp, "ledger.db-shm")); err != nil {
			rollback()
			return preservedLedger{}, err
		}
		p.hadSHM = true
	}
	return p, nil
}

func restorePreserved(dbPath string, p preservedLedger) error {
	if p.dir == "" {
		return nil
	}
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Rename(filepath.Join(p.dir, "ledger.db"), dbPath); err != nil {
		return err
	}
	if p.hadWAL {
		if err := os.Rename(filepath.Join(p.dir, "ledger.db-wal"), dbPath+"-wal"); err != nil {
			return err
		}
	}
	if p.hadSHM {
		if err := os.Rename(filepath.Join(p.dir, "ledger.db-shm"), dbPath+"-shm"); err != nil {
			return err
		}
	}
	return nil
}

func verifyRestoreSource(path string) error {
	l, err := ledger.OpenLedgerReadOnly(path)
	if err != nil {
		return err
	}
	version, verr := l.SchemaVersion()
	faults, err := l.VerifyChain("")
	l.Close()
	if verr != nil {
		return fmt.Errorf("schema version: %w", verr)
	}
	if version > ledger.CurrentSchemaVersion {
		return fmt.Errorf("backup schema version %d is newer than this binary supports (%d)", version, ledger.CurrentSchemaVersion)
	}
	if err != nil {
		return err
	}
	if len(faults) != 0 {
		return fmt.Errorf("%d chain fault(s)", len(faults))
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_query_only=1", path))
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("sqlite integrity_check: %s", result)
	}
	return nil
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
