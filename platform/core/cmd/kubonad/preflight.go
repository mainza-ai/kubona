package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// kubonad preflight checks the host/state posture before the stack serves:
// rate card validity, ledger chain/FK/schema, and optional backup freshness.
// It is read-only and exits non-zero on a hard failure, so compose can gate
// kubonad on service_completed_successfully.
func runPreflight(args []string) int {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	dbPath := fs.String("db", "", "ledger path (absolute; default KUBONA_DB_PATH or ~/.kubona/kubona.db)")
	backupDir := fs.String("backup-dir", "", "backup directory (absolute; default KUBONA_BACKUP_DIR)")
	maxAge := fs.Duration("max-backup-age", 26*time.Hour, "maximum backup age when checking a configured directory")
	checkRate := fs.Bool("rate-card", true, "validate the configured/embedded rate card")
	checkChain := fs.Bool("verify-chain", true, "verify the ledger hash chain")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fail := false
	report := func(label string, ok bool, detail string) {
		status := "ok"
		if !ok {
			status = "FAIL"
			fail = true
		}
		fmt.Printf("preflight: %-14s %s %s\n", label, status, detail)
	}
	warn := func(label, detail string) {
		fmt.Printf("preflight: %-14s warn %s\n", label, detail)
	}

	if *checkRate {
		if err := rates.ConfigureFromEnv(context.Background()); err != nil {
			report("rate card", false, err.Error())
		} else if err := rates.Validate(time.Now().UTC()); err != nil {
			report("rate card", false, err.Error())
		} else {
			report("rate card", true, fmt.Sprintf("source=%s version=%d", rates.Source(), rates.CardSnapshot().Version))
		}
	}

	resolvedDB := *dbPath
	if resolvedDB == "" {
		resolvedDB = os.Getenv("KUBONA_DB_PATH")
	}
	if resolvedDB == "" {
		home, _ := os.UserHomeDir()
		resolvedDB = filepath.Join(home, ".kubona", "kubona.db")
	}
	if resolvedDB != "" && !filepath.IsAbs(resolvedDB) {
		report("ledger", false, "--db/KUBONA_DB_PATH must be absolute")
	} else if _, err := os.Stat(resolvedDB); os.IsNotExist(err) {
		report("ledger", true, "absent (fresh state)")
	} else if err != nil {
		report("ledger", false, err.Error())
	} else if *checkChain {
		l, err := ledger.OpenLedgerReadOnly(resolvedDB)
		if err != nil {
			report("ledger", false, err.Error())
		} else {
			version, verr := l.SchemaVersion()
			faults, cerr := l.VerifyChain("")
			l.Close()
			switch {
			case verr != nil:
				report("ledger", false, "schema: "+verr.Error())
			case version > ledger.CurrentSchemaVersion:
				report("ledger", false, fmt.Sprintf("schema %d newer than binary %d", version, ledger.CurrentSchemaVersion))
			case cerr != nil:
				report("ledger", false, "chain: "+cerr.Error())
			case len(faults) > 0:
				report("ledger", false, fmt.Sprintf("%d chain fault(s)", len(faults)))
			default:
				report("ledger", true, fmt.Sprintf("schema=%d chain clean", version))
			}
		}
	}

	if resolvedDB != "" && filepath.IsAbs(resolvedDB) {
		if _, err := os.Stat(resolvedDB); err == nil {
			if err := checkForeignKeys(resolvedDB); err != nil {
				report("foreign keys", false, err.Error())
			} else {
				report("foreign keys", true, "clean")
			}
		}
	}

	dir := *backupDir
	if dir == "" {
		dir = os.Getenv("KUBONA_BACKUP_DIR")
	}
	requireBackup := os.Getenv("KUBONA_PREFLIGHT_REQUIRE_BACKUP") == "1"
	if dir == "" {
		report("backups", !requireBackup, "not configured (optional)")
	} else if !filepath.IsAbs(dir) {
		report("backups", false, "--backup-dir/KUBONA_BACKUP_DIR must be absolute")
	} else if _, err := os.Stat(dir); os.IsNotExist(err) {
		report("backups", !requireBackup, "directory absent (optional)")
	} else {
		st := inspectBackups(dir, *maxAge)
		if st.Fresh {
			report("backups", true, fmt.Sprintf("newest %s old", time.Duration(st.AgeSeconds)*time.Second))
		} else if requireBackup {
			report("backups", false, st.Error)
		} else {
			warn("backups", strings.TrimSpace(st.Error+" (set KUBONA_PREFLIGHT_REQUIRE_BACKUP=1 to make this fatal)"))
		}
	}

	if fail {
		fmt.Fprintln(os.Stderr, "preflight: FAILED")
		return 1
	}
	fmt.Println("preflight: clean")
	return 0
}

func checkForeignKeys(path string) error {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_query_only=1&_busy_timeout=5000", path))
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%d orphan row(s); run kubonad fk-quarantine --apply after a verified backup", count)
	}
	return nil
}
