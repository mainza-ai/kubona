package main

import (
	"os"
	"path/filepath"
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
		Sections:  []ledger.ManuscriptSection{{Slug: "results", Title: "Results", LaTeX: "body"}},
		Citations: []ledger.ManuscriptCitation{{Key: "k1", Title: "Ref", Year: "2024", Verified: true}},
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
	if err := l.SetManuscriptSignoff("s1", "ms1", ledger.ManuscriptSignoff{
		SignerPubKey: "pk", Signature: "sig", ContentHash: "h1", Expiry: "2030-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("signoff: %v", err)
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
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(out, "ms1", "artifacts", "main.pdf"))
	if err != nil || string(raw) != "%PDF-1.4 evidence" {
		t.Fatalf("artifact bytes = %q (%v)", raw, err)
	}
}
