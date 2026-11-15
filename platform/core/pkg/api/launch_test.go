package api_test

// B1 — launch is bound to a server-minted proposal. These pins cover: the
// client cannot substitute a circuit/backend at launch, the stored body is
// what dispatches, a proposal is single-use, the digest and basis are
// enforced, and dismissal/unknown ids refuse without an oracle.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const b1QASM = "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n"

func b1Session(t *testing.T, h http.Handler, title string) string {
	t.Helper()
	return idemSession(t, h, title)
}

func b1Launch(t *testing.T, h http.Handler, payload map[string]any) (int, string) {
	t.Helper()
	code, body, _ := idemPost(t, h, "/api/chat/launch", payload)
	return code, body
}

func b1Get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestChatLaunchRequiresMintedProposal(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "launch requires proposal")

	// Missing identity: no circuit, no backend — the request names nothing.
	if code, body := b1Launch(t, h, map[string]any{"session_id": sid}); code != http.StatusBadRequest || !strings.Contains(body, "proposal_id") {
		t.Fatalf("empty launch must 400 naming proposal_id, got %d %q", code, body)
	}
	// The old client-echoed fields are not accepted: strict decode refuses
	// the substitution rather than silently ignoring it.
	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "qasm": b1QASM, "backend": "local_simulator",
	}); code != http.StatusBadRequest || !strings.Contains(strings.ToLower(body), "unknown field") {
		t.Fatalf("client-supplied circuit fields must be refused as unknown fields, got %d %q", code, body)
	}
	// A fake id refuses like an absent one.
	if code, _ := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": "pr_missing", "digest": "sha256:00",
	}); code != http.StatusNotFound {
		t.Fatalf("unknown proposal must 404, got %d", code)
	}
}

