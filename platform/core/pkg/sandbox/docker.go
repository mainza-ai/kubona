package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrSandboxExecutionFailed = errors.New("sandbox execution failed")
	ErrDockerNotAvailable     = errors.New("docker daemon not available")
	ErrExecutionTimeout       = errors.New("execution timed out in sandbox")
	ErrDevModeInProduction    = errors.New("KUBONA_DEV_MODE is strictly prohibited in production")
)

// ExecutionResult captures the output, metrics, and security attributes of a run
type ExecutionResult struct {
	Stdout       string        `json:"stdout"`
	Stderr       string        `json:"stderr"`
	ExitCode     int           `json:"exit_code"`
	Duration     time.Duration `json:"duration"`
	Sandboxed    bool          `json:"sandboxed"`
	IsolationTag string        `json:"isolation_tag"`
	MemoryMB     float64       `json:"memory_mb"`
	Error        string        `json:"error,omitempty"`
}

// Config declares the security profile and resource clamps (Tier B Sandbox)
type Config struct {
	Image             string        `json:"image"`
	Timeout           time.Duration `json:"timeout"`
	CPULimit          string        `json:"cpu_limit"`
	MemoryLimit       string        `json:"memory_limit"`
	MemorySwap        string        `json:"memory_swap"`
	PidsLimit         int           `json:"pids_limit"`
	Network           string        `json:"network"`
	User              string        `json:"user"`
	AllowHostFallback bool          `json:"allow_host_fallback"`
}

// DefaultConfig provides production-grade NemoClaw/OpenClaw security parameters
func DefaultConfig() Config {
	return Config{
		Image:             "kubona/quantum-runner:latest",
		Timeout:           60 * time.Second,
		CPULimit:          "2.0",
		MemoryLimit:       "4g",
		MemorySwap:        "4g",
		PidsLimit:         128,
		Network:           "none",
		User:              "1000:1000",
		AllowHostFallback: false,
	}
}

// SandboxRunner coordinates execution inside hardened ephemeral containers
type SandboxRunner struct {
	cfg       Config
	useDocker bool
}

// NewSandboxRunner instantiates a sandbox runner verifying Docker availability
func NewSandboxRunner(image string, timeout time.Duration) *SandboxRunner {
	cfg := DefaultConfig()
	if image != "" {
		cfg.Image = image
	}
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	return NewSandboxRunnerWithConfig(cfg)
}

