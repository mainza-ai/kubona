package ledger

// R6 store tests: record/list round-trip, per-role upsert, validation.

import (
	"testing"
)

func TestTranscriptScoresRoundTrip(t *testing.T) {
	l, err := OpenLedger(t.TempDir() + "/scores.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer l.Close()
	rows := []TranscriptScore{
		{SessionID: "s", ObligationID: "ob_1", Role: "ansatz-scout", Score: 0.75, Signals: map[string]bool{"specificity": true}},
		{SessionID: "s", ObligationID: "ob_1", Role: "epistemic-scribe", Score: 0.5, Signals: map[string]bool{}},
	}
	if err := l.RecordTranscriptScores(rows); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, err := l.ListTranscriptScores("ob_1")
	if err != nil || len(got) != 2 {
		t.Fatalf("want 2 scores, got %+v %v", got, err)
	}
	// Rescore replaces per role (never duplicates).
	rows[0].Score = 1.0
	if err := l.RecordTranscriptScores(rows[:1]); err != nil {
		t.Fatalf("rescore: %v", err)
	}
	got, _ = l.ListTranscriptScores("ob_1")
	if len(got) != 2 || got[0].Score != 1.0 && got[1].Score != 1.0 {
		t.Fatalf("rescore must replace: %+v", got)
	}
	// Unscored obligation reads empty (absence is not failure).
	empty, err := l.ListTranscriptScores("ob_missing")
	if err != nil || len(empty) != 0 {
		t.Fatalf("unscored must read empty: %+v %v", empty, err)
	}
	// Validation.
	if err := l.RecordTranscriptScores([]TranscriptScore{{SessionID: "s", Role: "r", Score: 2}}); err == nil {
		t.Fatal("out-of-range score must refuse")
	}
	if err := l.RecordTranscriptScores([]TranscriptScore{{Role: "r", Score: 0.5}}); err == nil {
		t.Fatal("sessionless score must refuse")
	}
}
