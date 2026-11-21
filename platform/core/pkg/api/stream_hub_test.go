package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestStreamHubFansOutOnePollToManyClients(t *testing.T) {
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	h := newStreamHub(l)
	a, unsubA := h.subscribe("sess-a", 0)
	defer unsubA()
	b, unsubB := h.subscribe("sess-a", 0)
	defer unsubB()

	if err := l.AppendEvent("sess-a", "annotation", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	for name, sub := range map[string]*streamSub{"a": a, "b": b} {
		select {
		case f, ok := <-sub.ch:
			if !ok {
				t.Fatalf("%s: channel closed", name)
			}
			if f.ID == 0 || len(f.Data) == 0 {
				t.Fatalf("%s: bad frame %+v", name, f)
			}
		case <-time.After(4 * time.Second):
			t.Fatalf("%s: no frame within 4s", name)
		}
	}
	// One query per session tick, not one per subscriber. Allow the
	// immediate poll plus the next 2s tick.
	time.Sleep(2500 * time.Millisecond)
	if got := h.pollCount(); got > 4 {
		t.Fatalf("polls=%d for two subscribers; hub is not coalescing", got)
	}
}

func TestStreamHubResumesFromCursor(t *testing.T) {
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "cursor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.AppendEvent("sess-c", "annotation", map[string]string{"n": "old"}); err != nil {
		t.Fatal(err)
	}
	old, err := l.GetRecentEvents("sess-c", 1)
	if err != nil || len(old) != 1 {
		t.Fatalf("recent: %v %v", old, err)
	}
	h := newStreamHub(l)
	sub, unsub := h.subscribe("sess-c", old[0].ID)
	defer unsub()
	if err := l.AppendEvent("sess-c", "annotation", map[string]string{"n": "new"}); err != nil {
		t.Fatal(err)
	}
	select {
	case f, ok := <-sub.ch:
		if !ok || f.ID <= old[0].ID {
			t.Fatalf("frame=%+v ok=%v; expected only the new event", f, ok)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no frame after cursor")
	}
}

func TestStreamHubDropsSlowClientWithoutPanicking(t *testing.T) {
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "slow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for i := 0; i < 160; i++ {
		if err := l.AppendEvent("sess-slow", "annotation", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	h := newStreamHub(l)
	sub, unsub := h.subscribe("sess-slow", 0)
	defer unsub()
	// Wait for the initial poll to complete while never draining sub.ch:
	// the hub must fill 128 frames and close/drop sub cleanly on frame 129
	// instead of panicking on a send to the closed channel.
	waitPoll := time.Now().Add(4 * time.Second)
	for h.pollCount() == 0 && time.Now().Before(waitPoll) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	deadline := time.After(4 * time.Second)
	drained := 0
	for {
		select {
		case _, ok := <-sub.ch:
			if !ok {
				if drained != 128 {
					t.Fatalf("expected 128 buffered frames before drop, got %d", drained)
				}
				return
			}
			drained++
		case <-deadline:
			t.Fatalf("slow client was not dropped (drained=%d)", drained)
		}
	}
}
