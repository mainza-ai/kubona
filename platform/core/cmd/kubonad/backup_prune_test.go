package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBackup(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("backup"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestBackupPruneDryRunAndApply(t *testing.T) {
	dir := t.TempDir()
	writeBackup(t, dir, "kubona.db.backup-20261029T120000Z")
	writeBackup(t, dir, "kubona.db.backup-20261028T120000Z")
	writeBackup(t, dir, "kubona.db.backup-20261001T120000Z")
	writeBackup(t, dir, "notes.txt")
	if err := os.Mkdir(filepath.Join(dir, "kubona.db.backup-20260101T000000Z"), 0o700); err != nil {
		t.Fatalf("mkdir decoy: %v", err)
	}
	writeBackup(t, dir, "kubona.db.backup-nope")

	args := []string{"--dir", dir, "--now", "2026-10-29T13:00:00Z", "--keep-last", "1", "--keep-daily", "1", "--keep-weekly", "0"}
	if code := runBackupPrune(args); code != 0 {
		t.Fatalf("dry-run exit %d", code)
	}
	for _, name := range []string{
		"kubona.db.backup-20261029T120000Z",
		"kubona.db.backup-20261028T120000Z",
		"kubona.db.backup-20261001T120000Z",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("dry run removed %s", name)
		}
	}

	if code := runBackupPrune(append(args, "--apply")); code != 0 {
		t.Fatalf("apply exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "kubona.db.backup-20261029T120000Z")); err != nil {
		t.Fatalf("newest backup must survive: %v", err)
	}
	for _, name := range []string{"kubona.db.backup-20261028T120000Z", "kubona.db.backup-20261001T120000Z"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Fatalf("pruned backup still exists: %s", name)
		}
	}
	for _, name := range []string{"notes.txt", "kubona.db.backup-nope"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("non-matching file removed: %s", name)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "kubona.db.backup-20260101T000000Z")); err != nil || !info.IsDir() {
		t.Fatalf("matching-name directory must not be removed")
	}
}

func TestBackupPruneRefusesDeleteAll(t *testing.T) {
	dir := t.TempDir()
	writeBackup(t, dir, "kubona.db.backup-20261029T120000Z")
	if code := runBackupPrune([]string{"--dir", dir, "--keep-last", "0", "--keep-daily", "0", "--keep-weekly", "0"}); code != 2 {
		t.Fatalf("zero retention exit %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "kubona.db.backup-20261029T120000Z")); err != nil {
		t.Fatalf("refused delete-all still removed the file")
	}
}
