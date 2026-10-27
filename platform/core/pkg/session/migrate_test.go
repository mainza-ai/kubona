package session

// S5/I4 — DeleteSession fail-closed cascade. Threat: deleting live work
// or money trails silently (G6: previously orphaned 5 of 8 tables).

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func mustOpenCascade(t *testing.T) (*Manager, *ledger.Ledger) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/cascade.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return NewManager(l), l
}

func seedCascadeSession(t *testing.T, m *Manager, l *ledger.Ledger, sid string) {
	t.Helper()
	s := &Session{ID: sid, Title: "cascade", TenantID: "default", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Status: "active", SpendLimitUSD: 100}
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		s.ID, s.Title, s.TenantID, s.CreatedAt, s.UpdatedAt, s.Status, s.SpendLimitUSD); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := m.AddMessage(sid, "user", "hello", nil); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	if err := l.AppendEvent(sid, "chat_message_appended", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if err := l.SaveLoopState(ledger.LoopPersist{ID: "loop-" + sid, SessionID: sid, HarnessID: "ansatz-search", Status: "running", MaxSteps: 5, LastActivity: time.Now().UTC()}); err != nil {
		t.Fatalf("seed loop: %v", err)
	}
}

func TestDeleteSessionRefusesLiveWork(t *testing.T) {
	m, l := mustOpenCascade(t)
	seedCascadeSession(t, m, l, "sess-live")
	if err := m.DeleteSession("sess-live", testDeleteSigner()); err == nil {
		t.Fatalf("delete with a running loop must refuse")
	} else if !strings.Contains(strings.ToLower(err.Error()), "live loops") {
		t.Fatalf("refusal must name live loops, got %v", err)
	}
	// Non-terminal queued job with a reservation blocks too.
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount) VALUES ('j1', 'sess-live', 'JOB_STATUS_QUEUED', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 5.0)`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := m.DeleteSession("sess-live", testDeleteSigner()); err == nil {
		t.Fatalf("delete with reserved job must refuse")
	} else if !strings.Contains(strings.ToLower(err.Error()), "non-terminal jobs") {
		t.Fatalf("refusal must name non-terminal jobs, got %v", err)
	}
}

// W0.1 — terminal jobs (COMPLETED/FAILED) must not block deletion.
// Threat (verified): the old literal list never matched JOB_STATUS_FAILED,
// so any session with a failed job could never be deleted.
func TestDeleteBlockersTerminalJobsDoNotBlock(t *testing.T) {
	m, l := mustOpenCascade(t)
	seedCascadeSession(t, m, l, "sess-term")
	if _, err := l.DB().Exec(`UPDATE loops SET status = 'aborted' WHERE session_id = 'sess-term'`); err != nil {
		t.Fatalf("settle loop: %v", err)
	}
	for _, tc := range []struct{ id, status string }{
		{"j-failed", "JOB_STATUS_FAILED"},
		{"j-done", "JOB_STATUS_COMPLETED"},
	} {
		if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount) VALUES (?, 'sess-term', ?, 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 0)`, tc.id, tc.status); err != nil {
			t.Fatalf("seed job %s: %v", tc.id, err)
		}
	}
	if bl, err := m.DeleteBlockers("sess-term"); err != nil {
		t.Fatalf("blockers: %v", err)
	} else if len(bl) != 0 {
		t.Fatalf("terminal jobs must not block, got %v", bl)
	}
	if err := m.DeleteSession("sess-term", testDeleteSigner()); err != nil {
		t.Fatalf("settled delete must succeed: %v", err)
	}
	// A running job still blocks (fail-closed direction preserved).
	m2, l2 := mustOpenCascade(t)
	seedCascadeSession(t, m2, l2, "sess-live2")
	if _, err := l2.DB().Exec(`UPDATE loops SET status = 'aborted' WHERE session_id = 'sess-live2'`); err != nil {
		t.Fatalf("settle loop: %v", err)
	}
	if _, err := l2.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount) VALUES ('j-run', 'sess-live2', 'JOB_STATUS_RUNNING', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 0)`); err != nil {
		t.Fatalf("seed running job: %v", err)
	}
	if bl, err := m2.DeleteBlockers("sess-live2"); err != nil {
		t.Fatalf("blockers: %v", err)
	} else if len(bl) != 1 || bl[0] != "non-terminal jobs" {
		t.Fatalf("running job must block exactly once, got %v", bl)
	}
}

func TestDeleteSessionCascadesAll(t *testing.T) {
	m, l := mustOpenCascade(t)
	seedCascadeSession(t, m, l, "sess-gone")
	// Settle: abort the loop, complete the books (no jobs seeded here).
	if _, err := l.DB().Exec(`UPDATE loops SET status = 'aborted' WHERE session_id = 'sess-gone'`); err != nil {
		t.Fatalf("settle loop: %v", err)
	}
	if err := m.DeleteSession("sess-gone", testDeleteSigner()); err != nil {
		t.Fatalf("settled delete must succeed: %v", err)
	}
	tables := map[string]string{
		"chat_messages": "session_id", "campaign_states": "session_id",
		"campaign_versions": "session_id", "claim_records": "session_id",
		"canvas_layouts": "session_id", "jobs": "session_id", "loops": "session_id",
		"idempotency_keys": "session_id",
		"sessions":         "id",
	}
	for table, col := range tables {
		var n int
		if err := l.DB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?`, "sess-gone").Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("table %s must be empty after cascade, got %d rows", table, n)
		}
	}
	// The whole audit chain survives; the last event is the receipt.
	var evs int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE session_id = ?`, "sess-gone").Scan(&evs); err != nil || evs < 2 {
		t.Fatalf("audit chain and deletion receipt must survive: %v %d", err, evs)
	}
	var etype string
	if err := l.DB().QueryRow(`SELECT event_type FROM ledger_events WHERE session_id = ? ORDER BY id DESC LIMIT 1`, "sess-gone").Scan(&etype); err != nil || etype != "session_deleted" {
		t.Fatalf("last event must be session_deleted, got %v %q", err, etype)
	}
	if faults, err := l.VerifyChain("sess-gone"); err != nil || len(faults) != 0 {
		t.Fatalf("surviving audit chain must verify clean: %v %v", err, faults)
	}
	if err := m.DeleteSession("sess-ghost", testDeleteSigner()); err == nil {
		t.Fatalf("ghost delete must refuse")
	}
}

// R1 — section envelopes: export seals every section as a verifiable
// evidence envelope; import verifies the set; tamper/swap/partial refuse;
// manifests predating envelopes still import (digests verify as before).
func TestSectionEnvelopesRoundTrip(t *testing.T) {
	m1, _ := mustOpenCascade(t)
	seedCascadeSession(t, m1, m1.ledger, "env-sess")
	man, err := m1.ExportSession("env-sess")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(man.SectionEnvelopes) != 10 {
		t.Fatalf("want 10 section envelopes, got %d", len(man.SectionEnvelopes))
	}
	for name, env := range man.SectionEnvelopes {
		e := env
		if err := ledger.VerifyEnvelope(&e); err != nil {
			t.Fatalf("section %s envelope must verify: %v", name, err)
		}
		if e.NormalizedResult["section"] != name {
			t.Fatalf("section %s envelope binds %v", name, e.NormalizedResult["section"])
		}
		if e.TenantID != "default" {
			t.Fatalf("envelope tenant = %q, want session tenant", e.TenantID)
		}
	}
	m2, _ := mustOpenCascade(t)
	res, err := m2.ImportSession(man)
	if err != nil {
		t.Fatalf("import with envelopes: %v", err)
	}
	if res.SessionID != "env-sess" || res.Existing {
		t.Fatalf("fresh import must replay, got %+v", res)
	}
}

func TestSectionEnvelopesTamperRefused(t *testing.T) {
	m1, _ := mustOpenCascade(t)
	seedCascadeSession(t, m1, m1.ledger, "env-tamper")
	man, err := m1.ExportSession("env-tamper")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	m2, _ := mustOpenCascade(t)

	// Forged evidence_id on one section.
	forged := *man
	forged.SectionEnvelopes = map[string]ledger.Envelope{}
	for name, env := range man.SectionEnvelopes {
		forged.SectionEnvelopes[name] = env
	}
	messages := forged.SectionEnvelopes["messages"]
	messages.EvidenceID = "ev_" + strings.Repeat("0", 64)
	forged.SectionEnvelopes["messages"] = messages
	if _, err := m2.ImportSession(&forged); err == nil {
		t.Fatal("forged envelope id must refuse")
	}

	// Swapped envelopes between sections.
	swapped := *man
	swapped.SectionEnvelopes = map[string]ledger.Envelope{}
	for name, env := range man.SectionEnvelopes {
		swapped.SectionEnvelopes[name] = env
	}
	swapped.SectionEnvelopes["messages"], swapped.SectionEnvelopes["jobs"] =
		swapped.SectionEnvelopes["jobs"], swapped.SectionEnvelopes["messages"]
	if _, err := m2.ImportSession(&swapped); err == nil {
		t.Fatal("swapped section envelopes must refuse")
	}

	// Partial set (one envelope dropped).
	partial := *man
	partial.SectionEnvelopes = map[string]ledger.Envelope{}
	for name, env := range man.SectionEnvelopes {
		partial.SectionEnvelopes[name] = env
	}
	delete(partial.SectionEnvelopes, "claims")
	if _, err := m2.ImportSession(&partial); err == nil {
		t.Fatal("partial envelope set must refuse")
	}
}

func TestSectionEnvelopesAbsentStillImports(t *testing.T) {
	m1, _ := mustOpenCascade(t)
	seedCascadeSession(t, m1, m1.ledger, "env-legacy")
	man, err := m1.ExportSession("env-legacy")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	man.SectionEnvelopes = nil // pre-envelope manifest shape
	m2, _ := mustOpenCascade(t)
	if _, err := m2.ImportSession(man); err != nil {
		t.Fatalf("envelope-less manifest must still import: %v", err)
	}
}

// T-10a — ReconcileLoopsOnBoot must skip every terminal loop status in
// BOTH writer vocabularies (autoresearch + supervisor) and must never
// resurrect killed rows to sleeping. Parity guard: the skip set in
// ledger.ReconcileLoopsOnBoot must equal TerminalLoopStatuses (+ the
// already-settled sleeping). Threat: a killed loop flipped to sleeping
// pins its session forever (sleeping is non-terminal by design, wake
// has no engine post-restart, delete refuses "live loops").
func TestReconcileSkipsAllTerminalLoopStatuses(t *testing.T) {
	_, l := mustOpenCascade(t)
	seed := func(id, status string) {
		t.Helper()
		if err := l.SaveLoopState(ledger.LoopPersist{ID: id, SessionID: "sess-rec", HarnessID: "ansatz-search", Status: status, MaxSteps: 5, LastActivity: time.Now().UTC()}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	for _, st := range TerminalLoopStatuses {
		seed("loop-term-"+st, st)
	}
	seed("loop-sleeping", "sleeping")
	seed("loop-running", "running")
	n, err := l.ReconcileLoopsOnBoot()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if n != 1 {
		t.Fatalf("exactly the running row must flip, got %d", n)
	}
	rows, err := l.ListLoopStates()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]string{}
	for _, r := range rows {
		byID[r.ID] = r.Status
	}
	for _, st := range TerminalLoopStatuses {
		if got := byID["loop-term-"+st]; got != st {
			t.Fatalf("terminal status %q was rewritten to %q", st, got)
		}
	}
	if got := byID["loop-sleeping"]; got != "sleeping" {
		t.Fatalf("sleeping row must stay sleeping, got %q", got)
	}
	if got := byID["loop-running"]; got != "sleeping" {
		t.Fatalf("running row must flip to sleeping, got %q", got)
	}
}

func testDeleteSigner() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	return ed25519.NewKeyFromSeed(seed)
}
