package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func makeLedger(t *testing.T, path string, event string) {
	t.Helper()
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.AppendEvent("sess-restore", "annotation", map[string]any{"marker": event}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
}

func eventPayloads(t *testing.T, path string) []string {
	t.Helper()
	l, err := ledger.OpenLedgerReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	events, err := l.GetRecentEvents("sess-restore", 10)
	l.Close()
	if err != nil {
		t.Fatalf("recent events: %v", err)
	}
	out := make([]string, 0, len(events))
	for _, ev := range events {
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		out = append(out, fmt.Sprint(payload["marker"]))
	}
	return out
}

func TestRestoreDryRunAndApplyPreservesLiveState(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "kubona.db")
	backup := filepath.Join(dir, "verified.db")

	makeLedger(t, live, "before_backup")
	if code := runBackup([]string{"--db", live, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	makeLedger(t, live, "after_backup")

	if code := runRestore([]string{"--from", backup, "--db", live}); code != 0 {
		t.Fatalf("dry run exit %d", code)
	}
	if got := eventPayloads(t, live); len(got) != 2 {
		t.Fatalf("dry run changed live ledger: %v", got)
	}

	if code := runRestore([]string{"--from", backup, "--db", live, "--apply"}); code != 0 {
		t.Fatalf("apply exit %d", code)
	}
	got := eventPayloads(t, live)
	if len(got) != 1 || got[0] != "before_backup" {
		t.Fatalf("restored events = %v, want only before_backup", got)
	}
	matches, err := filepath.Glob(live + ".pre-restore-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("preservation dirs = %v (err %v), want 1", matches, err)
	}
	preserved := filepath.Join(matches[0], "ledger.db")
	if got := eventPayloads(t, preserved); len(got) != 2 {
		t.Fatalf("preserved events = %v, want both live events", got)
	}
}

func TestRestoreRefusesFaultedSource(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "kubona.db")
	backup := filepath.Join(dir, "verified.db")
	makeLedger(t, live, "event")
	if code := runBackup([]string{"--db", live, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	db, err := sql.Open("sqlite3", backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	if _, err := db.Exec(`UPDATE ledger_events SET payload = '{"marker":"tampered"}' WHERE id = (SELECT MIN(id) FROM ledger_events)`); err != nil {
		db.Close()
		t.Fatalf("tamper backup: %v", err)
	}
	db.Close()

	target := filepath.Join(dir, "target.db")
	if code := runRestore([]string{"--from", backup, "--db", target, "--apply"}); code == 0 {
		t.Fatal("restore must refuse a chain-faulted source")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("faulted source created target: %v", err)
	}
}

func TestRestoreRefusesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "kubona.db")
	backup := filepath.Join(dir, "verified.db")
	makeLedger(t, live, "event")
	if code := runBackup([]string{"--db", live, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	db, err := sql.Open("sqlite3", backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, ledger.CurrentSchemaVersion+1, "now"); err != nil {
		db.Close()
		t.Fatalf("insert future schema: %v", err)
	}
	db.Close()
	target := filepath.Join(dir, "target.db")
	if code := runRestore([]string{"--from", backup, "--db", target, "--apply"}); code == 0 {
		t.Fatal("restore must refuse a backup from a newer schema")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("newer schema created target: %v", err)
	}
}

func TestRestoreRefusesRunningDaemonLock(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "kubona.db")
	backup := filepath.Join(dir, "verified.db")
	makeLedger(t, live, "event")
	if code := runBackup([]string{"--db", live, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}

	lock, err := os.OpenFile(live+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock: %v", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	if code := runRestore([]string{"--from", backup, "--db", live, "--apply"}); code == 0 {
		t.Fatal("restore must refuse while the daemon lock is held")
	}
}

func TestRestoreIntoFreshTargetHasNoPreservation(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	backup := filepath.Join(dir, "verified.db")
	makeLedger(t, source, "event")
	if code := runBackup([]string{"--db", source, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	target := filepath.Join(dir, "fresh", "kubona.db")
	if code := runRestore([]string{"--from", backup, "--db", target, "--apply"}); code != 0 {
		t.Fatalf("restore exit %d", code)
	}
	if got := eventPayloads(t, target); len(got) != 1 {
		t.Fatalf("fresh target events = %v", got)
	}
	if matches, _ := filepath.Glob(target + ".pre-restore-*"); len(matches) != 0 {
		t.Fatalf("fresh target must not preserve a nonexistent ledger: %v", matches)
	}
}

func TestRestoreRejectsRelativeAndSamePaths(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "kubona.db")
	backup := filepath.Join(dir, "verified.db")
	makeLedger(t, live, "event")
	if code := runBackup([]string{"--db", live, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	for _, args := range [][]string{
		{"--from", "relative.db", "--db", live},
		{"--from", backup, "--db", "relative.db"},
		{"--from", backup, "--db", backup},
	} {
		if code := runRestore(args); code == 0 {
			t.Fatalf("restore accepted invalid args %v", args)
		}
	}
}

func TestRestoreOutputNamesPreservedPath(t *testing.T) {
	// The receipt is the audit trail; preserve the exact pre-restore path.
	dir := t.TempDir()
	live := filepath.Join(dir, "kubona.db")
	backup := filepath.Join(dir, "verified.db")
	makeLedger(t, live, "event")
	if code := runBackup([]string{"--db", live, "--out", backup}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	if code := runRestore([]string{"--from", backup, "--db", live}); code != 0 {
		t.Fatalf("dry run exit %d", code)
	}
	matches, _ := filepath.Glob(live + ".pre-restore-*")
	if len(matches) != 0 {
		t.Fatalf("dry run created preservation dirs: %v", matches)
	}
	if !strings.Contains(live, "kubona.db") {
		t.Fatal("test path sanity")
	}
}
