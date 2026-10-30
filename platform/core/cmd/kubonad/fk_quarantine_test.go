package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func seedOrphans(t *testing.T, path string) {
	t.Helper()
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES ('orphan-msg', 'ghost-session', 'user', 'hello', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan message: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, created_at) VALUES ('orphan-job', 'ghost-session', 'queued', 'test', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan job: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO campaign_versions (session_id, state_json, trigger, created_at) VALUES ('ghost-session', '{}', 'test', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan campaign version: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO claim_records (id, session_id, turn_kind, statement, verdict, created_at) VALUES ('orphan-claim', 'ghost-session', 'test', 's', 'v', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan claim: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
}

func TestFKQuarantineDryRunWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dry.db")
	seedOrphans(t, path)

	if code := runFKQuarantine([]string{"--db", path}); code != 1 {
		t.Fatalf("dry-run exit %d, want 1", code)
	}
	l, err := ledger.OpenLedgerReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer l.Close()
	var messages, jobs, quarantine int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&messages); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&jobs); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_fk_orphans'`).Scan(&quarantine); err != nil {
		t.Fatalf("check quarantine table: %v", err)
	}
	if messages != 1 || jobs != 1 || quarantine != 0 {
		t.Fatalf("dry run changed data: messages=%d jobs=%d quarantine_table=%d", messages, jobs, quarantine)
	}
}

func TestFKQuarantineApplyPreservesRowsAndClearsOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apply.db")
	seedOrphans(t, path)

	if code := runFKQuarantine([]string{"--db", path, "--apply"}); code != 0 {
		t.Fatalf("apply exit %d, want 0", code)
	}
	l, err := ledger.OpenLedgerReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer l.Close()
	var messages, jobs, quarantined int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&messages); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&jobs); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM _fk_orphans`).Scan(&quarantined); err != nil {
		t.Fatalf("count quarantine: %v", err)
	}
	if messages != 0 || jobs != 0 {
		t.Fatalf("live rows remain: messages=%d jobs=%d", messages, jobs)
	}
	if quarantined != 4 {
		t.Fatalf("quarantined = %d, want 4", quarantined)
	}
	var rowJSON string
	if err := l.DB().QueryRow(`SELECT row_json FROM _fk_orphans WHERE table_name='jobs'`).Scan(&rowJSON); err != nil {
		t.Fatalf("read quarantine json: %v", err)
	}
	if !strings.Contains(rowJSON, "ghost-session") || !strings.Contains(rowJSON, "orphan-job") {
		t.Fatalf("quarantine row lost fields: %s", rowJSON)
	}
}

func TestFKQuarantineCleanLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clean.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	l.Close()
	if code := runFKQuarantine([]string{"--db", path, "--apply"}); code != 0 {
		t.Fatalf("clean apply exit %d, want 0", code)
	}
}
