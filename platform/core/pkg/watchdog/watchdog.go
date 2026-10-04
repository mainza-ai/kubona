package watchdog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

// TaskWatchdog implements the paperclip task watchdog pattern (doc/TASK-WATCHDOG.md)
type TaskWatchdog struct {
	ledger                  *ledger.Ledger
	supervisor              *supervisor.Supervisor
	lastReviewedFingerprint string
	mu                      sync.Mutex
	stopCh                  chan struct{}
}

func NewTaskWatchdog(l *ledger.Ledger, s *supervisor.Supervisor) *TaskWatchdog {
	return &TaskWatchdog{
		ledger:     l,
		supervisor: s,
		stopCh:     make(chan struct{}),
	}
}

func (w *TaskWatchdog) ComputeStopFingerprint(sessionID string) (string, error) {
	// Query non-completed jobs and loops for this session
	query := `SELECT id, status FROM jobs WHERE session_id = ? AND status != 'JOB_STATUS_COMPLETED'`
	rows, err := w.ledger.DB().Query(query, sessionID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	hasher := sha256.New()
	count := 0
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return "", err
		}
		hasher.Write([]byte(fmt.Sprintf("%s:%s;", id, status)))
		count++
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

	if fingerprint == w.lastReviewedFingerprint {
		return false, "already_reviewed", nil
	}

	w.lastReviewedFingerprint = fingerprint
	w.ledger.AppendEvent(sessionID, "task_watchdog_woken", map[string]string{
		"session_id":       sessionID,
		"stop_fingerprint": fingerprint,
		"action":           "invoking_verification_agent",
	})

	return true, fingerprint, nil
}
