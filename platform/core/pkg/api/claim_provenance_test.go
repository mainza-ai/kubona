package api_test

// B4 API pin: provenance is a typed read of the claim's evidence links, bound
// to the owning session, and the summary states whether a verified claim has
// measured support.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func claimProvenanceGet(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestClaimProvenanceEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "provenance drill")
	other := idemSession(t, h, "other lab session")

	measured, err := srv.Ledger().MintClaimRecord(sid, "claim_eval", "VQE H2 measured", 0.9, 0.05, 0.05, "verified", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().AddClaimEvidence(sid, measured.ID, ledger.ClaimEvidence{
		Kind: "job_result", JobID: "job-42", ResultSHA256: ledger.ResultDigest([]byte(`{"energy":-1.13}`)),
	}); err != nil {
		t.Fatal(err)
	}
	incomplete, err := srv.Ledger().MintClaimRecord(sid, "tribunal", "Consensus without a measurement", -1, -1, -1, "verified", "{}")
	if err != nil {
		t.Fatal(err)
	}

	code, body := claimProvenanceGet(t, h, "/api/sessions/"+sid+"/claims/"+measured.ID+"/provenance")
	if code != http.StatusOK {
		t.Fatalf("provenance: %d %s", code, body)
	}
	var out struct {
		Claim    ledger.ClaimRecord            `json:"claim"`
		Evidence []ledger.ClaimEvidence        `json:"evidence"`
		Summary  ledger.ClaimProvenanceSummary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Claim.ID != measured.ID || len(out.Evidence) != 1 {
		t.Fatalf("claim/evidence mismatch: %s", body)
	}
	if out.Evidence[0].Kind != "job_result" || out.Evidence[0].JobID != "job-42" {
		t.Fatalf("evidence link malformed: %+v", out.Evidence[0])
	}
	if out.Summary.Measured != 1 || !out.Summary.Complete {
		t.Fatalf("measured evidence must complete the claim: %+v", out.Summary)
	}

	// Verified tribunal consensus with no measured job: readable, incomplete.
	code, body = claimProvenanceGet(t, h, "/api/sessions/"+sid+"/claims/"+incomplete.ID+"/provenance")
	if code != http.StatusOK {
		t.Fatalf("incomplete provenance: %d %s", code, body)
	}
	var bad struct {
		Summary ledger.ClaimProvenanceSummary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(body), &bad); err != nil {
		t.Fatal(err)
	}
	if bad.Summary.Complete || bad.Summary.Measured != 0 {
		t.Fatalf("unmeasured verified claim must report incomplete: %+v", bad.Summary)
	}

	// Unknown claims and another session's claims refuse without an oracle.
	if code, _ := claimProvenanceGet(t, h, "/api/sessions/"+sid+"/claims/clm-missing-0001/provenance"); code != http.StatusNotFound {
		t.Fatalf("unknown claim must 404, got %d", code)
	}
	if code, _ := claimProvenanceGet(t, h, "/api/sessions/"+other+"/claims/"+measured.ID+"/provenance"); code != http.StatusNotFound {
		t.Fatalf("cross-session claim must 404, got %d", code)
	}
}

func TestClaimEvidenceWriteEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "evidence write")

	// A real queued job: measured evidence is derived from its row, never
	// from a client-supplied digest.
	code, body, _ := idemPost(t, h, "/api/jobs", map[string]any{
		"session_id": sid, "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n",
		"backend": "local_simulator", "shots": 8,
	})
	if code != http.StatusCreated {
		t.Fatalf("job: %d %s", code, body)
	}
	var jobOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &jobOut); err != nil || jobOut.ID == "" {
		t.Fatalf("job decode: %v %s", err, body)
	}

	claim, err := srv.Ledger().MintClaimRecord(sid, "vqe_run", "measured claim", -1, -1, -1, "verified", "{}")
	if err != nil {
		t.Fatal(err)
	}
	// Wrong/foreign job ids refuse before any row.
	if code, _, _ := idemPost(t, h, "/api/sessions/"+sid+"/claims/"+claim.ID+"/evidence", map[string]any{
		"kind": "job_result", "job_id": "job-does-not-exist",
	}); code != http.StatusNotFound {
		t.Fatalf("unknown job evidence must 404, got %d", code)
	}
	// Unknown kind refuses.
	if code, _, _ := idemPost(t, h, "/api/sessions/"+sid+"/claims/"+claim.ID+"/evidence", map[string]any{
		"kind": "wizardry", "source_ref": "x",
	}); code != http.StatusBadRequest {
		t.Fatalf("unknown kind must 400, got %d", code)
	}
	// Measured link: server computes the digest from the job row.
	if code, body, _ := idemPost(t, h, "/api/sessions/"+sid+"/claims/"+claim.ID+"/evidence", map[string]any{
		"kind": "job_result", "job_id": jobOut.ID,
	}); code != http.StatusCreated {
		t.Fatalf("measured evidence: %d %s", code, body)
	}
	code, body = claimProvenanceGet(t, h, "/api/sessions/"+sid+"/claims/"+claim.ID+"/provenance")
	if code != http.StatusOK {
		t.Fatalf("provenance: %d %s", code, body)
	}
	var out struct {
		Evidence []ledger.ClaimEvidence        `json:"evidence"`
		Summary  ledger.ClaimProvenanceSummary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 || out.Evidence[0].ResultSHA256 == "" || out.Summary.Measured != 1 || !out.Summary.Complete {
		t.Fatalf("measured provenance must be complete with a server digest: %s", body)
	}
	// Explicit general knowledge is the other honest route.
	if code, body, _ := idemPost(t, h, "/api/sessions/"+sid+"/claims/"+claim.ID+"/evidence", map[string]any{
		"kind": "general_knowledge", "source_ref": "textbook:Atkins",
	}); code != http.StatusCreated {
		t.Fatalf("general knowledge: %d %s", code, body)
	}
}
