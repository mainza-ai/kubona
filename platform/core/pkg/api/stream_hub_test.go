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
