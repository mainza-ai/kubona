package ledger

import (
	"fmt"
	"strings"
	"time"
)

// RetentionConfig bounds the non-evidence tables that otherwise grow forever.
// Ledger events are deliberately absent: they are a hash-chained audit record,
// and deleting a row would break prev_hash verification for every later row.
// What is reaped is chat text (a working transcript, already bounded in the
// prompt) and retracted-memory tombstones (content is already cleared).
type RetentionConfig struct {
	ChatMessageDays int
	TombstoneDays   int
	VacuumPages     int
}

func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{ChatMessageDays: 90, TombstoneDays: 30, VacuumPages: 1000}
}

type RetentionResult struct {
	ChatMessages int64
	Tombstones   int64
}

// RunRetention deletes rows older than the configured windows and asks SQLite
// to return freed pages when incremental auto-vacuum is enabled. It never
// touches ledger_events, jobs, claims, manuscripts, or spend receipts.
func (l *Ledger) RunRetention(cfg RetentionConfig) (RetentionResult, error) {
	var res RetentionResult
	if l == nil || l.db == nil {
		return res, fmt.Errorf("retention: ledger is not open")
	}
	if cfg.ChatMessageDays <= 0 {
		cfg.ChatMessageDays = 90
	}
	if cfg.TombstoneDays <= 0 {
		cfg.TombstoneDays = 30
	}
	if cfg.VacuumPages <= 0 {
		cfg.VacuumPages = 1000
	}
	now := time.Now().UTC()

	if r, err := l.db.Exec(`DELETE FROM chat_messages WHERE created_at IS NOT NULL AND datetime(created_at) < datetime(?)`,
		now.AddDate(0, 0, -cfg.ChatMessageDays)); err != nil {
		return res, fmt.Errorf("retention chat: %w", err)
	} else if n, _ := r.RowsAffected(); n > 0 {
		res.ChatMessages = n
	}

	if r, err := l.db.Exec(`DELETE FROM hindsight_memories WHERE retracted_at IS NOT NULL AND datetime(retracted_at) < datetime(?)`,
		now.AddDate(0, 0, -cfg.TombstoneDays)); err != nil {
		// A store without the table (minimal test ledger) is not a failure.
		if !isNoSuchTable(err) {
			return res, fmt.Errorf("retention tombstones: %w", err)
		}
	} else if n, _ := r.RowsAffected(); n > 0 {
		res.Tombstones = n
	}

	if res.ChatMessages+res.Tombstones > 0 {
		// incremental_vacuum returns one row per page; the result set must be
		// drained or SQLite stops after the first page.
		rows, err := l.db.Query(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", cfg.VacuumPages))
		if err == nil {
			for rows.Next() {
			}
			rows.Close()
		}
		_ = l.AppendEvent("global", "retention_run", map[string]any{
			"chat_messages_deleted": res.ChatMessages,
			"tombstones_deleted":    res.Tombstones,
			"chat_days":             cfg.ChatMessageDays,
			"tombstone_days":        cfg.TombstoneDays,
		})
	}
	return res, nil
}

func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
}

// SnapshotTo writes a consistent VACUUM INTO copy at path. Exported for the
// operator vacuum/migration command; the daemon's own migration path uses the
// same helper.
func (l *Ledger) SnapshotTo(path string) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("snapshot: ledger is not open")
	}
	return snapshotBeforeMigration(l.db, path)
}
