package api

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestOperationDispositionVocabulary(t *testing.T) {
	cases := []struct {
		name string
		rec  ledger.IdempotencyRecord
		disp string
		why  string
	}{
		{"admitted", ledger.IdempotencyRecord{Status: ledger.IdemStatusInProgress}, "admitted", "queued"},
		{"applied", ledger.IdempotencyRecord{Status: ledger.IdemStatusComplete, StatusCode: 201, ReceiptBody: `{"id":"j"}`}, "applied", "completed"},
		{"refused", ledger.IdempotencyRecord{Status: ledger.IdemStatusComplete, StatusCode: 409, ReceiptBody: `{"error":"no"}`}, "refused", "handler_refused"},
		{"no_op", ledger.IdempotencyRecord{Status: ledger.IdemStatusComplete, StatusCode: 200, ReceiptBody: `{"unchanged":true}`}, "no_op", "unchanged"},
		{"unknown", ledger.IdempotencyRecord{Status: ledger.IdemStatusComplete, StatusCode: 201, ReceiptBody: `{"job":{"status":"outcome_unknown"}}`}, "outcome_unknown", "vendor_unconfirmed"},
	}
	for _, tc := range cases {
		disp, why := operationDisposition(tc.rec)
		if disp != tc.disp || why != tc.why {
			t.Fatalf("%s: got %s/%s", tc.name, disp, why)
		}
	}
}
