package supervisor

// C10 — operator steering. A steer is an instruction attached to a live run;
// it is never executed as a tool call. The next Step applies it: the row is
// marked applied, a `loop_steered` event is sealed, and a lead part carries
// the instruction into the run lane.

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type LoopSteer struct {
	ID        string `json:"id"`
	LoopID    string `json:"loop_id"`
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	AppliedAt string `json:"applied_at,omitempty"`
}

const maxSteerRunes = 500

// Steer queues one instruction for a live loop. Paused/terminal loops refuse:
// nothing would consume the instruction, and pretending it was accepted would
// be a lie.
func (s *Supervisor) Steer(loopID, text string) (*LoopSteer, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("steer text is required")
	}
	if len([]rune(text)) > maxSteerRunes {
		return nil, fmt.Errorf("steer text is %d runes; max is %d", len([]rune(text)), maxSteerRunes)
	}
	s.mu.RLock()
	loop, ok := s.loops[loopID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("loop not found: %s", loopID)
	}
	if loop.Status != LoopRunning && loop.Status != LoopRescoped {
		return nil, fmt.Errorf("loop %s is %s: steering applies only to a running run", loopID, loop.Status)
	}
	steer := &LoopSteer{
		ID: uuid.NewString(), LoopID: loopID, SessionID: loop.SessionID,
		Text: text, Status: "pending", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, err := s.ledger.DB().Exec(`INSERT INTO loop_steers (id, loop_id, session_id, text, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)`, steer.ID, steer.LoopID, steer.SessionID, steer.Text, steer.CreatedAt)
	if err != nil {
		return nil, err
	}
	return steer, nil
}

// QueueSteer inserts a pending instruction for a loop that is not held in
// the supervisor map (autoresearch engines are owned by their own registry).
// The caller must verify the run is live; this method only validates text.
func (s *Supervisor) QueueSteer(loopID, sessionID, text string) (*LoopSteer, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("steer text is required")
	}
	if len([]rune(text)) > maxSteerRunes {
		return nil, fmt.Errorf("steer text is %d runes; max is %d", len([]rune(text)), maxSteerRunes)
	}
	if loopID == "" || sessionID == "" {
		return nil, fmt.Errorf("steer needs loop and session ids")
	}
	steer := &LoopSteer{
		ID: uuid.NewString(), LoopID: loopID, SessionID: sessionID,
		Text: text, Status: "pending", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, err := s.ledger.DB().Exec(`INSERT INTO loop_steers (id, loop_id, session_id, text, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)`, steer.ID, steer.LoopID, steer.SessionID, steer.Text, steer.CreatedAt)
	if err != nil {
		return nil, err
	}
	return steer, nil
}

// MarkSteersApplied records that an autoresearch model prompt consumed the
// given pending rows. Conditional updates keep a concurrent cancel/edit from
// being overwritten: only rows still pending are marked.
func (s *Supervisor) MarkSteersApplied(loopID, sessionID string, ids []string) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		var text string
		if err := s.ledger.DB().QueryRow(`SELECT text FROM loop_steers WHERE id = ? AND loop_id = ? AND status = 'pending'`, id, loopID).Scan(&text); err != nil {
			continue
		}
		res, err := s.ledger.DB().Exec(
			`UPDATE loop_steers SET status = 'applied', applied_at = ? WHERE id = ? AND loop_id = ? AND status = 'pending'`, now, id, loopID)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n != 1 {
			continue
		}
		_ = s.ledger.AppendEvent(sessionID, "loop_steered", map[string]any{
			"loop_id": loopID, "steer_id": id, "text": text, "consumer": "autoresearch_prompt",
		})
		_, _ = s.RecordPart(sessionID, LoopPart{
			LoopID: loopID, Helper: "lead", Title: "Steering applied",
			Goal: text, State: "done",
		})
	}
}

