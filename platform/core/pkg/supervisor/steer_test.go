package supervisor_test

// C10 pins: a steer is queued for a live run and applied exactly once at the
// next step boundary, with an event and a lane part.

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

func steerTestSupervisor(t *testing.T) (*supervisor.Supervisor, *ledger.Ledger) {
	t.Helper()
	dbPath := t.TempDir() + "/steer.db"
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	sup := supervisor.NewSupervisor(l)
	t.Cleanup(func() { sup.Close() })
	return sup, l
}

func countSteerEvents(t *testing.T, l *ledger.Ledger, sid string) int {
	t.Helper()
	events, err := l.GetRecentEvents(sid, 200)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, event := range events {
		if event.EventType == "loop_steered" {
			n++
		}
	}
	return n
}

func TestSteerAppliedAtNextStepBoundary(t *testing.T) {
	sup, l := steerTestSupervisor(t)
	loop, err := sup.RegisterLoop("sess-steer", 5)
	if err != nil {
		t.Fatal(err)
	}
	steer, err := sup.Steer(loop.ID, "  prefer shorter ansatz circuits  ")
	if err != nil {
		t.Fatal(err)
	}
	if steer.Status != "pending" || steer.Text != "prefer shorter ansatz circuits" {
		t.Fatalf("steer must queue trimmed text: %+v", steer)
	}
	// Nothing is applied before a step boundary.
	if n := countSteerEvents(t, l, "sess-steer"); n != 0 {
		t.Fatalf("steer applied before a step: %d", n)
	}
	if _, err := sup.Step(loop.ID, 0.05); err != nil {
		t.Fatal(err)
	}
	if n := countSteerEvents(t, l, "sess-steer"); n != 1 {
		t.Fatalf("steer must apply exactly once at the boundary, got %d", n)
	}
	parts, err := sup.LoopParts(loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, part := range parts {
		if part.Title == "Steering applied" {
			found = true
			if part.Goal != "prefer shorter ansatz circuits" || part.State != "done" {
				t.Fatalf("steer part malformed: %+v", part)
			}
		}
	}
	if !found {
		t.Fatalf("steer must appear in the run lane: %+v", parts)
	}
	// A second step must not re-apply it.
	if _, err := sup.Step(loop.ID, 0.04); err != nil {
		t.Fatal(err)
	}
	if n := countSteerEvents(t, l, "sess-steer"); n != 1 {
		t.Fatalf("steer re-applied on a later step: %d", n)
	}
}

func TestSteerRefusesInvalidTargets(t *testing.T) {
	sup, _ := steerTestSupervisor(t)
	loop, err := sup.RegisterLoop("sess-steer-refuse", 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Steer("loop-missing", "x"); err == nil {
		t.Fatal("unknown loop must refuse")
	}
	if _, err := sup.Steer(loop.ID, "   "); err == nil {
		t.Fatal("blank steer must refuse")
	}
	long := make([]rune, 501)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := sup.Steer(loop.ID, string(long)); err == nil {
		t.Fatal("overlong steer must refuse")
	}
	if err := sup.Pause(loop.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Steer(loop.ID, "too late"); err == nil {
		t.Fatal("paused loop must refuse steering: nothing would consume it")
	}
}

func TestMultipleSteersApplyInOneBoundary(t *testing.T) {
	sup, l := steerTestSupervisor(t)
	loop, err := sup.RegisterLoop("sess-steer-multi", 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Steer(loop.ID, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Steer(loop.ID, "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Step(loop.ID, 0.05); err != nil {
		t.Fatal(err)
	}
	if n := countSteerEvents(t, l, "sess-steer-multi"); n != 2 {
		t.Fatalf("both steers apply at one boundary, got %d", n)
	}
}
