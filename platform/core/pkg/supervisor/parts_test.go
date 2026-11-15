package supervisor_test

// C1 store tests: parts are recorded, validated, and read back per loop.
// Nothing is inferred from a session's other events.

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

func TestLoopPartsRecordedAndScoped(t *testing.T) {
	dbPath := t.TempDir() + "/parts.db"
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sup := supervisor.NewSupervisor(l)
	defer sup.Close()

	loopA, err := sup.RegisterLoop("sess-parts", 5)
	if err != nil {
		t.Fatal(err)
	}
	loopB, err := sup.RegisterLoop("sess-parts", 5)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sup.Step(loopA.ID, 0.05); err != nil {
		t.Fatal(err)
	}
	if err := sup.Pause(loopA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Step(loopB.ID, 0.02); err != nil {
		t.Fatal(err)
	}

	parts, err := sup.LoopParts(loopA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("loop A must have 2 parts (step + pause), got %d: %+v", len(parts), parts)
	}
	if parts[0].Helper != "lead" || parts[0].State != "done" || parts[0].Title != "Optimizer step 1" {
		t.Fatalf("step part malformed: %+v", parts[0])
	}
	if parts[0].Now == "" || parts[0].EndedMS == 0 {
		t.Fatalf("step part must record its metric and end time: %+v", parts[0])
	}
	if parts[1].Title != "Paused by operator" || parts[1].State != "waiting" {
		t.Fatalf("pause part malformed: %+v", parts[1])
	}

	other, err := sup.LoopParts(loopB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].LoopID != loopB.ID {
		t.Fatalf("parts must be scoped by loop id, got %+v", other)
	}
}

func TestRecordPartRefusesUnknownVocabulary(t *testing.T) {
	dbPath := t.TempDir() + "/parts-refuse.db"
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sup := supervisor.NewSupervisor(l)
	defer sup.Close()
	loop, err := sup.RegisterLoop("sess-parts-refuse", 3)
	if err != nil {
		t.Fatal(err)
	}

	cases := []supervisor.LoopPart{
		{LoopID: loop.ID, Helper: "lead", Title: "t", State: "teleported"},
		{LoopID: loop.ID, Helper: "wizard", Title: "t", State: "running"},
		{LoopID: loop.ID, Helper: "lead", Title: "", State: "running"},
		{LoopID: "missing", Helper: "lead", Title: "t", State: "running"},
		{LoopID: "", Helper: "lead", Title: "t", State: "running"},
	}
	for i, part := range cases {
		if _, err := sup.RecordPart("sess-parts-refuse", part); err == nil {
			t.Fatalf("case %d must refuse: %+v", i, part)
		}
	}
	if _, err := sup.LoopParts("missing"); err == nil {
		t.Fatal("parts for an unknown loop must refuse")
	}
}
