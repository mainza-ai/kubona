package hf

// Regression test for the 2026-10-29 CI data race: runDownload read the
// mutable HFResolveBase global, so reassigning it (as every httptest-based
// test does in setup) could divert an already-spawned worker to the wrong
// origin. The worker must use the origin snapshotted at construction.
//
// Deterministic by construction: the global is flipped to an unroutable
// address AFTER the client is built but BEFORE the download starts. A
// worker reading the global would fail with connection refused; a worker
// using the snapshot completes against the live server.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestDownloadUsesSnapshotNotGlobal(t *testing.T) {
	payload := []byte("snapshot-not-global")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	oldBase := HFResolveBase
	HFResolveBase = srv.URL
	client := NewHubClient(t.TempDir())
	// Flip AFTER construction, BEFORE dispatch: any worker reading the
	// global instead of the snapshot dials a dead address and fails.
	HFResolveBase = "http://127.0.0.1:1"
	defer func() { HFResolveBase = oldBase }()

	task, err := client.StartDownload("Qwen/test-snap", "tiny.gguf")
	if err != nil {
		t.Fatalf("StartDownload failed: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		settled, err := client.GetDownload(task.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Status != "downloading" && settled.Status != "pending" {
			if settled.Status != "completed" {
				t.Fatalf("snapshot-violating worker failed: %+v", settled)
			}
			if settled.BytesDownloaded != int64(len(payload)) {
				t.Fatalf("byte count mismatch: got %d want %d", settled.BytesDownloaded, len(payload))
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("download never settled: %+v", settled)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