// NewSandboxRunnerWithConfig creates a runner with custom security configurations
func NewSandboxRunnerWithConfig(cfg Config) *SandboxRunner {
	if cfg.Image == "" {
		cfg.Image = "kubona/quantum-runner:latest"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.CPULimit == "" {
		cfg.CPULimit = "2.0"
	}
	if cfg.MemoryLimit == "" {
		cfg.MemoryLimit = "4g"
	}
	if cfg.MemorySwap == "" {
		cfg.MemorySwap = "4g"
	}
	if cfg.PidsLimit <= 0 {
		cfg.PidsLimit = 128
	}
	if cfg.Network == "" {
		cfg.Network = "none"
	}
	if cfg.User == "" {
		cfg.User = "1000:1000"
	}

	if os.Getenv("KUBONA_ENV") == "production" {
		cfg.AllowHostFallback = false
	}

	useDocker := false
	if exec.Command("docker", "info").Run() == nil {
		// Check primary image or fallback images
		if exec.Command("docker", "image", "inspect", cfg.Image).Run() == nil {
			useDocker = true
		} else {
			for _, alt := range []string{"python:3.11-slim", "python:3.12-slim", "python:3.12-alpine"} {
				if exec.Command("docker", "image", "inspect", alt).Run() == nil {
					cfg.Image = alt
					useDocker = true
					break
				}
			}
		}
	}

	if useDocker {
		_ = EnsureWarmRunner(cfg.Image)
	}

	return &SandboxRunner{
		cfg:       cfg,
		useDocker: useDocker,
	}
}

// EnsureWarmRunner ensures that a persistent warm runner container is active in Docker Desktop
func EnsureWarmRunner(image string) error {
	out, err := exec.Command("docker", "ps", "--filter", "name=kubona-quantum-runner", "--format", "{{.Names}}").Output()
	if err == nil && strings.TrimSpace(string(out)) == "kubona-quantum-runner" {
		return nil
	}
	_ = exec.Command("docker", "rm", "-f", "kubona-quantum-runner").Run()
	cmd := exec.Command("docker", "run", "-d", "--name", "kubona-quantum-runner", "--restart", "unless-stopped", "--entrypoint", "tail", image, "-f", "/dev/null")
	return cmd.Run()
}

// Config returns the active runner configuration
func (r *SandboxRunner) Config() Config {
	return r.cfg
}

// IsDockerAvailable indicates if the Docker container runtime is active
func (r *SandboxRunner) IsDockerAvailable() bool {
	return r.useDocker
}

// ExecutePython executes an ad-hoc python script in the hardened sandbox
func (r *SandboxRunner) ExecutePython(code string) (*ExecutionResult, error) {
	return r.ExecuteHarness(code, "script.py", "")
}

// ExecuteHarness implements dual-mount isolation:
// - /benchmark: read-only evaluation code
// - /workspace: read-write isolated directory for the mutable artifact
func (r *SandboxRunner) ExecuteHarness(benchmarkScript, mutableArtifactName, mutableArtifactContent string) (*ExecutionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "kubona-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox scratch dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	benchmarkDir := filepath.Join(tmpDir, "benchmark")
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(benchmarkDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspaceDir, 0777); err != nil {
		return nil, err
	}

	evalPath := filepath.Join(benchmarkDir, "eval.py")
	if err := os.WriteFile(evalPath, []byte(benchmarkScript), 0644); err != nil {
		return nil, fmt.Errorf("failed to write evaluation script: %w", err)
	}

	if mutableArtifactName != "" && mutableArtifactContent != "" {
		baseName := filepath.Base(mutableArtifactName)
		if baseName != mutableArtifactName || strings.Contains(mutableArtifactName, "..") {
			return nil, fmt.Errorf("invalid artifact name %q: path traversal rejected", mutableArtifactName)
		}
		artPath := filepath.Join(workspaceDir, baseName)
		if err := os.WriteFile(artPath, []byte(mutableArtifactContent), 0666); err != nil {
			return nil, fmt.Errorf("failed to write mutable artifact: %w", err)
		}
	}

	startTime := time.Now()

	if r.useDocker {
		// Strict hardened container invocation compliant with NemoClaw / OpenClaw
		args := []string{
			"run", "--rm",
			"--read-only",
			"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
			"--cap-drop=ALL",
			"--security-opt=no-new-privileges",
			"--network", r.cfg.Network,
			"--memory", r.cfg.MemoryLimit,
			"--memory-swap", r.cfg.MemorySwap,
			"--cpus", r.cfg.CPULimit,
			fmt.Sprintf("--pids-limit=%d", r.cfg.PidsLimit),
			"-v", fmt.Sprintf("%s:/benchmark:ro", benchmarkDir),
			"-v", fmt.Sprintf("%s:/workspace:rw", workspaceDir),
			"-w", "/workspace",
			"-e", "PYTHONPATH=/workspace",
			"-e", "PYTHONUNBUFFERED=1",
		}

		if r.cfg.User != "" && r.cfg.User != "root" {
			args = append(args, "--user", r.cfg.User)
		}

		args = append(args, "--entrypoint", "python3", r.cfg.Image, "/benchmark/eval.py")

		cmd := exec.CommandContext(ctx, "docker", args...)
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		runErr := cmd.Run()
		duration := time.Since(startTime)

		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return &ExecutionResult{
					Stderr:       "Execution timed out in sandbox",
					ExitCode:     124,
					Duration:     duration,
					Sandboxed:    true,
					IsolationTag: "docker-hardened",
					Error:        ErrExecutionTimeout.Error(),
				}, ErrExecutionTimeout
			} else {
				exitCode = 1
			}
		}

		return &ExecutionResult{
			Stdout:       stdoutBuf.String(),
			Stderr:       stderrBuf.String(),
			ExitCode:     exitCode,
			Duration:     duration,
			Sandboxed:    true,
			IsolationTag: fmt.Sprintf("docker:%s (net=%s, cpus=%s, mem=%s)", r.cfg.Image, r.cfg.Network, r.cfg.CPULimit, r.cfg.MemoryLimit),
			Error:        "",
		}, nil
	}

	// Docker unavailable: strictly refuse unless explicit AllowHostFallback or KUBONA_DEV_MODE is configured
	isProd := os.Getenv("KUBONA_ENV") == "production"
	devMode := os.Getenv("KUBONA_DEV_MODE") == "1"

	if isProd && devMode {
		return nil, ErrDevModeInProduction
	}

	if isProd || (!r.cfg.AllowHostFallback && !devMode) {
		return nil, fmt.Errorf("%w: Docker daemon or required image '%s' is not accessible and sandbox isolation is required", ErrDockerNotAvailable, r.cfg.Image)
	}

	fmt.Fprintln(os.Stderr, "⚠️  WARNING: Running un-sandboxed host execution fallback (KUBONA_DEV_MODE or AllowHostFallback enabled). NEVER use in production!")

	// Resolve the most specific Python interpreter available (KUBONA_PYTHON, VIRTUAL_ENV, or local .venv)
	pyBin, extraEnv := resolvePythonInterpreter(workspaceDir)

	cmd := exec.CommandContext(ctx, pyBin, evalPath)
	cmd.Dir = workspaceDir

	pathEnv := os.Getenv("PATH")
	if pyBin != "python3" {
		binDir := filepath.Dir(pyBin)
		pathEnv = binDir + string(os.PathListSeparator) + pathEnv
	}

	envVars := []string{
		"PATH=" + pathEnv,
		"PYTHONPATH=" + workspaceDir,
		"PYTHONUNBUFFERED=1",
	}
	envVars = append(envVars, extraEnv...)
	cmd.Env = envVars

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	duration := time.Since(startTime)

	exitCode := 0
	errStr := ""
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			exitCode = 124
			errStr = "execution timed out: " + r.cfg.Timeout.String()
		} else if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			errStr = exitErr.Error()
		} else {
			exitCode = 1
			errStr = runErr.Error()
		}
	}

	tag := "host-fallback (UNSANDBOXED)"
	if devMode {
		tag = "host-fallback (UNSANDBOXED - DEV MODE)"
	}

	return &ExecutionResult{
		Stdout:       stdoutBuf.String(),
		Stderr:       stderrBuf.String(),
		ExitCode:     exitCode,
		Duration:     duration,
		Sandboxed:    false, // Explicitly false! Never deceive caller.
		IsolationTag: tag,
		Error:        errStr,
	}, nil
}

