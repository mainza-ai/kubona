package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestCodesignPackPromoteRefusesMissingAndUnresolvedRefs(t *testing.T) {
	led, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "kubona.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer led.Close()

	srv := &Server{ledger: led}
	t.Setenv("KUBONA_API_TOKEN", "test-tok")

	// Missing proof_ref -> 400
	req := httptest.NewRequest(http.MethodPost, "/api/codesign/packs/numerics/promote",
		bytes.NewBufferString(`{"anchor_ref":"ledger:anchor:1"}`))
	req.Header.Set("Authorization", "Bearer test-tok")
	rec := httptest.NewRecorder()
	srv.handleCodesignPackPromote(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing proof_ref, got %d: %s", rec.Code, rec.Body.String())
	}

	// C4 fix: arbitrary non-existent ledger refs -> 400 unresolved
	reqUnres := httptest.NewRequest(http.MethodPost, "/api/codesign/packs/numerics/promote",
		bytes.NewBufferString(`{"anchor_ref":"ledger://test/anchor","proof_ref":"ledger://test/proof"}`))
	reqUnres.Header.Set("Authorization", "Bearer test-tok")
	recUnres := httptest.NewRecorder()
	srv.handleCodesignPackPromote(recUnres, reqUnres)
	if recUnres.Code != http.StatusBadRequest || !strings.Contains(recUnres.Body.String(), "unresolved anchor_ref") {
		t.Fatalf("C4 violated: arbitrary ref should 400 unresolved, got %d: %s", recUnres.Code, recUnres.Body.String())
	}

	// R3 fix: an invalid codesign_run (valid=false) exists in ledger (can anchor) but MUST NOT pass ResolveProofRef
	if err := led.SaveCodesignRun(ledger.CodesignRunRecord{
		RunID: "cs_invalid_run", SessionID: "s1", SpecKind: "and2", Valid: false,
	}); err != nil {
		t.Fatalf("save invalid run: %v", err)
	}
	reqBadProof := httptest.NewRequest(http.MethodPost, "/api/codesign/packs/numerics/promote",
		bytes.NewBufferString(`{"anchor_ref":"cs_invalid_run","proof_ref":"cs_invalid_run"}`))
	reqBadProof.Header.Set("Authorization", "Bearer test-tok")
	recBadProof := httptest.NewRecorder()
	srv.handleCodesignPackPromote(recBadProof, reqBadProof)
	if recBadProof.Code != http.StatusBadRequest || !strings.Contains(recBadProof.Body.String(), "unresolved or non-verified proof_ref") {
		t.Fatalf("R3 violated: invalid run accepted as proof_ref, got %d: %s", recBadProof.Code, recBadProof.Body.String())
	}

	// Non-promote subpath -> 404
	req2 := httptest.NewRequest(http.MethodPost, "/api/codesign/packs/numerics/other",
		bytes.NewBufferString(`{"anchor_ref":"a","proof_ref":"b"}`))
	rec2 := httptest.NewRecorder()
	srv.handleCodesignPackPromote(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown subpath, got %d: %s", rec2.Code, rec2.Body.String())
	}
}
