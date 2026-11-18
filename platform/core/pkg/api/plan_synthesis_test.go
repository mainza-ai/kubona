package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPlanSynthesisFromHypothesisPatch(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 12})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var gen struct {
		Hypotheses []struct {
			ID        string         `json:"id"`
			Statement string         `json:"statement"`
			SpecPatch map[string]any `json:"spec_patch"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &gen)
	var target string
	for _, h := range gen.Hypotheses {
		if strings.Contains(strings.ToLower(h.Statement), "under-expressive") {
			target = h.ID
			if h.SpecPatch["layers_delta"] != float64(1) {
				t.Fatalf("expressivity probe patch = %+v", h.SpecPatch)
			}
		}
	}
	if target == "" {
		t.Fatal("no expressivity probe in the generated set")
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/hypotheses/"+target, "", map[string]any{"status": "shortlisted"})
	if w.Code != http.StatusOK {
		t.Fatalf("shortlist = %d: %s", w.Code, w.Body.String())
	}

	// No caller spec: the server synthesizes from campaign facts + patch.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"hypothesis_id": target, "observable": "energy",
		"acceptance_metric": "fci_exact_energy", "tolerance": 0.02,
		"stop_rule": "one run; reject if outside tolerance", "budget_usd": 1.0,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("synthesized plan = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Plan struct {
			ID        string   `json:"id"`
			Spec      string   `json:"spec"`
			SpecBasis []string `json:"spec_basis"`
			Status    string   `json:"status"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	var spec map[string]any
	if err := json.Unmarshal([]byte(created.Plan.Spec), &spec); err != nil {
		t.Fatalf("spec is not JSON: %v", err)
	}
	if spec["kind"] != "vqe" || spec["molecule"] != "H2" {
		t.Fatalf("synthesized identity = %+v", spec)
	}
	if spec["bond_distance"] != 0.7414 {
		t.Fatalf("bond distance = %v", spec["bond_distance"])
	}
	if spec["layers"] != float64(3) {
		t.Fatalf("layers = %v, want the patched 3 (measured 2 + 1)", spec["layers"])
	}
	if spec["shots"] != float64(4096) {
		t.Fatalf("shots = %v", spec["shots"])
	}
	if len(created.Plan.SpecBasis) < 3 {
		t.Fatalf("spec provenance missing: %+v", created.Plan.SpecBasis)
	}
	joined := strings.Join(created.Plan.SpecBasis, "\n")
	for _, want := range []string{"molecule=H2", "layers", "shots=4096"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("basis missing %q:\n%s", want, joined)
		}
	}
	if created.Plan.Status != "proposed" {
		t.Fatalf("status = %q", created.Plan.Status)
	}

	// The synthesized plan executes through the real rail.
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+created.Plan.ID, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+created.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("execute = %d: %s", w.Code, w.Body.String())
	}
	var run struct {
		Accepted bool `json:"accepted"`
	}
	json.Unmarshal(w.Body.Bytes(), &run)
	if !run.Accepted {
		t.Fatalf("synthesized plan was not accepted: %s", w.Body.String())
	}
}

func TestPlanSynthesisRefusesWhenCampaignCannotGroundIt(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default") // no campaign at all

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var gen struct {
		Hypotheses []struct {
			ID string `json:"id"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &gen)
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/hypotheses/"+gen.Hypotheses[0].ID, "", map[string]any{"status": "shortlisted"})
	if w.Code != http.StatusOK {
		t.Fatalf("shortlist = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"hypothesis_id": gen.Hypotheses[0].ID, "observable": "energy",
		"acceptance_metric": "fci_exact_energy", "tolerance": 0.02, "stop_rule": "one run",
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "spec_unavailable") {
		t.Fatalf("ungrounded synthesis = %d: %s", w.Code, w.Body.String())
	}
	// A caller-supplied spec still works without campaign facts.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"hypothesis_id": gen.Hypotheses[0].ID, "observable": "energy",
		"acceptance_metric": "fci_exact_energy", "tolerance": 0.02, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("explicit-spec plan = %d: %s", w.Code, w.Body.String())
	}
}
