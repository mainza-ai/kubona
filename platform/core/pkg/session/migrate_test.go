package session

// S5/I4 — DeleteSession fail-closed cascade. Threat: deleting live work
// or money trails silently (G6: previously orphaned 5 of 8 tables).

import (
	"crypto/ed25519"
	"encoding/json"
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
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		s.ID, s.Title, s.TenantID, s.CreatedAt, s.UpdatedAt, s.Status, 100000000); err != nil {
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
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd) VALUES ('j1', 'sess-live', 'JOB_STATUS_QUEUED', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 5000000)`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := m.DeleteSession("sess-live", testDeleteSigner()); err == nil {
		t.Fatalf("delete with reserved job must refuse")
	} else if !strings.Contains(strings.ToLower(err.Error()), "non-terminal jobs") {
		t.Fatalf("refusal must name non-terminal jobs, got %v", err)
	}
}

func TestDeleteSessionRefusesOutcomeUnknown(t *testing.T) {
	m, l := mustOpenCascade(t)
	seedCascadeSession(t, m, l, "sess-unknown")
	if _, err := l.DB().Exec(`UPDATE loops SET status = 'aborted' WHERE session_id = 'sess-unknown'`); err != nil {
		t.Fatalf("settle loop: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd) VALUES ('j-unk', 'sess-unknown', 'JOB_STATUS_OUTCOME_UNKNOWN', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 1000000)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := m.DeleteSession("sess-unknown", testDeleteSigner())
	if err == nil || !strings.Contains(err.Error(), "outcome-unknown jobs") {
		t.Fatalf("outcome-unknown must block deletion by name, got %v", err)
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
		if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd) VALUES (?, 'sess-term', ?, 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 0)`, tc.id, tc.status); err != nil {
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
	if _, err := l2.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd) VALUES ('j-run', 'sess-live2', 'JOB_STATUS_RUNNING', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 0)`); err != nil {
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
	seedNewSessionTables(t, l, "sess-gone")
	if err := m.DeleteSession("sess-gone", testDeleteSigner()); err != nil {
		t.Fatalf("settled delete must succeed: %v", err)
	}
	tables := map[string]string{
		"chat_messages": "session_id", "campaign_states": "session_id",
		"campaign_versions": "session_id", "claim_records": "session_id",
		"canvas_layouts": "session_id", "jobs": "session_id", "loops": "session_id",
		"idempotency_keys": "session_id", "workbench_views": "session_id",
		"workbench_layouts": "session_id", "tasks": "session_id",
		"workflow_runs": "session_id", "workflow_approvals": "session_id",
		"proposals": "session_id", "claim_reviews": "session_id",
		"loop_steers": "session_id", "run_allowances": "session_id",
		"transcript_scores": "session_id",
		"sessions":          "id",
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

// seedNewSessionTables exercises every session-FK table added after the
// original cascade. Missing one is what made production deletion fail on
// workbench_views; this seed makes that regression loud.
func seedNewSessionTables(t *testing.T, l *ledger.Ledger, sid string) {
	t.Helper()
	now := time.Now().UTC()
	stmts := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workbench_views (session_id, view_revision, view_json, view_digest, updated_at) VALUES (?, 1, '{}', 'digest', ?)`, []any{sid, now}},
		{`INSERT INTO workbench_layouts (session_id, revision, document, updated_at) VALUES (?, 1, '{}', ?)`, []any{sid, now}},
		{`INSERT INTO tasks (id, session_id, title, status, source_domain, source_id, created_at, updated_at) VALUES ('task-'||?, ?, 'task', 'open', 'job', 'j', ?, ?)`, []any{sid, sid, now.Format(time.RFC3339), now.Format(time.RFC3339)}},
		{`INSERT INTO workflow_runs (id, session_id, external_id, status, workflow_json, budget_ceiling_micro_usd, spent_micro_usd, error, created_at, updated_at) VALUES ('wf-'||?, ?, '', 'running', '{}', 0, 0, '', ?, ?)`, []any{sid, sid, now, now}},
		{`INSERT INTO workflow_steps (run_id, step_id, ordinal, tool, status, job_id, result_json, error, updated_at) VALUES ('wf-'||?, 's1', 0, 'tool', 'pending', '', '{}', '', ?)`, []any{sid, now}},
		{`INSERT INTO workflow_approvals (token_hash, session_id, workflow_sha256, approved_by, expires_at, created_at, used_at) VALUES ('tok-'||?, ?, 'sha', 'admin', ?, ?, '')`, []any{sid, sid, now.Add(time.Hour), now}},
		{`INSERT INTO proposals (id, session_id, message_id, digest, body_json, basis_campaign_rev, basis_calibration_digest, basis_tainted, status, launched_job_id, created_at, updated_at) VALUES ('prop-'||?, ?, '', 'd', '{}', 0, '', 0, 'offered', '', ?, ?)`, []any{sid, sid, now, now}},
		{`INSERT INTO claim_records (id, session_id, turn_kind, statement, alpha, beta, gamma, verdict, evidence_json, created_at, request_id) VALUES ('claim-'||?, ?, 'assistant', 'claim', 0.9, 0.1, 0.2, 'supported', '{}', ?, '')`, []any{sid, sid, now}},
		{`INSERT INTO claim_reviews (id, session_id, claim_id, decision, reason, reviewer, evidence_json, created_at) VALUES ('review-'||?, ?, 'claim-'||?, 'supported', 'r', 'admin', '{}', ?)`, []any{sid, sid, sid, now}},
		{`INSERT INTO claim_evidence (claim_id, kind, job_id, result_sha256, receipt_entry_hash, source_ref, created_at) VALUES ('claim-'||?, 'job_result', 'j', 'sha', '', '', ?)`, []any{sid, now}},
		{`INSERT INTO loop_steers (id, loop_id, session_id, text, status, created_at) VALUES ('steer-'||?, 'loop-'||?, ?, 'go', 'pending', ?)`, []any{sid, sid, sid, now}},
		{`INSERT INTO transcript_scores (session_id, obligation_id, role, score, signals_json, recorded_at) VALUES (?, 'ob-'||?, 'device-auditor', 0.8, '{}', ?)`, []any{sid, sid, now.Format(time.RFC3339)}},
		{`INSERT INTO chat_feedback (message_id, session_id, vote, reason, actor, created_at, updated_at) VALUES ('msg-'||?, ?, 'up', 'test', 'local_researcher', ?, ?)`, []any{sid, sid, now.Format(time.RFC3339), now.Format(time.RFC3339)}},
		{`INSERT INTO notification_states (session_id, notification_key, read_at, resolved_at) VALUES (?, 'event:1', '', '')`, []any{sid}},
	}
	for _, stmt := range stmts {
		if _, err := l.DB().Exec(stmt.query, stmt.args...); err != nil {
			t.Fatalf("seed %q: %v", stmt.query[:40], err)
		}
	}
	allowance, err := l.MintRunAllowance(ledger.RunAllowance{
		ID: "allow-" + sid, TenantID: "default", SessionID: sid, ScopeDigest: "scope",
		Backends: []string{"sim"}, JobTypes: []string{"circuit_run"},
		MaxPerJobMicroUSD: 1000, MaxTotalMicroUSD: 2000, MaxJobs: 1, ShotsCap: 10,
		BasisRateCard: "test", Nonce: "n", SignerPubkeyHex: "pk", SignatureHex: "sig",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed allowance: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO run_allowance_draws (allowance_id, job_id, request_id, amount_micro_usd, created_at) VALUES (?, 'j', 'r', 1, ?)`, allowance.ID, now); err != nil {
		t.Fatalf("seed draw: %v", err)
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

func TestManifestKeyRotation(t *testing.T) {
	t.Setenv("KUBONA_IMPORT_HMAC_KEYS", "old=key-old,new=key-new")
	t.Setenv("KUBONA_IMPORT_HMAC_KEY", "")
	t.Setenv("KUBONA_API_TOKEN", "")

	src, _ := mustOpenCascade(t)
	sess, err := src.CreateSession("rotate-me", 100, "default")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("KUBONA_IMPORT_HMAC_KEY_ID", "new")
	man, err := src.ExportSession(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if man.KeyID != "new" || man.Signature == "" {
		t.Fatalf("export must name the active key: kid=%q sig=%q", man.KeyID, man.Signature)
	}
	dst, _ := mustOpenCascade(t)
	if _, err := dst.ImportSession(man); err != nil {
		t.Fatalf("active-key import: %v", err)
	}

	t.Setenv("KUBONA_IMPORT_HMAC_KEY_ID", "old")
	oldMan, err := src.ExportSession(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if oldMan.KeyID != "old" {
		t.Fatalf("expected old key id, got %q", oldMan.KeyID)
	}
	dst2, _ := mustOpenCascade(t)
	if _, err := dst2.ImportSession(oldMan); err != nil {
		t.Fatalf("rotated-out-but-configured key must verify: %v", err)
	}

	// A key id that is not in the keyring refuses even with a valid MAC.
	forged := *oldMan
	forged.KeyID = "gone"
	dst3, _ := mustOpenCascade(t)
	if _, err := dst3.ImportSession(&forged); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unknown key id must refuse, got %v", err)
	}

	t.Setenv("KUBONA_IMPORT_HMAC_KEY_ID", "missing")
	if _, err := src.ExportSession(sess.ID); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing active key id must refuse export, got %v", err)
	}

	// Legacy manifests (no key_id) still verify against any configured key.
	t.Setenv("KUBONA_IMPORT_HMAC_KEY_ID", "")
	legacy := *oldMan
	legacy.KeyID = ""
	dst4, _ := mustOpenCascade(t)
	if _, err := dst4.ImportSession(&legacy); err != nil {
		t.Fatalf("legacy keyless manifest must verify: %v", err)
	}
}

func TestImportRejectsTamperedEventHash(t *testing.T) {
	t.Setenv("KUBONA_ENV", "")
	src, _ := mustOpenCascade(t)
	sess, err := src.CreateSession("tamper", 100, "default")
	if err != nil {
		t.Fatal(err)
	}
	man, err := src.ExportSession(sess.ID)
	if err != nil || len(man.Events) == 0 {
		t.Fatalf("export: %v events=%d", err, len(man.Events))
	}
	man.SectionEnvelopes = nil
	man.Events[0].EntryHash = strings.Repeat("0", 64)
	man.Digests["events"], err = sectionDigest(man.Events)
	if err != nil {
		t.Fatal(err)
	}
	man.Signature = ""
	man.SignatureAlg = ""
	man.KeyID = ""
	dst, _ := mustOpenCascade(t)
	if _, err := dst.ImportSession(man); err == nil || !strings.Contains(err.Error(), "entry_hash mismatch") {
		t.Fatalf("tampered event hash must refuse, got %v", err)
	}
}

func TestImportCanonicalizesLegacyV1Payload(t *testing.T) {
	t.Setenv("KUBONA_ENV", "")
	src, _ := mustOpenCascade(t)
	sess, err := src.CreateSession("v1", 100, "default")
	if err != nil {
		t.Fatal(err)
	}
	man, err := src.ExportSession(sess.ID)
	if err != nil || len(man.Events) != 1 {
		t.Fatalf("export: %v events=%d", err, len(man.Events))
	}
	man.SectionEnvelopes = nil
	e := &man.Events[0]
	// Legacy v1 hash over compact payload, with the manifest carrying
	// pretty-printed bytes (the JSON round-trip case).
	var compact any
	if err := json.Unmarshal(e.Payload, &compact); err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(compact)
	e.Payload = json.RawMessage(prettyJSON(t, compact))
	e.HashVersion = 1
	e.EntryHash = ledger.ChainDigest(e.PrevHash, sess.ID, e.EventType, string(canonical))
	e.SignerPubkey, e.Signature, e.SignatureDomain = "", "", ""
	man.Digests["events"], err = sectionDigest(man.Events)
	if err != nil {
		t.Fatal(err)
	}
	man.Signature, man.SignatureAlg, man.KeyID = "", "", ""
	dst, dl := mustOpenCascade(t)
	if _, err := dst.ImportSession(man); err != nil {
		t.Fatalf("canonicalized v1 import must succeed: %v", err)
	}
	if faults, err := dl.VerifyChain(sess.ID); err != nil || len(faults) != 0 {
		t.Fatalf("imported v1 chain must verify: %v %+v", err, faults)
	}
}

func prettyJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
