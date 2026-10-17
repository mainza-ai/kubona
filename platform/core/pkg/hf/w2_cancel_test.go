package hf_test

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/hf"
)

// W2.20: cancel is terminal-guarded — a second cancel (or a cancel after
// completion) refuses instead of re-settling the state a racing worker set.
func TestW220CancelTerminalRefuses(t *testing.T) {
	c := hf.NewHubClient(t.TempDir())
	task, err := c.StartDownload("Qwen/Qwen2.5-Coder-7B-Instruct-GGUF", "model.gguf")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.CancelDownload(task.TaskID); err != nil {
		t.Fatalf("first cancel must succeed, got %v", err)
	}
	if err := c.CancelDownload(task.TaskID); err == nil {
		t.Error("second cancel on a terminal task must refuse")
	}
	if err := c.CancelDownload("dl-does-not-exist"); err == nil {
		t.Error("cancel of unknown task must refuse")
	}
}
