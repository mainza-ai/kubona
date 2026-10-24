package agents

// R4 tests: stable paging, lossless continuation, pinned-budget refusal,
// bounded soak, honest breach degradation, bundle resume with ancestry.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func TestEvidencePageStable(t *testing.T) {
	s := NewEvidenceStore("test")
	it := s.Put("turn", "t1", "abcdefghijklmnopqrstuvwxyz", "")
	if it.ID != "evd-test-0" {
		t.Fatalf("stable id, got %q", it.ID)
	}
	// Windows.
	page, next, total, err := s.Page(it.ID, 0, 10)
	if err != nil || page != "abcdefghij" || next != 10 || total != 26 {
		t.Fatalf("page: %q %d %d %v", page, next, total, err)
	}
	page, next, _, err = s.Page(it.ID, 10, 100)
	if err != nil || page != "klmnopqrstuvwxyz" || next != -1 {
		t.Fatalf("clamped tail: %q %d %v", page, next, err)
	}
	page, next, _, err = s.Page(it.ID, 26, 10)
	if err != nil || page != "" || next != -1 {
		t.Fatalf("offset at end: %q %d %v", page, next, err)
	}
	// Refusals.
	for _, tc := range [][2]int{{-1, 10}, {0, 0}, {0, -5}} {
		if _, _, _, err := s.Page(it.ID, tc[0], tc[1]); err == nil {
			t.Fatalf("offset=%d limit=%d must refuse", tc[0], tc[1])
		}
	}
	if _, _, _, err := s.Page("evd-nope-0", 0, 10); err == nil {
		t.Fatal("unknown evidence must refuse")
	}
	// Growth never shifts earlier pages (append-only stability).
	s.Put("turn", "t2", strings.Repeat("z", 5000), "")
	page2, _, _, err := s.Page(it.ID, 0, 10)
	if err != nil || page2 != "abcdefghij" {
		t.Fatalf("pages must be stable across growth: %q %v", page2, err)
	}
	// Parent linkage round-trips.
	child := s.Put("turn", "t3", "x", it.ID)
	got, ok := s.Get(child.ID)
	if !ok || got.ParentID != it.ID {
		t.Fatalf("parent link must survive: %+v", got)
	}
	if _, ok := s.Get("evd-nope-9"); ok {
		t.Fatal("unknown get must miss")
	}
}

func TestSummarizeBoundsViews(t *testing.T) {
	short := EvidenceItem{ID: "evd-x-0", Kind: "turn", Label: "t1", Content: "small"}
	if got := Summarize(short, 1500); !strings.Contains(got, "small") || strings.Contains(got, "evidence_continue") {
		t.Fatalf("short content passes through unmarked: %q", got)
	}
	long := EvidenceItem{ID: "evd-x-1", Kind: "observation", Label: "tool", Content: strings.Repeat("d", 5000)}
	got := Summarize(long, 1500)
	if len([]rune(got)) > 2000 || !strings.Contains(got, "evidence_continue evd-x-1") {
		t.Fatalf("long content must bound + ref: len=%d %q", len([]rune(got)), got[:120])
	}
	parented := EvidenceItem{ID: "evd-x-2", Kind: "turn", Label: "t", Content: "c", ParentID: "evd-old-3"}
	if !strings.Contains(Summarize(parented, 1500), "parent=evd-old-3") {
		t.Fatal("parent link must ride the summary")
	}
}

func TestProjectTurnContextBudget(t *testing.T) {
	ok, err := ProjectTurnContext("t", []string{"a", "", "b"}, 16000)
	if err != nil || ok != "a\n\nb" {
		t.Fatalf("empties skipped, joined: %q %v", ok, err)
	}
	big := strings.Repeat("x", 20000)
	_, err = ProjectTurnContext("turn-9 probe", []string{big}, 16000)
	rce, isRCE := err.(*ResourceConstraintError)
	if !isRCE {
		t.Fatalf("breach must return *ResourceConstraintError, got %T %v", err, err)
	}
	if rce.Code() != "resource_constraint" || rce.Have != 20000 || rce.Budget != 16000 {
		t.Fatalf("refusal must name sizes + code: %+v", rce)
	}
}

