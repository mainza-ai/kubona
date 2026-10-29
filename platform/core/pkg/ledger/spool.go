package ledger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// spooledEvent is the durable fallback when the ledger append fails after a
// side effect (for example an Atlas dispatch). The file contains the exact
// event the ledger would have accepted; startup replay appends it and
// removes the file only after the ledger acknowledges it.
type spooledEvent struct {
	SessionID   string          `json:"session_id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	LedgerError string          `json:"ledger_error"`
	CreatedAt   string          `json:"created_at"`
}

// AppendEventWithSpool tries the ledger first. If that fails, it writes the
// event to <state>/provenance-spool atomically and reports sealed=false.
// An error means both the ledger and the spool failed: the caller must not
// claim a durable receipt.
func (l *Ledger) AppendEventWithSpool(sessionID, eventType string, payload any) (sealed bool, err error) {
	appendErr := l.AppendEvent(sessionID, eventType, payload)
	if appendErr == nil {
		return true, nil
	}
	if spoolErr := l.spoolEvent(sessionID, eventType, payload, appendErr); spoolErr != nil {
		return false, fmt.Errorf("ledger append failed (%v) and provenance spool failed: %w", appendErr, spoolErr)
	}
	slog.Warn("[ledger] provenance append failed; event durably spooled", "session_id", sessionID, "event_type", eventType, "error", appendErr)
	return false, nil
}

func (l *Ledger) spoolDir() string {
	return filepath.Join(filepath.Dir(l.dbPath), "provenance-spool")
}

func (l *Ledger) spoolEvent(sessionID, eventType string, payload any, ledgerErr error) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal spooled payload: %w", err)
	}
	record, err := json.Marshal(spooledEvent{
		SessionID:   sessionID,
		EventType:   eventType,
		Payload:     payloadJSON,
		LedgerError: ledgerErr.Error(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal spooled event: %w", err)
	}
	dir := l.spoolDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create spool dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "provenance-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create spool file: %w", err)
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(record); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write spool file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("sync spool file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close spool file: %w", err)
	}
	final := strings.TrimSuffix(name, ".tmp")
	if err := os.Rename(name, final); err != nil {
		cleanup()
		return fmt.Errorf("publish spool file: %w", err)
	}
	return nil
}

// replayProvenanceSpool appends any spooled events and removes each file only
// after the ledger accepts it. Called by the writer open before serving. A
// failed replay is logged and left in place; startup continues so the
// operator can still audit/repair the spool.
func (l *Ledger) replayProvenanceSpool() {
	dir := l.spoolDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("[ledger] cannot read provenance spool", "dir", dir, "error", err)
		}
		return
	}
	replayed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("[ledger] cannot read spooled event", "path", path, "error", err)
			continue
		}
		var ev spooledEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			slog.Warn("[ledger] spooled event is invalid; left in place", "path", path, "error", err)
			continue
		}
		if err := l.AppendEvent(ev.SessionID, ev.EventType, ev.Payload); err != nil {
			slog.Warn("[ledger] spool replay failed; left in place", "path", path, "error", err)
			continue
		}
		if err := os.Remove(path); err != nil {
			slog.Warn("[ledger] replayed spooled event but could not remove it", "path", path, "error", err)
			continue
		}
		replayed++
	}
	if replayed != 0 {
		slog.Info("[ledger] replayed provenance spool", "count", replayed)
	}
}
