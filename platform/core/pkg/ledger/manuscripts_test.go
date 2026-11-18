package ledger_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func openManuscriptLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "manuscripts.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func draft(id, session string) *ledger.Manuscript {
	return &ledger.Manuscript{
		ID: id, SessionID: session, TenantID: "lab-1", Title: "Measured results",
		Abstract: "No claim.", ContentHash: "hash-1",
		Sections: []ledger.ManuscriptSection{
			{Slug: "introduction", Title: "Introduction", LaTeX: "Intro.", Position: 0},
			{Slug: "results", Title: "Results", LaTeX: "Table.", Position: 1},
		},
		Citations: []ledger.ManuscriptCitation{{Key: "k1", Title: "T", Year: "2024", DOI: "10.1/x"}},
	}
}

func TestManuscriptRoundTripAndList(t *testing.T) {
	l := openManuscriptLedger(t)
	if err := l.CreateManuscript(draft("ms1", "s1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := l.GetManuscript("s1", "ms1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Measured results" || got.State != ledger.ManuscriptStateDraft {
		t.Fatalf("unexpected manuscript: %+v", got)
	}
	if len(got.Sections) != 2 || got.Sections[1].Slug != "results" {
		t.Fatalf("sections out of order: %+v", got.Sections)
	}
	if len(got.Citations) != 1 || got.Citations[0].DOI != "10.1/x" {
		t.Fatalf("citations: %+v", got.Citations)
	}
	list, err := l.ListManuscripts("s1")
	if err != nil || len(list) != 1 || list[0].ID != "ms1" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if _, err := l.GetManuscript("other", "ms1"); !errors.Is(err, ledger.ErrManuscriptNotFound) {
		t.Fatalf("cross-session read = %v, want not found", err)
	}
}

func TestManuscriptDraftIsImmutableAfterReview(t *testing.T) {
	l := openManuscriptLedger(t)
	if err := l.CreateManuscript(draft("ms1", "s1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := l.SetManuscriptReviewState("s1", "ms1", `{"recommendation":"REVISE"}`); err != nil {
		t.Fatalf("review state: %v", err)
	}
	edited := draft("ms1", "s1")
	edited.ContentHash = "hash-2"
	if err := l.UpdateManuscriptDraft(edited); !errors.Is(err, ledger.ErrManuscriptState) {
		t.Fatalf("edit after review = %v, want state refusal", err)
	}
}

func TestManuscriptSignoffBindsContentHash(t *testing.T) {
	l := openManuscriptLedger(t)
	if err := l.CreateManuscript(draft("ms1", "s1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	bad := ledger.ManuscriptSignoff{
		SignerPubKey: "pk", Signature: "sig", ContentHash: "other", Expiry: "2030-01-01T00:00:00Z",
	}
	if err := l.SetManuscriptSignoff("s1", "ms1", bad); !errors.Is(err, ledger.ErrManuscriptState) {
		t.Fatalf("mismatched signoff = %v, want state refusal", err)
	}
	ok := bad
	ok.ContentHash = "hash-1"
	if err := l.SetManuscriptSignoff("s1", "ms1", ok); err != nil {
		t.Fatalf("signoff: %v", err)
	}
	got, err := l.GetManuscriptSignoff("ms1")
	if err != nil || got == nil || got.ContentHash != "hash-1" {
		t.Fatalf("signoff read: %v %+v", err, got)
	}
	ms, _ := l.GetManuscript("s1", "ms1")
	if ms.State != ledger.ManuscriptStateSigned {
		t.Fatalf("state = %q, want signed", ms.State)
	}
}

func TestManuscriptArtifactsAreHashCheckedAndReplaced(t *testing.T) {
	l := openManuscriptLedger(t)
	if err := l.CreateManuscript(draft("ms1", "s1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	files := map[string][]byte{"main.pdf": []byte("%PDF-1.4 test"), "main.tex": []byte("\\documentclass{article}")}
	if err := l.ReplaceManuscriptArtifacts("s1", "ms1", "build-1", files); err != nil {
		t.Fatalf("artifacts: %v", err)
	}
	raw, sum, err := l.GetManuscriptArtifact("s1", "ms1", "main.pdf")
	if err != nil || !bytes.Equal(raw, files["main.pdf"]) || sum == "" {
		t.Fatalf("artifact read: %v %q", err, sum)
	}
	if _, _, err := l.GetManuscriptArtifact("s2", "ms1", "main.pdf"); !errors.Is(err, ledger.ErrManuscriptNotFound) {
		t.Fatalf("cross-session artifact = %v, want not found", err)
	}
	arts, err := l.ListManuscriptArtifacts("ms1")
	if err != nil || len(arts) != 2 {
		t.Fatalf("artifact list: %v %+v", err, arts)
	}
	// A rebuild replaces the set; it does not append.
	if err := l.ReplaceManuscriptArtifacts("s1", "ms1", "build-2", map[string][]byte{"main.pdf": []byte("%PDF-1.5")}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	arts, _ = l.ListManuscriptArtifacts("ms1")
	if len(arts) != 1 {
		t.Fatalf("artifact set not replaced: %+v", arts)
	}
}

func TestManuscriptAnnotationsLifecycle(t *testing.T) {
	l := openManuscriptLedger(t)
	if err := l.CreateManuscript(draft("ms1", "s1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	a := ledger.ManuscriptAnnotation{ID: "ann1", Author: "operator", Anchor: "results", Body: "Clarify the anchor."}
	if err := l.AddManuscriptAnnotation("s1", "ms1", a); err != nil {
		t.Fatalf("add: %v", err)
	}
	list, err := l.ListManuscriptAnnotations("s1", "ms1")
	if err != nil || len(list) != 1 || list[0].Status != ledger.AnnotationOpen {
		t.Fatalf("list: %v %+v", err, list)
	}
	if list[0].ContentHash != "hash-1" {
		t.Fatalf("annotation must bind the draft content hash: %+v", list[0])
	}
	open, err := l.CountOpenAnnotations("s1", "ms1")
	if err != nil || open != 1 {
		t.Fatalf("open count = %d (%v)", open, err)
	}
	if err := l.SetManuscriptAnnotationStatus("s1", "ms1", "ann1", ledger.AnnotationAddressed); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if open, _ := l.CountOpenAnnotations("s1", "ms1"); open != 0 {
		t.Fatalf("open after resolve = %d", open)
	}
	if err := l.SetManuscriptAnnotationStatus("s1", "ms1", "ann1", "bogus"); err == nil {
		t.Fatal("invalid status accepted")
	}
	if _, err := l.ListManuscriptAnnotations("other", "ms1"); !errors.Is(err, ledger.ErrManuscriptNotFound) {
		t.Fatalf("cross-session annotations = %v", err)
	}
}
