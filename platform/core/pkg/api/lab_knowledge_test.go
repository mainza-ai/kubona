package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const repeatStatement = "Scan the H2 bond distance around the equilibrium point for a lower energy minimum."

func TestPriorNegativeRegistryBlocksRepeatsAcrossCampaigns(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	planBody := func(ack bool) map[string]any {
		return map[string]any{
			"statement": repeatStatement, "method": "vqe", "observable": "energy",
			"acceptance_metric": "nonexistent_anchor", "tolerance": 0.05, "stop_rule": "one run",
			"acknowledge_prior_negative": ack,
			"spec":                       map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
		}
	}
	// First campaign: the plan runs and is rejected (the anchor field is absent),
	// so the negative result is published.
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", planBody(false))
	if w.Code != http.StatusCreated {
		t.Fatalf("first plan = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
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
	if run.Accepted {
		t.Fatal("the probe was expected to be rejected so it can become a negative result")
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/lab-knowledge?kind=negative_result", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("knowledge list = %d: %s", w.Code, w.Body.String())
	}
	var listed struct {
		Entries []struct {
			ID        string `json:"id"`
			Statement string `json:"statement"`
			Kind      string `json:"kind"`
			Evidence  string `json:"evidence_json"`
		} `json:"entries"`
	}
	json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Entries) != 1 || listed.Entries[0].Kind != "negative_result" ||
		!strings.Contains(listed.Entries[0].Evidence, "plan_id") {
		t.Fatalf("published negative = %+v", listed.Entries)
	}
	entryID := listed.Entries[0].ID

	// Second campaign, same tenant: the repeated plan refuses without an ack.
	sid2 := createSession(t, srv, "default")
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid2+"/plans", "", planBody(false))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "prior_negative_exists") {
		t.Fatalf("repeated plan = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid2+"/plans", "", planBody(true))
	if w.Code != http.StatusCreated {
		t.Fatalf("acknowledged plan = %d: %s", w.Code, w.Body.String())
	}
	var acked struct {
		PriorNegative []struct {
			ID         string  `json:"id"`
			Similarity float64 `json:"similarity"`
		} `json:"prior_negative"`
		PriorNegativeAck bool `json:"prior_negative_ack"`
	}
	json.Unmarshal(w.Body.Bytes(), &acked)
	if len(acked.PriorNegative) != 1 || !acked.PriorNegativeAck ||
		acked.PriorNegative[0].Similarity < 0.8 {
		t.Fatalf("acknowledged response = %+v", acked)
	}

	// Another tenant is scoped out until the owner shares the entry.
	other := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+other+"/plans", "op-token", planBody(false))
	if w.Code != http.StatusCreated {
		t.Fatalf("other tenant plan = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/lab-knowledge/"+entryID+"/share", "op-token",
		map[string]any{"grantee_tenant": "other"})
	if w.Code != http.StatusOK {
		t.Fatalf("share = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+other+"/lab-knowledge/lookup?statement="+url.QueryEscape(repeatStatement), "op-token", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"avoid":true`) {
		t.Fatalf("shared lookup = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+other+"/plans", "op-token", planBody(false))
	if w.Code != http.StatusConflict {
		t.Fatalf("shared tenant repeat = %d: %s", w.Code, w.Body.String())
	}

	// Only the owning tenant may share.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+other+"/lab-knowledge/"+entryID+"/share", "op-token",
		map[string]any{"grantee_tenant": "default"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign share = %d: %s", w.Code, w.Body.String())
	}
}

func TestCollapseAndSignoffPublishKnowledgeEntries(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	claim, err := srv.Ledger().MintClaimRecord(sid, "claim_eval", "H2 measured", 0.9, 0.05, 0.05,
		"verified", `{"energy":-1.13727}`)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	if w.Code != http.StatusCreated {
		t.Fatalf("hypotheses = %d: %s", w.Code, w.Body.String())
	}
	var gen struct {
		Hypotheses []struct {
			ID string `json:"id"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &gen)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+gen.Hypotheses[0].ID+"/collapse", "",
		map[string]any{"claim_id": claim.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("collapse = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("manuscript = %d: %s", w.Code, w.Body.String())
	}
	var ms struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &ms)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+ms.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": ms.Manuscript.ContentHash})
	if w.Code != http.StatusOK {
		t.Fatalf("signoff = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/lab-knowledge", "", nil)
	var all struct {
		Entries []struct {
			Kind     string `json:"kind"`
			Evidence string `json:"evidence_json"`
		} `json:"entries"`
	}
	json.Unmarshal(w.Body.Bytes(), &all)
	kinds := map[string]string{}
	for _, e := range all.Entries {
		kinds[e.Kind] = e.Evidence
	}
	if !strings.Contains(kinds["claim"], claim.ID) {
		t.Fatalf("claim entry missing: %+v", all.Entries)
	}
	if !strings.Contains(kinds["report"], ms.Manuscript.ID) {
		t.Fatalf("report entry missing: %+v", all.Entries)
	}
}
