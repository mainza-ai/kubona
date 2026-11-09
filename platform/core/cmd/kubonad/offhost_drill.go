package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

//	kubonad offhost-drill --db /abs/kubona.db --offhost-dir /mnt/offhost \
//	  --age-recipient age1... --age-identity /abs/identity.txt
//
// The command proves the full recovery path, not just a local copy:
//  1. verified SQLite backup;
//  2. age-encrypt into the off-host destination;
//  3. age-decrypt back into a scratch directory (reads the off-host copy);
//  4. restore into a fresh SQLite file;
//  5. compare core row counts against the live ledger.
//
// The destination must be a genuinely separate/off-host mount; the caller
// owns that physical property (the command cannot invent it).
func runOffhostDrill(args []string) int {
	fs := flag.NewFlagSet("offhost-drill", flag.ContinueOnError)
	dbPath := fs.String("db", "", "live ledger path (absolute)")
	offhostDir := fs.String("offhost-dir", "", "off-host destination directory (absolute)")
	recipient := fs.String("age-recipient", "", "age public recipient for the encrypted backup")
	identity := fs.String("age-identity", "", "age identity file used to decrypt the off-host copy")
	scratch := fs.String("scratch", "", "scratch directory (default: temp)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	if *offhostDir == "" || !filepath.IsAbs(*offhostDir) {
		fmt.Fprintln(os.Stderr, "error: --offhost-dir must be an absolute off-host path")
		return 1
	}
	if *recipient == "" || *identity == "" {
		fmt.Fprintln(os.Stderr, "error: --age-recipient and --age-identity are required")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: live ledger:", err)
		return 1
	}
	if st, err := os.Stat(*offhostDir); err != nil || !st.IsDir() {
		fmt.Fprintln(os.Stderr, "error: off-host directory is not a directory:", err)
		return 1
	}
	if _, err := exec.LookPath("age"); err != nil {
		fmt.Fprintln(os.Stderr, "error: age binary not found: install age before running the encrypted drill")
		return 1
	}
	if _, err := os.Stat(*identity); err != nil {
		fmt.Fprintln(os.Stderr, "error: age identity file:", err)
		return 1
	}
	dir := *scratch
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "kubona-offhost-drill-*")
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
	backup := filepath.Join(dir, "live.db")
	encrypted := filepath.Join(*offhostDir, fmt.Sprintf("kubona-offhost-%s.db.age", time.Now().UTC().Format("20060102T150405Z")))
	decrypted := filepath.Join(dir, "offhost-restored.db.age")
	restored := filepath.Join(dir, "restored.db")

	if code := runBackup([]string{"--db", *dbPath, "--out", backup}); code != 0 {
		return code
	}
	if err := ageEncrypt(*recipient, backup, encrypted); err != nil {
		fmt.Fprintln(os.Stderr, "error: age encrypt:", err)
		return 1
	}
	if err := ageDecrypt(*identity, encrypted, decrypted); err != nil {
		fmt.Fprintln(os.Stderr, "error: age decrypt from off-host copy:", err)
		return 1
	}
	if code := runRestore([]string{"--from", decrypted, "--db", restored, "--apply"}); code != 0 {
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
			fmt.Fprintf(os.Stderr, "offhost-drill: %s mismatch: live=%d restored=%d\n", table, live[table], drill[table])
			return 1
		}
	}
	hash, err := sha256File(encrypted)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: hash encrypted artifact:", err)
		return 1
	}
	fmt.Printf("offhost-drill: clean (artifact=%s sha256=%s sessions=%d jobs=%d events=%d claims=%d)\n",
		encrypted, hash, drill["sessions"], drill["jobs"], drill["ledger_events"], drill["claim_records"])
	return 0
}

func ageEncrypt(recipient, in, out string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "age", "--recipient", recipient, "--output", out, in)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(raw))
	}
	return nil
}

func ageDecrypt(identity, in, out string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "age", "--decrypt", "--identity", identity, "--output", out, in)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(raw))
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
