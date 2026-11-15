package ledger

// B1 store tests: mint is server-owned (valid JSON + digest required), the
// status transition is compare-and-swap, and a message→proposal link survives
// read-back so history reloads can launch by id.

import (
	"encoding/json"
	"testing"
)

func TestProposalMintAndCAS(t *testing.T) {
	l := mustOpenObligationLedger(t)
	body, _ := json.Marshal(map[string]any{
		"qasm": "OPENQASM 2.0;", "backend": "local_simulator", "max_cost_usd": 0,
	})
	p, err := l.MintProposal("sess-1", "msg-1", "sha256:abc", body, 7, "", false)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if p.Status != ProposalOffered || p.BasisCampaignRev != 7 {
		t.Fatalf("unexpected proposal: %+v", p)
	}

	got, err := l.GetProposal(p.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.BodyJSON != string(body) || got.Digest != "sha256:abc" {
		t.Fatalf("body/digest must round-trip verbatim: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at must be recorded")
	}

	// One winner: the second CAS from `offered` loses.
	if ok, err := l.CASProposalStatus(p.ID, ProposalOffered, ProposalLaunched, "job-1"); err != nil || !ok {
		t.Fatalf("first CAS must win: %v %v", ok, err)
	}
	if ok, _ := l.CASProposalStatus(p.ID, ProposalOffered, ProposalLaunched, "job-2"); ok {
		t.Fatal("second launch must lose the CAS")
	}
	got, _ = l.GetProposal(p.ID)
	if got.Status != ProposalLaunched || got.LaunchedJobID != "job-1" {
		t.Fatalf("launch state must persist: %+v", got)
	}

	// Unknown transitions refuse instead of silently writing.
	if _, err := l.CASProposalStatus(p.ID, "offered", "teleported", ""); err == nil {
		t.Fatal("uncatalogued status must refuse")
	}
	if _, err := l.CASProposalStatus(p.ID, ProposalOffered, ProposalOffered, ""); err == nil {
		t.Fatal("self-transition must refuse")
	}
}

func TestProposalMintRefusesInvalidBody(t *testing.T) {
	l := mustOpenObligationLedger(t)
	if _, err := l.MintProposal("sess-1", "", "sha256:x", []byte("not json"), 0, "", false); err == nil {
		t.Fatal("invalid JSON must refuse")
	}
	if _, err := l.MintProposal("sess-1", "", "", []byte(`{}`), 0, "", false); err == nil {
		t.Fatal("missing digest must refuse")
	}
	if _, err := l.MintProposal("", "", "sha256:x", []byte(`{}`), 0, "", false); err == nil {
		t.Fatal("missing session must refuse")
	}
	if p, _ := l.GetProposal("pr_missing"); p != nil {
		t.Fatal("absent proposal must be nil")
	}
}

func TestProposalMessageLink(t *testing.T) {
	l := mustOpenObligationLedger(t)
	body, _ := json.Marshal(map[string]any{"qasm": "OPENQASM 2.0;", "backend": "local_simulator"})
	p, err := l.MintProposal("sess-1", "msg-1", "sha256:abc", body, 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, status, spend_limit_micro_usd, current_spend_micro_usd, tenant_id, created_at, updated_at)
		VALUES ('sess-1', 'p', 'active', 0, 0, 'default', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO chat_messages (id, session_id, role, content, proposal_json, created_at)
		VALUES ('msg-1', 'sess-1', 'assistant', 'plan', ?, CURRENT_TIMESTAMP)`, string(body)); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	if err := l.SetMessageProposal("msg-1", p.ID, p.Digest); err != nil {
		t.Fatalf("link: %v", err)
	}
	got, err := l.GetProposalForMessage("msg-1")
	if err != nil || got == nil || got.ID != p.ID {
		t.Fatalf("message link must resolve the proposal: %+v %v", got, err)
	}
	if got, _ := l.GetProposalForMessage("msg-missing"); got != nil {
		t.Fatal("absent message must be nil")
	}
}
