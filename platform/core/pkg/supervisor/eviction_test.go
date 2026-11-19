package supervisor

import (
	"testing"
	"time"
)

func TestEvictTerminalLoopsKeepsLiveOnes(t *testing.T) {
	old := time.Now().UTC().Add(-120 * 24 * time.Hour)
	s := &Supervisor{
		loops: map[string]*ManagedLoop{
			"done":  {ID: "done", Status: LoopCompleted, UpdatedAt: old},
			"live":  {ID: "live", Status: LoopRunning, UpdatedAt: old},
			"fresh": {ID: "fresh", Status: LoopCompleted, UpdatedAt: time.Now().UTC()},
		},
		snapshots: map[string][]*LoopSnapshot{"done": {{SnapshotID: "s"}}},
	}
	if n := s.EvictTerminalLoops(time.Now().UTC().Add(-90 * 24 * time.Hour)); n != 1 {
		t.Fatalf("evicted=%d want 1", n)
	}
	if _, ok := s.loops["done"]; ok {
		t.Fatal("terminal loop was not evicted")
	}
	if _, ok := s.snapshots["done"]; ok {
		t.Fatal("evicted loop snapshots remain")
	}
	if _, ok := s.loops["live"]; !ok {
		t.Fatal("live loop must survive")
	}
	if _, ok := s.loops["fresh"]; !ok {
		t.Fatal("recent terminal loop must survive")
	}
}