// PendingSteers lists queued instructions for a loop, oldest first.
func (s *Supervisor) PendingSteers(loopID string) ([]LoopSteer, error) {
	rows, err := s.ledger.DB().Query(
		`SELECT id, loop_id, session_id, text, status, created_at, COALESCE(applied_at, '') FROM loop_steers WHERE loop_id = ? AND status = 'pending' ORDER BY created_at, rowid`, loopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoopSteer
	for rows.Next() {
		var item LoopSteer
		if err := rows.Scan(&item.ID, &item.LoopID, &item.SessionID, &item.Text, &item.Status, &item.CreatedAt, &item.AppliedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// UpdateSteer edits a pending instruction before the next boundary. The
// conditional update makes a race with applySteers fail closed: if the row is
// no longer pending, the edit is refused instead of rewriting applied history.
func (s *Supervisor) UpdateSteer(loopID, steerID, text string) (*LoopSteer, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("steer text is required")
	}
	if len([]rune(text)) > maxSteerRunes {
		return nil, fmt.Errorf("steer text is %d runes; max is %d", len([]rune(text)), maxSteerRunes)
	}
	s.mu.RLock()
	loop, ok := s.loops[loopID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("loop not found: %s", loopID)
	}
	if loop.Status != LoopRunning && loop.Status != LoopRescoped && loop.Status != LoopPaused {
		return nil, fmt.Errorf("loop %s is %s: only a live or paused run can edit a queued steer", loopID, loop.Status)
	}
	res, err := s.ledger.DB().Exec(`UPDATE loop_steers SET text = ? WHERE id = ? AND loop_id = ? AND status = 'pending'`, text, steerID, loopID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, fmt.Errorf("steer %s is not pending", steerID)
	}
	_ = s.ledger.AppendEvent(loop.SessionID, "loop_steer_updated", map[string]any{
		"loop_id": loopID, "steer_id": steerID, "text": text,
	})
	return &LoopSteer{ID: steerID, LoopID: loopID, SessionID: loop.SessionID, Text: text, Status: "pending"}, nil
}

// CancelSteer removes a pending instruction before the boundary. Applied
// history is immutable; only pending rows can be cancelled.
func (s *Supervisor) CancelSteer(loopID, steerID string) error {
	s.mu.RLock()
	loop, ok := s.loops[loopID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("loop not found: %s", loopID)
	}
	if loop.Status != LoopRunning && loop.Status != LoopRescoped && loop.Status != LoopPaused {
		return fmt.Errorf("loop %s is %s: only a live or paused run can cancel a queued steer", loopID, loop.Status)
	}
	res, err := s.ledger.DB().Exec(`UPDATE loop_steers SET status = 'cancelled' WHERE id = ? AND loop_id = ? AND status = 'pending'`, steerID, loopID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("steer %s is not pending", steerID)
	}
	_ = s.ledger.AppendEvent(loop.SessionID, "loop_steer_canceled", map[string]any{
		"loop_id": loopID, "steer_id": steerID,
	})
	return nil
}

// applySteers is called at a step boundary. Each pending row is claimed with a
// conditional update, so two concurrent steps can never apply one steer twice.
func (s *Supervisor) applySteers(loopID, sessionID string) {
	rows, err := s.ledger.DB().Query(
		`SELECT id, text FROM loop_steers WHERE loop_id = ? AND status = 'pending' ORDER BY created_at, rowid`, loopID)
	if err != nil {
		return
	}
	type pending struct{ id, text string }
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.text); err == nil {
			items = append(items, item)
		}
	}
	rows.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range items {
		res, err := s.ledger.DB().Exec(
			`UPDATE loop_steers SET status = 'applied', applied_at = ? WHERE id = ? AND status = 'pending'`, now, item.id)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n != 1 {
			continue // another step claimed it
		}
		_ = s.ledger.AppendEvent(sessionID, "loop_steered", map[string]any{
			"loop_id": loopID, "steer_id": item.id, "text": item.text,
		})
		_, _ = s.RecordPart(sessionID, LoopPart{
			LoopID: loopID, Helper: "lead", Title: "Steering applied",
			Goal: item.text, State: "done",
		})
	}
}
