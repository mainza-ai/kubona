package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestSandboxExecution(t *testing.T) {
	runner := NewSandboxRunner("python:3.12-slim", 5*time.Second)

	code := `
print("Hello from Kubona Sandbox")
`
	res, err := runner.ExecutePython(code)
	if err != nil {
		t.Fatalf("failed to execute sandbox code: %v", err)
	}

	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, output: %s", res.ExitCode, res.Stdout)
	}

	if !strings.Contains(res.Stdout, "Hello from Kubona Sandbox") {
		t.Fatalf("unexpected stdout: %s", res.Stdout)
	}
}