func TestChatLaunchDispatchesServerOwnedBody(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "launch body")
	pid, digest := mintTestProposal(t, srv, sid, b1QASM, "local_simulator", 1.0)

	code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest, "request_id": "b1-body-1",
	})
	if code != http.StatusCreated {
		t.Fatalf("launch: %d %s", code, body)
	}
	var out struct {
		Job struct {
			Params map[string]any `json:"params_json"`
		} `json:"job"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Job.Params["qasm"] != b1QASM {
		t.Fatalf("job must carry the stored proposal circuit, got %v", out.Job.Params["qasm"])
	}
	if out.Job.Params["requested_backend"] != "local_simulator" {
		t.Fatalf("job must carry the normalized backend, got %v", out.Job.Params["requested_backend"])
	}
	if out.Job.Params["proposal_id"] != pid || out.Job.Params["proposal_digest"] != digest {
		t.Fatalf("job must record the proposal identity, got %v/%v", out.Job.Params["proposal_id"], out.Job.Params["proposal_digest"])
	}
	if got := int(out.Job.Params["shots"].(float64)); got != 1024 {
		t.Fatalf("proposal default shots must be recorded, got %d", got)
	}
}

func TestChatLaunchCarriesStoredAttribution(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "attribution")

	// Seed a stored assistant plan, then mint from it: attribution comes from
	// the server-owned body, never from the launch request.
	proposal := map[string]any{
		"target": "H2", "qasm": b1QASM, "recommended_tier": "local_simulator",
		"designed_by": "server-fallback",
	}
	raw, _ := json.Marshal(proposal)
	if _, err := srv.Ledger().DB().Exec(`INSERT INTO chat_messages (id, session_id, role, content, proposal_json, created_at)
		VALUES ('msg-b1', ?, 'assistant', 'plan', ?, CURRENT_TIMESTAMP)`, sid, string(raw)); err != nil {
		t.Fatal(err)
	}
	code, body, _ := idemPost(t, h, "/api/proposals", map[string]any{"session_id": sid, "source": "message", "message_id": "msg-b1"})
	if code != http.StatusCreated {
		t.Fatalf("mint from message: %d %s", code, body)
	}
	var minted struct {
		Proposal struct {
			ID     string `json:"id"`
			Digest string `json:"digest"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal([]byte(body), &minted); err != nil {
		t.Fatal(err)
	}

	code, body = b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": minted.Proposal.ID, "digest": minted.Proposal.Digest,
	})
	if code != http.StatusCreated {
		t.Fatalf("launch: %d %s", code, body)
	}
	var out struct {
		Job struct {
			Params map[string]any `json:"params_json"`
		} `json:"job"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Job.Params["designed_by"] != "server-fallback" {
		t.Fatalf("stored attribution must ride the job, got %v", out.Job.Params["designed_by"])
	}
}

func TestChatLaunchEnforcesDigestAndBasis(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "digest and basis")
	pid, digest := mintTestProposal(t, srv, sid, b1QASM, "local_simulator", 1.0)

	// Wrong digest never dispatches.
	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": "sha256:deadbeef",
	}); code != http.StatusBadRequest || !strings.Contains(body, "digest") {
		t.Fatalf("digest mismatch must 400, got %d %q", code, body)
	}

	// The campaign moved after the plan was priced: launch refuses stale.
	state, _ := json.Marshal(map[string]any{
		"session_id": sid, "target_system": "H2",
		"quantum_factors": map[string]any{"qasm": b1QASM, "recommended_backend": "local_simulator", "estimated_spend_usd": 1.0},
	})
	if err := srv.Ledger().SaveCampaignState(sid, state); err != nil {
		t.Fatal(err)
	}
	code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest,
	})
	if code != http.StatusConflict || !strings.Contains(body, "proposal_stale") {
		t.Fatalf("moved campaign must 409 proposal_stale, got %d %q", code, body)
	}
}

func TestProposalIsSingleUse(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "single use")
	pid, digest := mintTestProposal(t, srv, sid, b1QASM, "local_simulator", 1.0)

	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest, "request_id": "b1-once",
	}); code != http.StatusCreated {
		t.Fatalf("first launch: %d %s", code, body)
	}
	// Same request id replays the admitted job (lost-reply recovery).
	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest, "request_id": "b1-once",
	}); code != http.StatusCreated || !strings.Contains(body, `"job"`) {
		t.Fatalf("idempotent replay must return the job, got %d %s", code, body)
	}
	// A new request id cannot launch the burned proposal again.
	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest, "request_id": "b1-twice",
	}); code != http.StatusConflict || !strings.Contains(body, "proposal_not_offered") {
		t.Fatalf("second launch must 409 proposal_not_offered, got %d %q", code, body)
	}

	// The persisted state carries the launched job id.
	code, body := b1Get(t, h, "/api/proposals/"+pid)
	if code != http.StatusOK {
		t.Fatalf("proposal read: %d %s", code, body)
	}
	var out struct {
		Proposal struct {
			Status        string `json:"status"`
			LaunchedJobID string `json:"launched_job_id"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Proposal.Status != "launched" || out.Proposal.LaunchedJobID == "" {
		t.Fatalf("launch state must persist, got %+v", out.Proposal)
	}
}

func TestProposalDismissAndSessionBinding(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "dismiss")
	other := b1Session(t, h, "other lab session")
	pid, digest := mintTestProposal(t, srv, sid, b1QASM, "local_simulator", 0)

	// Another session cannot see or launch this proposal (no oracle).
	if code, _ := b1Launch(t, h, map[string]any{
		"session_id": other, "proposal_id": pid, "digest": digest,
	}); code != http.StatusNotFound {
		t.Fatalf("cross-session launch must 404, got %d", code)
	}
	if code, _ := b1Get(t, h, "/api/proposals/"+pid); code != http.StatusOK {
		t.Fatalf("owner read must 200, got %d", code)
	}

	// Dismiss is a CAS offered→dismissed; launch afterwards refuses.
	if code, body, _ := idemPost(t, h, "/api/proposals/"+pid+"/dismiss", map[string]any{}); code != http.StatusOK {
		t.Fatalf("dismiss: %d %s", code, body)
	}
	if code, body, _ := idemPost(t, h, "/api/proposals/"+pid+"/dismiss", map[string]any{}); code != http.StatusConflict {
		t.Fatalf("second dismiss must 409, got %d %s", code, body)
	}
	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest,
	}); code != http.StatusConflict || !strings.Contains(body, "proposal_not_offered") {
		t.Fatalf("dismissed proposal must not launch, got %d %q", code, body)
	}
}

