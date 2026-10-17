package agents_test

// C3 tests: sealing lives in the collective. A direct (non-HTTP) run with
// a ledger seals exactly the run's 4 turns with the obligation attached;
// re-sealing a turn is a no-op receipt; nil-ledger runs are untouched
// (covered by the existing TestExecuteTribunal* suite).

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func mustOpenTribunalLedger(t *testing.T) (*ledger.Ledger, string) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/tribunal.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	sid := "sess-tribunal-c3"
	now := time.Now().UTC()
	if _, err := l.DB().Exec(`INSERT INTO sessions
		(id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd)
		VALUES (?, 't', 'default', ?, ?, 'active', 100, 0)`, sid, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return l, sid
}

func tribunalTurnEvents(t *testing.T, l *ledger.Ledger, sid string) []ledger.EventRecord {
	t.Helper()
	events, err := l.GetRecentEvents(sid, 200)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var turns []ledger.EventRecord
	for _, e := range events {
		if e.EventType == "tribunal_turn" {
			turns = append(turns, e)
		}
	}
	return turns
}

func TestDirectCollectiveRunSealsTurns(t *testing.T) {
	l, sid := mustOpenTribunalLedger(t)
	ac := agents.NewAgentCollective()
	ac.Ledger = l
	res, err := ac.ExecuteTribunalWithSession(t.Context(), nil, nil, nil, sid, "Investigate H2 ground state")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ObligationID == "" {
		t.Fatal("wired run must carry an obligation id")
	}
	turns := tribunalTurnEvents(t, l, sid)
	if len(turns) != 4 {
		t.Fatalf("direct run must seal exactly 4 turns, got %d", len(turns))
	}
	seen := map[int]bool{}
	for _, e := range turns {
		var p struct {
			TurnIndex    int    `json:"turn_index"`
			ObligationID string `json:"obligation_id"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("event payload: %v", err)
		}
		if p.ObligationID != res.ObligationID {
			t.Fatalf("turn event must carry the run obligation, got %q", p.ObligationID)
		}
		seen[p.TurnIndex] = true
	}
	for i := 1; i <= 4; i++ {
		if !seen[i] {
			t.Fatalf("turn %d not sealed", i)
		}
	}
	// Cases recorded (fallback turns fail honestly — no LLM authored them).
	cases, err := l.ListObligationCases(res.ObligationID)
	if err != nil || len(cases) != 4 {
		t.Fatalf("want 4 turn cases, got %+v %v", cases, err)
	}
	// Fallback run stays open: ownerless + failed cases + anchor unknown.
	ob, _ := l.GetObligation(res.ObligationID)
	if ob.Disposition != "open" || len(ob.Unknowns) == 0 {
		t.Fatalf("fallback run must stay open with the anchor unknown: %+v", ob)
	}
	// Re-sealing any turn is a no-op receipt (dedupe by turn index).
	if err := ac.SealTurn(sid, res.ObligationID, res.Turns[0], false); err != nil {
		t.Fatalf("reseal: %v", err)
	}
	if n := len(tribunalTurnEvents(t, l, sid)); n != 4 {
		t.Fatalf("reseal must not double-seal, got %d events", n)
	}
	// Owner closes nothing here (fallback fails) — but the close path is
	// live: reasons name the failures, never empty prose.
	_ = l.SetObligationOwner(res.ObligationID, "session:"+sid)
	closed, reasons, err := agents.AttemptClose(l, res.ObligationID)
	if err != nil || closed || len(reasons) == 0 {
		t.Fatalf("fallback obligation must refuse close with reasons: %v %v %v", closed, reasons, err)
	}
}
