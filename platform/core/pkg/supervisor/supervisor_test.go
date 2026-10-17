package supervisor_test

import (
	"os"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

func TestReconciliationGate(t *testing.T) {
	gate := supervisor.NewReconciliationGate(100 * time.Millisecond)

	if !gate.Acquire("worker-1") {
		t.Fatal("expected worker-1 to acquire gate")
	}

	if gate.Acquire("worker-2") {
		t.Fatal("worker-2 should not acquire active gate")
	}

	gate.Release("worker-1")
	if !gate.Acquire("worker-2") {
		t.Fatal("worker-2 should acquire released gate")
	}
	gate.Release("worker-2")
}

func TestLoopSupervisorAndWatchdogTrigger(t *testing.T) {
	dbPath := "test_sup.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	sup := supervisor.NewSupervisor(l)
	defer sup.Close()

	loop, err := sup.RegisterLoop("sess-abc", 5)
	if err != nil {
		t.Fatalf("failed to register loop: %v", err)
	}

	// Normal step
	status, err := sup.Step(loop.ID, 0.05)
	if err != nil || status != "running" {
		t.Fatalf("expected running, got %s, err %v", status, err)
	}

	// Checkpoint snapshot
	snap, err := sup.SaveSnapshot(loop.ID, []float64{0.1, 0.2, 0.3}, -1.135)
	if err != nil {
		t.Fatalf("failed to save snapshot: %v", err)
	}
	if snap.Metric != -1.135 {
		t.Fatalf("unexpected snapshot metric: %v", snap.Metric)
	}

	// Barren plateau trigger (Var(grad) < 1e-6)
	status, err = sup.Step(loop.ID, 1e-8)
	if err != nil || status != "diverged" {
		t.Fatalf("expected diverged, got %s, err %v", status, err)
	}

	// Rollback to snapshot
	restored, err := sup.RollbackToSnapshot(loop.ID)
	if err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}
	if restored.SnapshotID != snap.SnapshotID {
		t.Fatalf("expected snapshot %s, got %s", snap.SnapshotID, restored.SnapshotID)
	}

	// Re-scope loop
	err = sup.RescopeLoop(loop.ID, 10)
	if err != nil {
		t.Fatalf("failed to rescope: %v", err)
	}
}

func TestLocalIPv4PinGuard(t *testing.T) {
	if !supervisor.IsLocalIPv4("127.0.0.1") {
		t.Fatal("expected 127.0.0.1 to be local")
	}
	if !supervisor.IsLocalIPv4("localhost") {
		t.Fatal("expected localhost to be local")
	}
	if supervisor.IsLocalIPv4("192.168.1.100") {
		t.Fatal("expected private LAN IP not to be loopback")
	}
	if supervisor.IsLocalIPv4("8.8.8.8") {
		t.Fatal("expected public IP not to be loopback")
	}
}

// W0.21: terminal states are immutable — killed/completed never revive,
// every transition persists to the loops table, and ListLoops returns
// copies.
func TestLoopTerminalMachine(t *testing.T) {
	dbPath := "test_sup_w021.db"
	defer os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer l.Close()
	sup := supervisor.NewSupervisor(l)
	defer sup.Close()

	loop, err := sup.RegisterLoop("sess-w", 5)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// Ghost + non-positive bound refuse.
	if _, err := sup.RegisterLoop("", 5); err == nil {
		t.Fatalf("ghost loop must refuse")
	}
	if _, err := sup.RegisterLoop("sess-w", 0); err == nil {
		t.Fatalf("zero maxSteps must refuse")
	}
	// Pause then resume works; kill is terminal.
	if err := sup.Pause(loop.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := sup.Resume(loop.ID); err != nil {
		t.Fatalf("resume from paused: %v", err)
	}
	if err := sup.Kill(loop.ID); err != nil {
		t.Fatalf("kill: %v", err)
	}
	for _, op := range []func(string) error{sup.Resume, sup.Pause} {
		if err := op(loop.ID); err == nil {
			t.Fatalf("terminal loop must refuse further transitions")
		}
	}
	// Kill is idempotent (same-state no-op success, not resurrection).
	if err := sup.Kill(loop.ID); err != nil {
		t.Fatalf("kill on killed must succeed idempotently: %v", err)
	}
	if _, err := sup.RollbackToSnapshot(loop.ID); err == nil {
		t.Fatalf("rollback of terminal history must refuse")
	}
	// Persisted: the table agrees with memory.
	var status string
	if err := l.DB().QueryRow(`SELECT status FROM loops WHERE id = ?`, loop.ID).Scan(&status); err != nil || status != "killed" {
		t.Fatalf("kill must persist, got %q %v", status, err)
	}
	// Copies, not live pointers.
	got := sup.ListLoops()
	if len(got) == 0 {
		t.Fatalf("expected loops listed")
	}
	got[0].Status = "hacked"
	if sup.ListLoops()[0].Status != "killed" {
		t.Fatalf("ListLoops must return copies")
	}
	// Host:port splits before the pin check.
	if !supervisor.IsLocalIPv4("127.0.0.1:8787") {
		t.Fatalf("loopback with port must pin local")
	}
	if supervisor.IsLocalIPv4("93.184.216.34:8787") {
		t.Fatalf("public host with port must not pin local")
	}
}
