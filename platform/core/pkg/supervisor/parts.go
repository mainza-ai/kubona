package supervisor

// C1 — recorded run parts. A loop does not get lanes from guesswork: every
// part here was emitted by the rail at the moment it did the work (a supervisor
// step, a job dispatch, a lifecycle transition). Parts are ledger events, so
// they carry the same audit guarantees as the rest of the run.

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// Closed helper vocabulary. Adding a helper means adding it here and in the
// console run-column order. Literature/prover and copilot tool calls have no
// loop identity today (chat turns are not loops); they are intentionally not
// fabricated into parts until a real run rail owns them.
var LoopPartHelpers = map[string]bool{
	"lead":        true,
	"tool":        true,
	"quantum_job": true,
	"literature":  true,
	"prover":      true,
}

// Closed part-state vocabulary (Zephium's run-shape states).
var LoopPartStates = map[string]bool{
	"planned": true,
	"running": true,
	"waiting": true,
	"done":    true,
	"failed":  true,
	"stopped": true,
}

type LoopPart struct {
	ID      string   `json:"part_id"`
	LoopID  string   `json:"loop_id"`
	Helper  string   `json:"helper"`
	Title   string   `json:"title"`
	Goal    string   `json:"goal,omitempty"`
	State   string   `json:"state"`
	Now     string   `json:"now,omitempty"`
	BeganMS int64    `json:"began_ms"`
	EndedMS int64    `json:"ended_ms,omitempty"`
	After   string   `json:"after,omitempty"`
	JobID   string   `json:"job_id,omitempty"`
	Cited   []string `json:"cited,omitempty"`
}

// RecordPart appends one part for a live loop. Unknown loops, helpers, and
// states refuse: a lane never contains an item the vocabulary cannot name.
func (s *Supervisor) RecordPart(sessionID string, part LoopPart) (*LoopPart, error) {
	if part.LoopID == "" {
		return nil, fmt.Errorf("loop part needs a loop id")
	}
	if part.Title == "" {
		return nil, fmt.Errorf("loop part needs a title")
	}
	if !LoopPartHelpers[part.Helper] {
		return nil, fmt.Errorf("unknown loop part helper %q", part.Helper)
	}
	if !LoopPartStates[part.State] {
		return nil, fmt.Errorf("unknown loop part state %q", part.State)
	}
	s.mu.RLock()
	loop, ok := s.loops[part.LoopID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("loop not found: %s", part.LoopID)
	}
	if sessionID == "" {
		sessionID = loop.SessionID
	}
	now := time.Now().UTC()
	if part.ID == "" {
		part.ID = uuid.New().String()
	}
	if part.BeganMS == 0 {
		part.BeganMS = now.UnixMilli()
	}
	if part.EndedMS == 0 && (part.State == "done" || part.State == "failed" || part.State == "stopped") {
		part.EndedMS = now.UnixMilli()
	}
	if s.ledger == nil {
		return nil, fmt.Errorf("loop part needs a ledger")
	}
	if err := s.ledger.AppendEvent(sessionID, "loop_part", part); err != nil {
		return nil, err
	}
	return &part, nil
}

// recentPartLimit bounds one parts read; parts are ledger events, so an
// unbounded pull would cost a large scan for a long-running loop.
const recentPartLimit = 1000

// LoopParts reads recorded parts for one loop from the session ledger,
// oldest first. The API layer enriches job parts with the job's live state.
func (s *Supervisor) LoopParts(loopID string) ([]LoopPart, error) {
	s.mu.RLock()
	loop, ok := s.loops[loopID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("loop not found: %s", loopID)
	}
	events, err := s.ledger.GetRecentEvents(loop.SessionID, recentPartLimit)
	if err != nil {
		return nil, err
	}
	type recorded struct {
		part LoopPart
		seq  int64
	}
	items := make([]recorded, 0, 16)
	for _, event := range events {
		if event.EventType != "loop_part" {
			continue
		}
		var part LoopPart
		if err := unmarshalEvent(event, &part); err != nil {
			continue
		}
		if part.LoopID != loopID {
			continue
		}
		items = append(items, recorded{part: part, seq: event.ID})
	}
	// BeganMS keeps the run narrative even when an event was delayed; the
	// ledger sequence breaks same-millisecond ties deterministically (two
	// transitions in one step must not swap order between reads).
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].part.BeganMS != items[j].part.BeganMS {
			return items[i].part.BeganMS < items[j].part.BeganMS
		}
		return items[i].seq < items[j].seq
	})
	out := make([]LoopPart, 0, len(items))
	for _, item := range items {
		out = append(out, item.part)
	}
	return out, nil
}

func unmarshalEvent(event ledger.EventRecord, dst any) error {
	return json.Unmarshal(event.Payload, dst)
}
