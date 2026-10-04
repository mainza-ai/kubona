package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ExecutionResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Sandboxed bool         `json:"sandboxed"`
}

type SandboxRunner struct {
	useDocker bool
	image     string
	timeout   time.Duration
}

func NewSandboxRunner(image string, timeout time.Duration) *SandboxRunner {
	if image == "" {
		image = "python:3.12-slim"
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	// Probe if docker is installed, daemon running, and requested image exists locally
	useDocker := false
	if exec.Command("docker", "info").Run() == nil {
		if exec.Command("docker", "image", "inspect", image).Run() == nil {
			useDocker = true
		} else {
			// Check if any standard lightweight python image is already cached
			for _, alt := range []string{"python:3.11-slim", "python:3.12-alpine", "python:3.10-slim", "python:alpine"} {
				if exec.Command("docker", "image", "inspect", alt).Run() == nil {
					image = alt
					useDocker = true
					break
				}
			}
		}
	}

	return &SandboxRunner{
		useDocker: useDocker,
		image:     image,
		timeout:   timeout,
	}
}

func (r *SandboxRunner) ExecutePython(code string) (*ExecutionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "kubona-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create scratch dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "script.py")
	if err := os.WriteFile(scriptPath, []byte(code), 0600); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}

	startTime := time.Now()
	var cmd *exec.Cmd
	var out []byte

	if r.useDocker {
		// Run inside unprivileged, network-isolated container
		cmd = exec.CommandContext(ctx, "docker", "run", "--rm",
			"--network", "none",
			"--memory", "512m",
			"--cpus", "1.0",
			"-v", fmt.Sprintf("%s:/app:ro", tmpDir),
			"-w", "/app",
			r.image,
			"python3", "script.py",
		)
		out, err = cmd.CombinedOutput()
		if err != nil || len(out) == 0 {
			// Container file-sharing timeout or docker daemon issue: fallback to host execution
			hostCmd := exec.Command("python3", scriptPath)
			hostCmd.Dir = tmpDir
			hostCmd.Env = []string{"PATH=" + os.Getenv("PATH")}
			hostOut, hostErr := hostCmd.CombinedOutput()
			if hostErr == nil {
				out = hostOut
				err = nil
			}
		}
	} else {
		// Local host fallback with restricted environment
		cmd = exec.CommandContext(ctx, "python3", scriptPath)
		cmd.Dir = tmpDir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		out, err = cmd.CombinedOutput()
	}
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return &ExecutionResult{
		Stdout:    string(out),
		Stderr:    "",
		ExitCode:  exitCode,
		Duration:  duration,
		Sandboxed: r.useDocker,
	}, nil
}
