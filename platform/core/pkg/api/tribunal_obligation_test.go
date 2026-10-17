package api_test

// R2/C3 HTTP tests: a tribunal run over HTTP seals exactly its 4 turns
// (no doubles from the removed handler-side sealing), carries the run
// obligation on every turn event, and a second run appends 4 more under
// a distinct obligation. The verdict comes from the obligation query.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func postTribunal(t *testing.T, h http.Handler, sid, prompt string) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"session_id": sid, "prompt": prompt})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tribunal", bytes.NewReader(raw)).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("tribunal: %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("tribunal decode: %v", err)
	}
	return out
}

func turnEvents(t *testing.T, h http.Handler, sid string) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/events?session_id="+sid+"&limit=200", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("events: %d", w.Code)
	}
	var out struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("events decode: %v", err)
	}
	var turns []map[string]any
	for _, e := range out.Events {
		if e["event_type"] == "tribunal_turn" {
			turns = append(turns, e)
		}
	}
	return turns
}

func TestTribunalHTTPSealsExactlyFour(t *testing.T) {
	srv, _, close := setupIdemServer(t)
	defer close()
	h := srv.Handler()
	sid := idemSession(t, h, "obligation http")

	out := postTribunal(t, h, sid, "Investigate H2 ground state")
	trib, _ := out["tribunal"].(map[string]any)
	if trib == nil {
		t.Fatalf("no tribunal in response: %v", out)
	}
	if turns, _ := trib["turns"].([]any); len(turns) != 4 {
		t.Fatalf("want 4 turns, got %v", len(turns))
	}
	obID, _ := out["obligation_id"].(string)
	if obID == "" {
		t.Fatal("response must carry the run obligation id")
	}
	// Fallback run (no LLM in tests): verdict stays open/false.
	if trib["consensus_passed"] == true {
		t.Fatal("fallback tribunal must not claim consensus")
	}

	events := turnEvents(t, h, sid)
	if len(events) != 4 {
		t.Fatalf("HTTP run must seal exactly 4 turn events, got %d", len(events))
	}
	for _, e := range events {
		var p struct {
			TurnIndex    int    `json:"turn_index"`
			ObligationID string `json:"obligation_id"`
		}
		raw, _ := json.Marshal(e["payload"])
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("event payload: %v", err)
		}
		if p.ObligationID != obID {
			t.Fatalf("turn event carries %q, want run obligation %q", p.ObligationID, obID)
		}
	}

	// Second run: 4 more events under a DISTINCT obligation (runs never
	// share obligations; closed history is never rewritten).
	out2 := postTribunal(t, h, sid, "Investigate H2 ground state")
	obID2, _ := out2["obligation_id"].(string)
	if obID2 == "" || obID2 == obID {
		t.Fatalf("second run needs a distinct obligation, got %q vs %q", obID2, obID)
	}
	events = turnEvents(t, h, sid)
	if len(events) != 8 {
		t.Fatalf("two runs must seal 8 turn events, got %d", len(events))
	}
	byOb := map[string]int{}
	for _, e := range events {
		var p struct {
			ObligationID string `json:"obligation_id"`
		}
		raw, _ := json.Marshal(e["payload"])
		_ = json.Unmarshal(raw, &p)
		byOb[p.ObligationID]++
	}
	if len(byOb) != 2 || byOb[obID] != 4 || byOb[obID2] != 4 {
		t.Fatalf("turns must split 4+4 across obligations: %v", byOb)
	}
}
