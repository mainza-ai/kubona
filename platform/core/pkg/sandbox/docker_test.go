package sandbox

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSandboxExecution(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = 10 * time.Second
	cfg.AllowHostFallback = true // Allowed for test resilience across CI environments
	runner := NewSandboxRunnerWithConfig(cfg)

	code := `
print("Hello from Kubona Hardened Sandbox")
`
	res, err := runner.ExecutePython(code)
	if err != nil {
		t.Fatalf("failed to execute sandbox code: %v", err)
	}

	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr: %s", res.ExitCode, res.Stderr)
	}

	if !strings.Contains(res.Stdout, "Hello from Kubona Hardened Sandbox") {
		t.Fatalf("unexpected stdout: %s", res.Stdout)
	}

	if res.Sandboxed && !strings.Contains(res.IsolationTag, "docker") {
		t.Fatalf("expected docker isolation tag when sandboxed, got: %s", res.IsolationTag)
	}
}

func TestSandboxDualMountHarness(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = 10 * time.Second
	cfg.AllowHostFallback = true
	runner := NewSandboxRunnerWithConfig(cfg)

	benchmarkScript := `
import os
import sys

# Read mutable artifact from /workspace
with open("ansatz.py", "r") as f:
    content = f.read()

print(f"EVAL_SUCCESS: {len(content)} bytes")
`
	mutableContent := `def ansatz(): pass`

	res, err := runner.ExecuteHarness(benchmarkScript, "ansatz.py", mutableContent)
	if err != nil {
		t.Fatalf("failed to execute harness: %v", err)
	}

	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr: %s", res.ExitCode, res.Stderr)
	}

	if !strings.Contains(res.Stdout, "EVAL_SUCCESS: 18 bytes") {
		t.Fatalf("unexpected stdout: %s", res.Stdout)
	}
}

func TestSandboxStrictRefusalWithoutFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Image = "non-existent-image-xyz-12345"
	cfg.AllowHostFallback = false
	runner := NewSandboxRunnerWithConfig(cfg)

	// Since image doesn't exist, if docker cannot inspect it and fallback is false:
	if !runner.IsDockerAvailable() {
		_, err := runner.ExecutePython(`print("should fail")`)
		if err == nil {
			t.Fatalf("expected error when docker unavailable and AllowHostFallback=false")
		}
	}
}

func TestSandboxProductionFailClosed(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_DEV_MODE", "")

	cfg := DefaultConfig()
	cfg.AllowHostFallback = true // Attempt to request host fallback
	runner := NewSandboxRunnerWithConfig(cfg)

	// In production, AllowHostFallback must be overridden to false
	if runner.Config().AllowHostFallback {
		t.Fatalf("expected AllowHostFallback to be false in production")
	}

	// Manually force useDocker=false to test fail-closed behavior
	runner.useDocker = false
	_, err := runner.ExecuteHarness("print('eval')", "script.py", "pass")
	if err == nil {
		t.Fatalf("expected error when docker is unavailable in production")
	}
	if !strings.Contains(err.Error(), ErrDockerNotAvailable.Error()) {
		t.Fatalf("expected ErrDockerNotAvailable, got: %v", err)
	}
}

func TestSandboxDevModeInProductionProhibited(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_DEV_MODE", "1")

	cfg := DefaultConfig()
	runner := NewSandboxRunnerWithConfig(cfg)
	runner.useDocker = false

	_, err := runner.ExecuteHarness("print('eval')", "script.py", "pass")
	if err == nil {
		t.Fatalf("expected error when KUBONA_DEV_MODE=1 in production")
	}
	if !errors.Is(err, ErrDevModeInProduction) {
		t.Fatalf("expected ErrDevModeInProduction, got: %v", err)
	}
}

func TestSandboxDevModeFallback(t *testing.T) {
	t.Setenv("KUBONA_ENV", "development")
	t.Setenv("KUBONA_DEV_MODE", "1")

	cfg := DefaultConfig()
	cfg.AllowHostFallback = false // Config says no, but dev mode overrides
	runner := NewSandboxRunnerWithConfig(cfg)
	runner.useDocker = false

	res, err := runner.ExecutePython("print('dev-mode-test')")
	if err != nil {
		t.Fatalf("unexpected error in dev mode fallback: %v", err)
	}
	if res.Sandboxed {
		t.Fatalf("expected Sandboxed=false in dev mode fallback")
	}
	if !strings.Contains(res.IsolationTag, "UNSANDBOXED - DEV MODE") {
		t.Fatalf("expected DEV MODE tag, got: %s", res.IsolationTag)
	}
	if !strings.Contains(res.Stdout, "dev-mode-test") {
		t.Fatalf("expected output, got: %s", res.Stdout)
	}
}

func TestHarnessHostFallbackReadsArtifactFromWorkspaceCWD(t *testing.T) {
	t.Setenv("KUBONA_ENV", "")
	t.Setenv("KUBONA_DEV_MODE", "")
	cfg := DefaultConfig()
	cfg.AllowHostFallback = true
	cfg.Timeout = 20 * time.Second
	runner := NewSandboxRunnerWithConfig(cfg)
	runner.useDocker = false

	script := `import json
print(json.dumps({"text": open("artifact.txt").read()}))`
	res, err := runner.ExecuteHarness(script, "artifact.txt", "relative-path-ok")
	if err != nil {
		t.Fatalf("host fallback: %v", err)
	}
	if !strings.Contains(res.Stdout, "relative-path-ok") {
		t.Fatalf("artifact must be readable from the workspace cwd, got: %q (stderr %q)", res.Stdout, res.Stderr)
	}
}
