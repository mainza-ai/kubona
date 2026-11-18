package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestManuscriptExportWritesVerifiableEvidence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "export.db")
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ms := &ledger.Manuscript{
		ID: "ms1", SessionID: "s1", TenantID: "default", Title: "T", Abstract: "A",
		ContentHash: "h1", RequestID: "req1",
		AuthorsJSON: `["Ada Lovelace"]`,
		Sections:    []ledger.ManuscriptSection{{Slug: "results", Title: "Results", LaTeX: "body"}},
		Citations:   []ledger.ManuscriptCitation{{Key: "k1", Title: "Ref", Year: "2024", Verified: true}},
	}
	if err := l.CreateManuscript(ms); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := l.ReplaceManuscriptArtifacts("s1", "ms1", "build1", map[string][]byte{
		"main.pdf":             []byte("%PDF-1.4 evidence"),
		"sections/results.tex": []byte("\\section{Results}"),
	}); err != nil {
		t.Fatalf("artifacts: %v", err)
	}
	if err := l.AddManuscriptAnnotation("s1", "ms1", ledger.ManuscriptAnnotation{
		ID: "ann1", Author: "operator", Anchor: "results", Body: "note",
	}); err != nil {
		t.Fatalf("annotation: %v", err)
	}
	if err := l.AddManuscriptReview("s1", "ms1", ledger.ManuscriptReviewRecord{
		ID: "rev1", Reviewer: "auto", Recommendation: "REVISE", ContentHash: "h1",
	}); err != nil {
		t.Fatalf("review: %v", err)
	}
	if err := l.SetManuscriptVerification("s1", "ms1",
		`{"version":"verification-v1","content_hash":"h1","checked_at":"2026-11-18T10:04:00Z","summary":{"status":"verified"}}`); err != nil {
		t.Fatalf("verification: %v", err)
	}
	if err := l.SetManuscriptSignoff("s1", "ms1", ledger.ManuscriptSignoff{
		SignerPubKey: "pk", Signature: "sig", ContentHash: "h1", Expiry: "2030-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("signoff: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at)
		VALUES ('job1', 's1', 'JOB_STATUS_COMPLETED', 'vqe_run', '{}', '{"energy":-1.1}',
		'2026-11-18T10:01:00Z', '2026-11-18T10:02:00Z')`); err != nil {
		t.Fatalf("job row: %v", err)
	}
	if _, err := l.MintClaimRecord("s1", "claim_eval", "H2 measured", 0.9, 0.05, 0.05,
		"verified", `{"job_id":"job1"}`); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := l.CreateHypothesis("s1", "default", &ledger.Hypothesis{
		ID: "hyp1", Statement: "The ansatz is under-expressive", Falsifier: "more layers do not help",
		ExperimentKind: "vqe", Source: "llm", Status: ledger.HypothesisCollapsed, ClaimID: "clm-s1-0001",
	}); err != nil {
		t.Fatalf("hypothesis: %v", err)
	}
	l.Close()

	out := filepath.Join(dir, "out")
	if code := runManuscriptExport([]string{
		"--db", dbPath, "--session", "s1", "--out", out,
	}); code != 0 {
		t.Fatalf("export exit %d", code)
	}
	for _, rel := range []string{
		"ms1/metadata.json", "ms1/sections.json", "ms1/citations.json",
		"ms1/reviews.json", "ms1/annotations.json",
		"ms1/artifacts/main.pdf", "ms1/artifacts/sections/results.tex",
		"ms1/prov.json", "ms1/credit.json",
		"ms1/ro-crate-metadata.json",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(out, "ms1", "artifacts", "main.pdf"))
	if err != nil || string(raw) != "%PDF-1.4 evidence" {
		t.Fatalf("artifact bytes = %q (%v)", raw, err)
	}
	crate, err := os.ReadFile(filepath.Join(out, "ms1", "ro-crate-metadata.json"))
	if err != nil {
		t.Fatalf("ro-crate: %v", err)
	}
	for _, want := range []string{
		"https://w3id.org/ro/crate/1.1/context", "ms1", "artifacts/main.pdf", "sha256",
		"prov:wasGeneratedBy", "kubona:VerificationReport", "kubona:creditRole",
		"prov.json", "credit.json",
	} {
		if !strings.Contains(string(crate), want) {
			t.Fatalf("ro-crate missing %q: %s", want, crate)
		}
	}

	provRaw, err := os.ReadFile(filepath.Join(out, "ms1", "prov.json"))
	if err != nil {
		t.Fatalf("prov: %v", err)
	}
	var provDoc map[string]any
	if err := json.Unmarshal(provRaw, &provDoc); err != nil {
		t.Fatalf("prov.json is not JSON: %v", err)
	}
	for _, key := range []string{"prefix", "entity", "activity", "agent", "wasGeneratedBy", "used", "wasAssociatedWith", "wasDerivedFrom"} {
		if _, ok := provDoc[key]; !ok {
			t.Fatalf("prov.json missing %q", key)
		}
	}
	entities := provDoc["entity"].(map[string]any)
	for _, id := range []string{"kubona:manuscript/ms1", "kubona:claim/clm-s1-0001", "kubona:job/job1", "kubona:verification/ms1"} {
		if _, ok := entities[id]; !ok {
			t.Fatalf("prov entity %q missing: %v", id, entities)
		}
	}

	creditRaw, err := os.ReadFile(filepath.Join(out, "ms1", "credit.json"))
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	var credit struct {
		Contributors []struct {
			Type  string   `json:"type"`
			Roles []string `json:"credit_roles"`
		} `json:"contributors"`
	}
	if err := json.Unmarshal(creditRaw, &credit); err != nil {
		t.Fatalf("credit.json is not JSON: %v", err)
	}
	types := map[string]bool{}
	for _, c := range credit.Contributors {
		types[c.Type] = true
		if len(c.Roles) == 0 {
			t.Fatalf("contributor without roles: %+v", c)
		}
	}
	for _, want := range []string{"declared-author", "software", "model", "human-operator"} {
		if !types[want] {
			t.Fatalf("credit type %q missing: %+v", want, types)
		}
	}
}
