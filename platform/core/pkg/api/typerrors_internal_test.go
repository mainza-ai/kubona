package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteAPIErrorCarriesTypedNeed(t *testing.T) {
	cases := map[string]string{
		"ceiling_exceeded":  "budget_exceeded",
		"approval_required": "approval",
		"anchor_missing":    "anchor_missing",
	}
	for code, wantKind := range cases {
		rec := httptest.NewRecorder()
		WriteAPIError(rec, code, "refused", nil)
		var body struct {
			Details struct {
				Need struct {
					Kind string `json:"kind"`
				} `json:"need"`
			} `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s body: %v", code, err)
		}
		if body.Details.Need.Kind != wantKind {
			t.Fatalf("%s need kind = %q, want %q", code, body.Details.Need.Kind, wantKind)
		}
	}
}
