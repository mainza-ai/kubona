package api_test

// I3 HTTP tests: funded loops reserve at admission (tenant + session),
// refuse when the ceiling cannot cover, settle on stop, and sleep/wake
// symmetrically. Priced roles are enabled on a scratch provider (local
// pricing math — no LLM calls happen at admission).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupFundedServer(t *testing.T) (*api.Server, *ledger.Ledger, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	l, err := ledger.OpenLedger(t.TempDir() + "/funded.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Price the loop roles on the scratch provider (math only).
	set, err := llm.LoadSettings(l.DB())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	snap := set.GetSnapshot()
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.Enabled = true
	snap.Providers[llm.ProviderOpenCode] = pc
	snap.RoleAssignments["coder"] = llm.RoleModelAssignment{Role: "coder", Provider: llm.ProviderOpenCode, Model: "deepseek-v4.1-flash"}
	snap.RoleAssignments["critic"] = llm.RoleModelAssignment{Role: "critic", Provider: llm.ProviderOpenCode, Model: "deepseek-v4.1-flash"}
	if err := set.Update(&snap, nil); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	if err := set.Save(l.DB()); err != nil {
		t.Fatalf("settings save: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	return srv, l, func() { l.Close() }
}

func startLoop(t *testing.T, h http.Handler, sid string, maxIter int) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"session_id": sid, "harness_id": "ansatz-search",
		"initial_artifact": "", "max_iterations": maxIter,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/autoresearch/start", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func loopSpendAction(t *testing.T, h http.Handler, loopID, action string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+loopID+"/"+action, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestLoopAdmissionReserves(t *testing.T) {
	srv, l, close := setupFundedServer(t)
	defer close()
	h := srv.Handler()
	sid := idemSession(t, h, "funded loop")

	code, out := startLoop(t, h, sid, 10)
	if code != http.StatusCreated {
		t.Fatalf("start: %d %v", code, out)
	}
	loopID, _ := out["id"].(string)
	if loopID == "" {
		t.Fatalf("no loop id: %v", out)
	}
	// 10 iters x priced roles: recompute the bound from the same
	// persisted settings the server priced (no unexported access).
	persisted, err := llm.LoadSettings(l.DB())
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	perIter, _ := llm.LoopIterationBound(persisted, []string{"coder", "critic"})
	want := perIter * 10
	tenant, err := srv.TenantManager().GetTenant("default")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if tenant.SpentUSD != want {
		t.Fatalf("tenant must hold the reservation %.4f, got %.4f", want, tenant.SpentUSD)
	}
	row, err := l.GetLoopState(loopID)
	if err != nil || row == nil {
		t.Fatalf("loop row: %+v %v", row, err)
	}
	if row.ReservedAmount != want || row.ReservedTenant != "default" || row.ConsumedAmount != 0 {
		t.Fatalf("row books wrong: %+v", row)
	}
}

func TestLoopAdmissionRefusesUnfunded(t *testing.T) {
	srv, _, close := setupFundedServer(t)
	defer close()
	h := srv.Handler()
	sid := idemSession(t, h, "broke loop")

	code, out := startLoop(t, h, sid, 1000000)
	if code != http.StatusForbidden {
		t.Fatalf("unfundable loop must 403, got %d %v", code, out)
	}
	if out["code"] != "ceiling_exceeded" {
		t.Fatalf("refusal must carry typed ceiling_exceeded: %v", out)
	}
}

func TestLoopStopReleases(t *testing.T) {
	srv, l, close := setupFundedServer(t)
	defer close()
	h := srv.Handler()
	sid := idemSession(t, h, "stop releases")

	code, out := startLoop(t, h, sid, 10)
	if code != http.StatusCreated {
		t.Fatalf("start: %d %v", code, out)
	}
	loopID := out["id"].(string)
	before, _ := srv.TenantManager().GetTenant("default")

	if code, _ := loopSpendAction(t, h, loopID, "stop"); code != http.StatusOK {
		t.Fatalf("stop: %d", code)
	}
	after, _ := srv.TenantManager().GetTenant("default")
	if after.SpentUSD != 0 {
		t.Fatalf("stop must release the full reservation (nothing consumed): before=%.4f after=%.4f", before.SpentUSD, after.SpentUSD)
	}
	row, _ := l.GetLoopState(loopID)
	if row.ReservedAmount != 0 {
		t.Fatalf("released row must read zero: %+v", row)
	}
	// Second stop: idempotent no-op (claim-once).
	if code, _ := loopSpendAction(t, h, loopID, "stop"); code != http.StatusOK {
		t.Fatalf("second stop: %d", code)
	}
	again, _ := srv.TenantManager().GetTenant("default")
	if again.SpentUSD != 0 {
		t.Fatalf("double stop must not move books: %.4f", again.SpentUSD)
	}
}

func TestLoopSleepWakeSymmetry(t *testing.T) {
	srv, _, close := setupFundedServer(t)
	defer close()
	h := srv.Handler()
	sid := idemSession(t, h, "sleep wake")

	code, out := startLoop(t, h, sid, 10)
	if code != http.StatusCreated {
		t.Fatalf("start: %d %v", code, out)
	}
	loopID := out["id"].(string)
	funded, _ := srv.TenantManager().GetTenant("default")
	if funded.SpentUSD == 0 {
		t.Fatal("loop must hold a reservation to test symmetry")
	}
	// Sleep releases.
	if code, _ := loopSpendAction(t, h, loopID, "sleep"); code != http.StatusOK {
		t.Fatalf("sleep: %d", code)
	}
	slept, _ := srv.TenantManager().GetTenant("default")
	if slept.SpentUSD != 0 {
		t.Fatalf("sleep must release the reservation, spent=%.4f", slept.SpentUSD)
	}
	// Wake re-reserves the remainder.
	if code, _ := loopSpendAction(t, h, loopID, "wake"); code != http.StatusOK {
		t.Fatalf("wake: %d", code)
	}
	woke, _ := srv.TenantManager().GetTenant("default")
	if woke.SpentUSD != funded.SpentUSD {
		t.Fatalf("wake must restore the reservation %.4f, got %.4f", funded.SpentUSD, woke.SpentUSD)
	}
}
