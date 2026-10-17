package autoresearch

// I3 engine tests: measured spend accumulates in the mirrors, recorder
// failure pauses, overrun pauses. The server persists (tested api-side).

import (
	"errors"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

func spendTestEngine(t *testing.T) *LoopEngine {
	t.Helper()
	reg := NewHarnessRegistry()
	harness, err := reg.Get("ansatz-search")
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 5
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)
	return NewLoopEngine("loop-spend-1", "sess-spend", harness, runner, nil, "", 10)
}

func TestRecordSpendMirrors(t *testing.T) {
	e := spendTestEngine(t)
	var calls int
	e.SetSpendRecorder(func(role string, p, c int) (float64, error) {
		calls++
		return 0.5, nil
	})
	e.SetSpendReservation(3.0)
	e.recordSpend("coder", 100, 200)
	e.recordSpend("coder", 100, 200)
	if calls != 2 {
		t.Fatalf("recorder must fire per call, got %d", calls)
	}
	st := e.State()
	if st.ReservedAmount != 3.0 || st.ConsumedAmount != 1.0 {
		t.Fatalf("mirrors must track reservation + drawdown: %+v", st)
	}
	if st.Status == StatusPaused {
		t.Fatal("covered spend must not pause")
	}
}

func TestRecordSpendRecorderErrorPauses(t *testing.T) {
	e := spendTestEngine(t)
	e.SetSpendRecorder(func(role string, p, c int) (float64, error) {
		return 0, errors.New("books unreachable")
	})
	e.SetSpendReservation(3.0)
	e.recordSpend("coder", 100, 200)
	if e.State().Status == StatusRunning {
		t.Fatal("unrecordable spend must pause the loop")
	}
}

func TestRecordSpendOverrunPauses(t *testing.T) {
	e := spendTestEngine(t)
	e.SetSpendRecorder(func(role string, p, c int) (float64, error) {
		return 5.0, nil
	})
	e.SetSpendReservation(3.0)
	e.recordSpend("coder", 100000, 100000)
	if e.State().Status == StatusRunning {
		t.Fatal("overrun past the reservation must pause the loop")
	}
}

func TestRecordSpendNilRecorderSkips(t *testing.T) {
	e := spendTestEngine(t)
	e.SetSpendReservation(3.0)
	e.recordSpend("coder", 100, 200) // no recorder: books untouched, no pause
	if st := e.State(); st.ConsumedAmount != 0 || st.Status == StatusPaused {
		t.Fatalf("nil recorder must skip silently: %+v", st)
	}
}
