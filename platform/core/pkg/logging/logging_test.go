package logging

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestSetupJSONAndLevel(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	t.Setenv("KUBONA_LOG_FORMAT", "json")
	t.Setenv("KUBONA_LOG_LEVEL", "debug")
	Setup()
	slog.Debug("drill", "k", "v")
	_ = w.Close()
	out, _ := io.ReadAll(r)
	os.Stderr = old
	var line map[string]any
	if err := json.Unmarshal(out, &line); err != nil {
		t.Fatalf("log is not JSON: %v\n%s", err, out)
	}
	if line["msg"] != "drill" || line["k"] != "v" || line["level"] != "DEBUG" {
		t.Fatalf("unexpected structured fields: %v", line)
	}
}
