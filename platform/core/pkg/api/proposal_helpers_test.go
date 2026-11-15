package api_test

// Shared B1 test helper: mint a proposal from a stored campaign state and
// return {id, digest}. Tests launch by naming it instead of echoing a circuit.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
)

func mintTestProposal(t *testing.T, srv *api.Server, sid, qasm, backend string, cost float64) (id, digest string) {
	t.Helper()
	state := map[string]any{
		"session_id":    sid,
		"target_system": "H2",
		"quantum_factors": map[string]any{
			"qasm":                qasm,
			"recommended_backend": backend,
			"estimated_spend_usd": cost,
		},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().SaveCampaignState(sid, raw); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	code, body, _ := idemPost(t, srv.Handler(), "/api/proposals", map[string]any{
		"session_id": sid, "source": "campaign",
	})
	if code != http.StatusCreated {
		t.Fatalf("mint proposal: %d %s", code, body)
	}
	var out struct {
		Proposal struct {
			ID     string `json:"id"`
			Digest string `json:"digest"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || out.Proposal.ID == "" || out.Proposal.Digest == "" {
		t.Fatalf("mint decode: %v %s", err, body)
	}
	return out.Proposal.ID, out.Proposal.Digest
}
