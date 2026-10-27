package api_test

// T-10b: settle-and-archive for restart-orphan sleeping loops. Pins are
// HTTP-level through the real mux, DB, and spend books: orphans settle
// terminal with books zeroed and the session deletable; live rows refuse
// the whole call; ghosts 404; foreign labs 403.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupSettleDrill(t *testing.T) (*api.Server, *ledger.Ledger, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	l, err := ledger.OpenLedger(t.TempDir() + "/settle.db")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	return api.NewServer(l, sm, jr, sup, wd), l, func() { l.Close() }
}

func settlePost(t *testing.T, srv *api.Server, sessionID, cookie string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/loops/settle", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestLoopSettleOrphansTerminal(t *testing.T) {
	srv, l, cleanup := setupSettleDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	cookieA := drillLogin(t, srv, "lab-a")
	sess := drillSession(t, srv, "settle-me", "lab-a", "test-token")

	// Real holds on both rails (mirrors ReserveSpend's effect).
	if _, err := l.DB().Exec(`UPDATE tenants SET spent_usd = 5 WHERE id = 'lab-a'`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`UPDATE sessions SET current_spend_usd = 5 WHERE id = ?`, sess); err != nil {
		t.Fatal(err)
	}
	seed := func(id, status string, reserved float64) {
		t.Helper()
		if err := l.SaveLoopState(ledger.LoopPersist{
			ID: id, SessionID: sess, HarnessID: "ansatz-search",
			Status: status, MaxSteps: 5, LastActivity: time.Now().UTC(),
			ReservedAmount: reserved, ReservedTenant: "lab-a",
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("orphan-held", "sleeping", 5)
	seed("orphan-clean", "sleeping", 0)
	// Already-terminal rows are skipped, not refused and not rewritten.
	seed("already-dead", "killed", 0)

	code, body := settlePost(t, srv, sess, cookieA)
	if code != http.StatusOK {
		t.Fatalf("settle must 200, got %d: %s", code, body)
	}
	if !strings.Contains(body, "orphan-held") || !strings.Contains(body, "orphan-clean") {
		t.Fatalf("settle must name both orphans: %s", body)
	}
	for _, id := range []string{"orphan-held", "orphan-clean"} {
		st, err := l.GetLoopState(id)
		if err != nil || st == nil {
			t.Fatalf("row %s: %v", id, err)
		}
		if st.Status != "aborted" {
			t.Errorf("orphan %s must be terminal aborted, got %q", id, st.Status)
		}
	}
	if st, err := l.GetLoopState("already-dead"); err != nil || st == nil || st.Status != "killed" {
		t.Errorf("terminal killed row must be skipped untouched, got %+v %v", st, err)
	}
	var tenantSpent, sessSpent float64
	if err := l.DB().QueryRow(`SELECT spent_usd FROM tenants WHERE id = 'lab-a'`).Scan(&tenantSpent); err != nil {
		t.Fatal(err)
	}
	if err := l.DB().QueryRow(`SELECT current_spend_usd FROM sessions WHERE id = ?`, sess).Scan(&sessSpent); err != nil {
		t.Fatal(err)
	}
	if tenantSpent != 0 || sessSpent != 0 {
		t.Errorf("both rails must zero out, tenant=%v session=%v", tenantSpent, sessSpent)
	}
	// Ledger carries the terminal receipts (hash-chain contract); the
	// chain now survives DeleteSession (audit retention).
	events, err := l.GetRecentEvents(sess, 50)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.EventType] = true
	}
	for _, want := range []string{"loop_settled", "loop_spend_released"} {
		if !seen[want] {
			t.Errorf("ledger must seal %q", want)
		}
	}
	// Blockers clear: the session that was pinned forever deletes now.
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+sess, nil)
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookieA})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("session must delete after settle, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLoopSettleRefusesLiveLoops(t *testing.T) {
	srv, l, cleanup := setupSettleDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	cookieA := drillLogin(t, srv, "lab-a")
	sess := drillSession(t, srv, "live-one", "lab-a", "test-token")

	if err := l.SaveLoopState(ledger.LoopPersist{
		ID: "live-loop", SessionID: sess, HarnessID: "ansatz-search",
		Status: "running", MaxSteps: 5, LastActivity: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.SaveLoopState(ledger.LoopPersist{
		ID: "quiet-orphan", SessionID: sess, HarnessID: "ansatz-search",
		Status: "sleeping", MaxSteps: 5, LastActivity: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if code, body := settlePost(t, srv, sess, cookieA); code != http.StatusConflict {
		t.Fatalf("settle with a live loop must 409, got %d: %s", code, body)
	}
	// Fail-closed: nothing flipped, not even the quiet orphan.
	for id, want := range map[string]string{"live-loop": "running", "quiet-orphan": "sleeping"} {
		st, err := l.GetLoopState(id)
		if err != nil || st == nil || st.Status != want {
			t.Errorf("row %s must stay %q", id, want)
		}
	}
}

func TestLoopSettleGhostAndTenant(t *testing.T) {
	srv, _, cleanup := setupSettleDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")
	cookieB := drillLogin(t, srv, "lab-b")
	sess := drillSession(t, srv, "foreign-work", "lab-a", "test-token")

	if code, _ := settlePost(t, srv, "ghost", cookieA); code != http.StatusNotFound {
		t.Errorf("ghost settle must 404, got %d", code)
	}
	if code, body := settlePost(t, srv, sess, cookieB); code != http.StatusForbidden {
		t.Errorf("cross-lab settle must 403, got %d: %s", code, body)
	} else if !strings.Contains(body, "another lab") {
		t.Errorf("refusal must name the lab boundary: %s", body)
	}
	var out struct {
		Settled []string `json:"settled"`
	}
	code, body := settlePost(t, srv, sess, cookieA)
	if code != http.StatusOK {
		t.Fatalf("own settle must 200, got %d: %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Settled == nil {
		t.Errorf("empty settle must read as [], not null: %s", body)
	}
}