// resolvePythonInterpreter searches for a suitable Python 3 executable in virtual environments
// before falling back to system "python3".
func resolvePythonInterpreter(workspaceDir string) (string, []string) {
	// 1. Explicit environment override
	if p := os.Getenv("KUBONA_PYTHON"); p != "" {
		if _, err := os.Stat(p); err == nil {
			venvDir := filepath.Dir(filepath.Dir(p))
			return p, []string{"VIRTUAL_ENV=" + venvDir}
		}
	}

	// 2. Active virtual environment
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		p := filepath.Join(venv, "bin", "python3")
		if _, err := os.Stat(p); err == nil {
			return p, []string{"VIRTUAL_ENV=" + venv}
		}
	}

	// 3. Search common project locations walking up from workspace and current working directory
	cwd, _ := os.Getwd()
	searchRoots := []string{workspaceDir, cwd}
	for _, base := range searchRoots {
		if base == "" {
			continue
		}
		for dir := base; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			candidates := []string{
				filepath.Join(dir, "platform", ".venv", "bin", "python3"),
				filepath.Join(dir, ".venv", "bin", "python3"),
			}
			for _, cand := range candidates {
				if stat, err := os.Stat(cand); err == nil && !stat.IsDir() {
					venvDir := filepath.Dir(filepath.Dir(cand))
					return cand, []string{"VIRTUAL_ENV=" + venvDir}
				}
			}
		}
	}

	return "python3", nil
}
