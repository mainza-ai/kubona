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
