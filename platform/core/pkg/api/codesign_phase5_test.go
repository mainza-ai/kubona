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

	// Non-promote subpath -> 404
	req2 := httptest.NewRequest(http.MethodPost, "/api/codesign/packs/numerics/other",
		bytes.NewBufferString(`{"anchor_ref":"a","proof_ref":"b"}`))
	rec2 := httptest.NewRecorder()
	srv.handleCodesignPackPromote(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown subpath, got %d: %s", rec2.Code, rec2.Body.String())
	}
}
