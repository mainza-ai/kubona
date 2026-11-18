package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func touchBackup(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not-a-real-ledger"), 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	return path
}

func TestBackupStatusFreshAndStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	fresh := touchBackup(t, dir, "kubona.db.backup-"+now.Format("20060102T150405Z"))
	st := inspectBackups(dir, 26*time.Hour)
	if !st.Fresh || st.Path != fresh || st.Encrypted {
		t.Fatalf("fresh status = %+v", st)
	}

	old := now.Add(-72 * time.Hour)
	oldPath := touchBackup(t, dir, "kubona.db.backup-"+old.Format("20060102T150405Z"))
	st = inspectBackups(dir, 26*time.Hour)
	if !st.Fresh || st.Path != fresh || st.Path == oldPath || st.BackupCount != 2 {
		t.Fatalf("newest selection = %+v", st)
	}
}

func TestBackupStatusStaleNewest(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().UTC().Add(-72 * time.Hour)
	touchBackup(t, dir, "kubona.db.backup-"+old.Format("20060102T150405Z"))
	st := inspectBackups(dir, 26*time.Hour)
	if st.Fresh || st.BackupCount != 1 {
		t.Fatalf("stale status = %+v", st)
	}
}

func TestBackupStatusFailureMarkerIsUnhealthy(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	touchBackup(t, dir, "kubona.db.backup-"+now.Format("20060102T150405Z"))
	if err := os.WriteFile(filepath.Join(dir, ".last-success"), []byte(now.Add(-time.Hour).Format(time.RFC3339)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".last-failure"), []byte(now.Format(time.RFC3339)), 0o600); err != nil {
		t.Fatal(err)
	}
	st := inspectBackups(dir, 26*time.Hour)
	if st.Fresh || st.Error == "" {
		t.Fatalf("failure marker status = %+v", st)
	}
}

func TestBackupStatusEncryptedAndMissing(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	touchBackup(t, dir, "kubona.db.backup-"+now.Format("20060102T150405Z")+".age")
	st := inspectBackups(dir, 26*time.Hour)
	if !st.Fresh || !st.Encrypted {
		t.Fatalf("encrypted status = %+v", st)
	}

	empty := t.TempDir()
	st = inspectBackups(empty, 26*time.Hour)
	if st.Fresh || st.Error == "" {
		t.Fatalf("missing status = %+v", st)
	}
}

func TestBackupStatusEnforcesDeclaredEncryption(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	touchBackup(t, dir, "kubona.db.backup-"+now.Format("20060102T150405Z"))

	t.Setenv("KUBONA_BACKUP_REQUIRE_ENCRYPTION", "1")
	if code := runBackupStatus([]string{"--dir", dir}); code == 0 {
		t.Fatal("plaintext backup passed while encryption is required")
	}

	// With the requirement off, the same directory is healthy.
	t.Setenv("KUBONA_BACKUP_REQUIRE_ENCRYPTION", "0")
	if code := runBackupStatus([]string{"--dir", dir}); code != 0 {
		t.Fatalf("plaintext backup failed without a requirement: exit %d", code)
	}

	// An encrypted newest artifact satisfies the requirement.
	t.Setenv("KUBONA_BACKUP_REQUIRE_ENCRYPTION", "1")
	touchBackup(t, dir, "kubona.db.backup-"+now.Add(time.Minute).Format("20060102T150405Z")+".age")
	if code := runBackupStatus([]string{"--dir", dir}); code != 0 {
		t.Fatalf("encrypted backup failed under the requirement: exit %d", code)
	}
}
