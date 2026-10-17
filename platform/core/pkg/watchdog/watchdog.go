package watchdog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

// TaskWatchdog implements the paperclip task watchdog pattern (doc/TASK-WATCHDOG.md)
type TaskWatchdog struct {
	ledger     *ledger.Ledger
	supervisor *supervisor.Supervisor
	// W0.21: fingerprints are per-session — the old global let sessions
	// evict each other into infinite re-wake.
	lastReviewed map[string]string
	mu           sync.Mutex
}

func NewTaskWatchdog(l *ledger.Ledger, s *supervisor.Supervisor) *TaskWatchdog {
	return &TaskWatchdog{
		ledger:       l,
		supervisor:   s,
		lastReviewed: make(map[string]string),
	}
}

func (w *TaskWatchdog) ComputeStopFingerprint(sessionID string) (string, error) {
	// W0.21: jobs AND loops (the comment always claimed both; the query
	// read jobs only, so diverged loops never woke). Terminal sets come
	// from the single sources: FAILED jobs are settled (they used to keep
	// subtrees "unsettled" forever), and loop terminality matches the
	// deletion blockers. ORDER BY id: without it the same set hashes
	// differently per row order — phantom "new" fingerprints re-woke.
	jobQ := `SELECT id, status FROM jobs WHERE session_id = ? AND status NOT IN (` +
		session.Placeholders(len(session.TerminalJobStatuses)) + `) ORDER BY id`
	jobArgs := make([]any, 0, len(session.TerminalJobStatuses)+1)
	jobArgs = append(jobArgs, sessionID)
	for _, st := range session.TerminalJobStatuses {
		jobArgs = append(jobArgs, st)
	}
	loopQ := `SELECT id, status FROM loops WHERE session_id = ? AND status NOT IN (` +
		session.Placeholders(len(session.TerminalLoopStatuses)) + `) ORDER BY id`
	loopArgs := make([]any, 0, len(session.TerminalLoopStatuses)+1)
	loopArgs = append(loopArgs, sessionID)
	for _, st := range session.TerminalLoopStatuses {
		loopArgs = append(loopArgs, st)
	}

	hasher := sha256.New()
	count := 0
	writeRows := func(query string, args []any) error {
		rows, err := w.ledger.DB().Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, status string
			if err := rows.Scan(&id, &status); err != nil {
				return err
			}
			hasher.Write([]byte(fmt.Sprintf("%s:%s;", id, status)))
			count++
		}
		return rows.Err()
	}
	if err := writeRows(jobQ, jobArgs); err != nil {
		return "", err
	}
	if err := writeRows(loopQ, loopArgs); err != nil {
		return "", err
	}

	if count == 0 {
		return "", nil // Subtree is fully settled
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (w *TaskWatchdog) ScanSubtree(sessionID string) (bool, string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	fingerprint, err := w.ComputeStopFingerprint(sessionID)
	if err != nil {
		return false, "", err
	}

	if fingerprint == "" {
		return false, "all_settled", nil
	}

	if fingerprint == w.lastReviewed[sessionID] {
		return false, "already_reviewed", nil
	}

	w.lastReviewed[sessionID] = fingerprint
	w.ledger.AppendEvent(sessionID, "task_watchdog_woken", map[string]string{
		"session_id":       sessionID,
		"stop_fingerprint": fingerprint,
		"action":           "invoking_verification_agent",
	})

	return true, fingerprint, nil
}
