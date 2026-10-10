package hf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestHubClientSearch(t *testing.T) {
	client := NewHubClient(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := client.SearchModels(ctx, "qwen", "gguf", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected at least 1 model in search results")
	}

	foundQwen := false
	for _, m := range models {
		if m.ID != "" && m.Author != "" {
			foundQwen = true
			break
		}
	}
	if !foundQwen {
		t.Errorf("expected valid model metadata")
	}
}

func TestHubClientModelDetails(t *testing.T) {
	client := NewHubClient(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	detail, err := client.GetModelDetails(ctx, "Qwen/Qwen2.5-Coder-32B-Instruct-GGUF")
	if err != nil {
		t.Fatalf("failed to get model details: %v", err)
	}
	if detail.ID == "" || len(detail.Quantizations) == 0 {
		t.Fatalf("expected model details with quantizations: %+v", detail)
	}
}

func TestHubClientDownloads(t *testing.T) {
	client := NewHubClient(t.TempDir())

	task, err := client.StartDownload("Qwen/Qwen2.5-Coder-7B-Instruct-GGUF", "model.gguf")
	if err != nil {
		t.Fatalf("failed to start download: %v", err)
	}
	if task.TaskID == "" || task.Status != "downloading" {
		t.Fatalf("unexpected task state: %+v", task)
	}

	downloads := client.ListDownloads()
	if len(downloads) != 1 {
		t.Fatalf("expected 1 task in list, got %d", len(downloads))
	}

	err = client.CancelDownload(task.TaskID)
	if err != nil {
		t.Fatalf("failed to cancel download: %v", err)
	}
	// Re-fetch: the StartDownload snapshot predates cancellation.
	current, err := client.GetDownload(task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %s", current.Status)
	}
}

func TestHubClientRealDownloadBytes(t *testing.T) {
	payload := make([]byte, 256*1024)
	for i := range payload {
		payload[i] = byte(i * 31 % 251)
	}
	wantSHA := sha256Hex(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Qwen/test-tiny/resolve/main/tiny.gguf" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	oldBase := HFResolveBase
	HFResolveBase = srv.URL
	defer func() { HFResolveBase = oldBase }()

	client := NewHubClient(t.TempDir())
	task, err := client.StartDownload("Qwen/test-tiny", "tiny.gguf")
	if err != nil {
		t.Fatalf("StartDownload failed: %v", err)
	}
	// Poll snapshots: the StartDownload return predates worker writes.
	var settled *HFDownloadTask
	deadline := time.Now().Add(15 * time.Second)
	for {
		var err error
		settled, err = client.GetDownload(task.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Status != "downloading" && settled.Status != "pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("download never settled: %+v", settled)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if settled.Status != "completed" {
		t.Fatalf("expected completed, got %+v", settled)
	}
	if settled.SHA256 != wantSHA {
		t.Fatalf("SHA mismatch: got %s want %s", settled.SHA256, wantSHA)
	}
	if settled.BytesDownloaded != int64(len(payload)) {
		t.Fatalf("byte count mismatch: got %d want %d", settled.BytesDownloaded, len(payload))
	}
	if got, err := os.ReadFile(settled.LocalPath); err != nil || len(got) != len(payload) {
		t.Fatalf("local file missing or truncated: %v", err)
	}
}

func TestHubClientDownload404FailsHonestly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	oldBase := HFResolveBase
	HFResolveBase = srv.URL
	defer func() { HFResolveBase = oldBase }()

	client := NewHubClient(t.TempDir())
	task, err := client.StartDownload("Qwen/test-tiny", "missing.gguf")
	if err != nil {
		t.Fatalf("StartDownload failed: %v", err)
	}
	var settled *HFDownloadTask
	deadline := time.Now().Add(15 * time.Second)
	for {
		var err error
		settled, err = client.GetDownload(task.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Status != "downloading" && settled.Status != "pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("download never settled: %+v", settled)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if settled.Status != "failed" || settled.Error == "" {
		t.Fatalf("404 must fail honestly with an error: %+v", settled)
	}
	if _, statErr := os.Stat(settled.LocalPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed download must not leave a weight file behind")
	}
}

func TestHubClientRejectsTraversal(t *testing.T) {
	client := NewHubClient(t.TempDir())
	for _, tc := range [][2]string{
		{"Qwen/x", "../../evil.gguf"},
		{"../x", "f.gguf"},
		{"", "f.gguf"},
		{"Qwen/x", ""},
		{"Qwen/x", "sub/dir.gguf"},
	} {
		if _, err := client.StartDownload(tc[0], tc[1]); err == nil {
			t.Fatalf("expected rejection for %q / %q", tc[0], tc[1])
		}
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