func TestContinuationRoundTripsLosslessly(t *testing.T) {
	s := NewEvidenceStore("cont")
	full := strings.Repeat("0123456789abcdef", 2000) // 32k runes
	it := s.Put("observation", "big_tool", full, "")
	// Resolve exactly like a role would, through the shared definition.
	reg := tools.NewRegistry()
	if err := reg.Register(EvidenceContinueDefinition()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Allow("ansatz-scout", "evidence_continue"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	ctx := ContextWithEvidenceStore(context.Background(), s)
	var assembled strings.Builder
	offset := 0
	for {
		out, err := reg.Dispatch(ctx, "ansatz-scout", tools.Call{
			Name:      "evidence_continue",
			Arguments: `{"evidence_id": "` + it.ID + `", "offset": ` + itoa(offset) + `, "limit": 5000}`,
		})
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		m := out.(map[string]any)
		assembled.WriteString(m["content"].(string))
		next := m["next_offset"]
		// End of evidence is next == -1 (explicit, JSON-safe — never a
		// null that callers must distinguish from zero). In-process
		// dispatch keeps ints; over HTTP/MCP the same field arrives
		// as float64. Accept both.
		done := false
		switch n := next.(type) {
		case int:
			if n == -1 {
				done = true
			} else {
				offset = n
			}
		case float64:
			if n == -1 {
				done = true
			} else {
				offset = int(n)
			}
		default:
			t.Fatalf("next_offset has wrong type %T", next)
		}
		if done {
			break
		}
	}
	if assembled.String() != full {
		t.Fatal("paged continuation must reassemble losslessly")
	}
	// Unknown id + storeless context refuse (never hallucinate).
	if _, err := reg.Dispatch(ctx, "ansatz-scout", tools.Call{Name: "evidence_continue", Arguments: `{"evidence_id":"evd-nope-0"}`}); err == nil {
		t.Fatal("unknown evidence must refuse")
	}
	if _, err := reg.Dispatch(context.Background(), "ansatz-scout", tools.Call{Name: "evidence_continue", Arguments: `{"evidence_id":"` + it.ID + `"}`}); err == nil {
		t.Fatal("storeless context must refuse")
	}
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

// sizeRecordingStub serves big replies while recording every prompt's
// byte size (the soak assertion reads the max).
func sizeRecordingStub(t *testing.T, replyRunes int, maxSeen *int, mu *sync.Mutex) *llm.Client {
	t.Helper()
	big := strings.Repeat("e", replyRunes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		n := 0
		if msgs, ok := body["messages"].([]any); ok {
			for _, m := range msgs {
				if mm, ok := m.(map[string]any); ok {
					if c, ok := mm["content"].(string); ok {
						n += len([]rune(c))
					}
				}
			}
		}
		mu.Lock()
		if n > *maxSeen {
			*maxSeen = n
		}
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": big},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OPENCODE_API_KEY", "test-key")
	settings := llm.DefaultSettings()
	snap := settings.GetSnapshot()
	// Evidence tests drive the cloud stub: opt out of the air-gap
	// default like an operator would.
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderOpenCode] = pc
	for _, role := range []string{"ansatz-scout", "disconfirmation-officer", "device-auditor", "epistemic-scribe"} {
		snap.RoleAssignments[role] = llm.RoleModelAssignment{Role: role, Provider: llm.ProviderOpenCode, Model: "stub"}
	}
	if err := settings.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	return llm.NewClient(settings)
}

func TestTurnPromptsBoundedSoak(t *testing.T) {
	budget := llm.DefaultBudgetConfig().TurnContextRunes
	var maxSeen int
	var mu sync.Mutex
	cli := sizeRecordingStub(t, 50000, &maxSeen, &mu)
	ac := NewAgentCollective()
	// 3 full runs with 50k-rune replies: every constructed prompt must
	// fit the pinned budget (max, not p95 — stronger than the accept).
	for i := 0; i < 3; i++ {
		if _, err := ac.ExecuteTribunal(context.Background(), cli, nil, "soak run"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if maxSeen == 0 {
		t.Fatal("stub saw no prompts")
	}
	if maxSeen > budget {
		t.Fatalf("p95/max turn context %d exceeds pinned budget %d", maxSeen, budget)
	}
	t.Logf("soak max turn prompt: %d runes vs %d budget", maxSeen, budget)
}

func mustOpenR4Ledger(t *testing.T) (*ledger.Ledger, string) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/r4.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	sid := "sess-r4"
	now := time.Now().UTC()
	if _, err := l.DB().Exec(`INSERT INTO sessions
		(id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd)
		VALUES (?, 't', 'default', ?, ?, 'active', 100, 0)`, sid, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return l, sid
}

func TestBudgetBreachDegradesHonestly(t *testing.T) {
	l, sid := mustOpenR4Ledger(t)
	ac := NewAgentCollective()
	ac.Ledger = l
	// 20k-rune prompt breaches the 16k budget on turn 1: the run must
	// complete on fallback (never a priced runaway, never an error),
	// with the breach recorded as a closure-blocking unknown.
	res, err := ac.ExecuteTribunalWithSession(context.Background(), nil, nil, nil, sid, strings.Repeat("q", 20000))
	if err != nil {
		t.Fatalf("breach must degrade, not fail: %v", err)
	}
	if len(res.Turns) != 4 {
		t.Fatalf("breached run still completes 4 turns, got %d", len(res.Turns))
	}
	ob, _ := l.GetObligation(res.ObligationID)
	found := false
	for _, u := range ob.Unknowns {
		if u.ID == "context-budget-turn-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("breach must record a context-budget unknown: %+v", ob.Unknowns)
	}
	_ = l.SetObligationOwner(res.ObligationID, "session:"+sid)
	if closed, _, _ := AttemptClose(l, res.ObligationID); closed {
		t.Fatal("budget-breach run must stay open")
	}
}

func TestBundleResumeFlow(t *testing.T) {
	l, sid := mustOpenR4Ledger(t)
	ac := NewAgentCollective()
	ac.Ledger = l
	res, err := ac.ExecuteTribunalWithSession(context.Background(), nil, nil, nil, sid, "Investigate H2 ground state")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := MarshalTribunalBundle(res)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	b, err := ParseTribunalBundle(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b.Version != 1 || len(b.Turns) != 4 || b.ObligationID != res.ObligationID {
		t.Fatalf("bundle must round-trip the run: %+v", b)
	}
	if _, err := ParseTribunalBundle([]byte(`{"version": 99}`)); err == nil {
		t.Fatal("unknown bundle major must refuse")
	}
	if _, err := ParseTribunalBundle([]byte(`{"version": 1}`)); err == nil {
		t.Fatal("turnless bundle must refuse")
	}
	if _, err := ParseTribunalBundle([]byte(`{nope`)); err == nil {
		t.Fatal("corrupt bundle must refuse")
	}

	// Resume with a recording stub: the ancestor obligation id must reach
	// turn-1's prompt (ancestry is observable end-to-end, not just stored).
	var mu sync.Mutex
	var sawAncestor bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if msgs, ok := body["messages"].([]any); ok {
			for _, m := range msgs {
				if mm, ok := m.(map[string]any); ok {
					if c, ok := mm["content"].(string); ok && strings.Contains(c, res.ObligationID) {
						mu.Lock()
						sawAncestor = true
						mu.Unlock()
					}
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "authored continuation with sufficient length for the authored flag"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OPENCODE_API_KEY", "test-key")
	settings := llm.DefaultSettings()
	snap := settings.GetSnapshot()
	// Evidence tests drive the cloud stub: opt out of the air-gap
	// default like an operator would.
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderOpenCode] = pc
	for _, role := range []string{"ansatz-scout", "disconfirmation-officer", "device-auditor", "epistemic-scribe"} {
		snap.RoleAssignments[role] = llm.RoleModelAssignment{Role: role, Provider: llm.ProviderOpenCode, Model: "stub"}
	}
	if err := settings.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	res2, err := ac.ExecuteTribunalResume(context.Background(), llm.NewClient(settings), nil, nil, sid, "continue the debate", raw)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(res2.Turns) != 4 {
		t.Fatalf("resumed run completes 4 turns, got %d", len(res2.Turns))
	}
	if res2.ObligationID == "" || res2.ObligationID == res.ObligationID {
		t.Fatal("resumed run opens a FRESH obligation (closed history never rewritten)")
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawAncestor {
		t.Fatal("ancestor obligation id must reach the resumed run's prompts")
	}
}