func TestProposalMintRefusesUnpriceablePlans(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "unpriceable")

	// Unknown backend: cannot be priced or routed honestly.
	state, _ := json.Marshal(map[string]any{
		"session_id": sid, "target_system": "H2",
		"quantum_factors": map[string]any{"qasm": b1QASM, "recommended_backend": "mystery_box", "estimated_spend_usd": 1.0},
	})
	if err := srv.Ledger().SaveCampaignState(sid, state); err != nil {
		t.Fatal(err)
	}
	if code, body, _ := idemPost(t, h, "/api/proposals", map[string]any{"session_id": sid, "source": "campaign"}); code != http.StatusBadRequest || !strings.Contains(body, "mystery_box") {
		t.Fatalf("unknown backend must 400 naming it, got %d %s", code, body)
	}

	// Metered hardware with no numeric bound: text estimates are not a spend
	// bound. The mint refuses instead of defaulting a $1 ceiling.
	state, _ = json.Marshal(map[string]any{
		"session_id": sid, "target_system": "H2",
		"quantum_factors": map[string]any{"qasm": b1QASM, "recommended_backend": "ibm_fez", "estimated_spend_usd": 0},
	})
	if err := srv.Ledger().SaveCampaignState(sid, state); err != nil {
		t.Fatal(err)
	}
	if code, body, _ := idemPost(t, h, "/api/proposals", map[string]any{"session_id": sid, "source": "campaign"}); code != http.StatusBadRequest || !strings.Contains(body, "max_cost_usd") {
		t.Fatalf("unpriced hardware must 400 naming max_cost_usd, got %d %s", code, body)
	}
}

func TestProposalMintIsContentIdempotent(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "mint twice")

	state, _ := json.Marshal(map[string]any{
		"session_id": sid, "target_system": "H2",
		"quantum_factors": map[string]any{"qasm": b1QASM, "recommended_backend": "local_simulator", "estimated_spend_usd": 1.0},
	})
	if err := srv.Ledger().SaveCampaignState(sid, state); err != nil {
		t.Fatal(err)
	}
	mint := func() (int, string) {
		code, body, _ := idemPost(t, h, "/api/proposals", map[string]any{"session_id": sid, "source": "campaign"})
		var out struct {
			Proposal struct {
				ID string `json:"id"`
			} `json:"proposal"`
		}
		if code != http.StatusCreated && code != http.StatusOK {
			t.Fatalf("mint: %d %s", code, body)
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		return code, out.Proposal.ID
	}
	_, first := mint()
	_, second := mint()
	if first == "" || second != first {
		t.Fatalf("same plan must mint one row, got %q then %q", first, second)
	}
}

func TestProposalSimulatorDowngradeIsMonotone(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := b1Session(t, h, "downgrade")

	state, _ := json.Marshal(map[string]any{
		"session_id": sid, "target_system": "H2",
		"quantum_factors": map[string]any{"qasm": b1QASM, "recommended_backend": "ibm_fez", "estimated_spend_usd": 25.0},
	})
	if err := srv.Ledger().SaveCampaignState(sid, state); err != nil {
		t.Fatal(err)
	}
	code, body, _ := idemPost(t, h, "/api/proposals", map[string]any{"session_id": sid, "source": "campaign", "downgrade": "simulator"})
	if code != http.StatusCreated {
		t.Fatalf("downgrade mint: %d %s", code, body)
	}
	var minted struct {
		Proposal struct {
			ID     string `json:"id"`
			Digest string `json:"digest"`
			Body   struct {
				Backend    string  `json:"backend"`
				MaxCostUSD float64 `json:"max_cost_usd"`
			} `json:"body"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal([]byte(body), &minted); err != nil {
		t.Fatal(err)
	}
	if minted.Proposal.Body.Backend != "local_simulator" || minted.Proposal.Body.MaxCostUSD != 0 {
		t.Fatalf("downgrade must be local/zero-cost, got %+v", minted.Proposal.Body)
	}
	// Any other downgrade value refuses: this field can only reduce risk.
	if code, body, _ := idemPost(t, h, "/api/proposals", map[string]any{"session_id": sid, "source": "campaign", "downgrade": "ibm_brisbane"}); code != http.StatusBadRequest {
		t.Fatalf("non-simulator downgrade must 400, got %d %s", code, body)
	}
	// The downgraded plan launches without a mandate (simulator rail).
	if code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": minted.Proposal.ID, "digest": minted.Proposal.Digest, "request_id": "downgrade-1",
	}); code != http.StatusCreated {
		t.Fatalf("downgraded launch must admit: %d %s", code, body)
	}
}
