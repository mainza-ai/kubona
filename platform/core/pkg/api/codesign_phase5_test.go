package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCodesignPackPromoteRefusesMissingRefs(t *testing.T) {
	srv := &Server{}
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

	// Non-promote subpath -> 404
	req2 := httptest.NewRequest(http.MethodPost, "/api/codesign/packs/numerics/other",
		bytes.NewBufferString(`{"anchor_ref":"a","proof_ref":"b"}`))
	rec2 := httptest.NewRecorder()
	srv.handleCodesignPackPromote(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown subpath, got %d: %s", rec2.Code, rec2.Body.String())
	}
}
