package sandbox

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultSeccompProfile is the shipped Phase-2 seccomp profile, embedded so
// the kubonad binary carries it wherever it runs.
//
// Why a duplicate + embed + temp file (and not a repo-relative path or a
// go:embed of the canonical file):
//   - go:embed is confined to the Go module (platform/core), so the
//     canonical platform/bridge/seccomp-profile.json CANNOT be embedded
//     directly; this copy (pkg/sandbox/seccomp-profile.json) is the
//     embeddable duplicate. A semantic parity test
//     (seccomp_test.go::TestEmbeddedProfileMatchesBridgeCanonical) fails on
//     any drift — same defaultAction, arch list, and syscall allow sets,
//     compared as parsed JSON, not bytes.
//   - The daemon resolves `--security-opt seccomp=<path>` on ITS OWN host
//     filesystem, so we cannot pass an in-memory profile: at run time the
//     embedded bytes are written to the execution scratch dir and THAT path
//     is passed. The scratch dir is already bind-mounted into the container
//     (benchmark/workspace), so the profile file inherits exactly the same
//     daemon visibility as the workload mounts — no new topology
//     constraint: wherever `docker run -v <tmpdir>:...` works (host-docker
//     topology: kubonad on the host talking to a local daemon), the profile
//     path works too.
//   - Compose topology (platform/docker/compose.prod.yml): kubonad there
//     mounts NO docker socket, so it cannot run containers at all (already
//     fail-closed via ErrDockerNotAvailable) — the profile needs no
//     visibility into compose. If an operator DID mount the socket into a
//     kubonad container, the temp path would be container-local and the
//     daemon would reject it; ExecuteHarness then fails closed (the docker
//     error is returned, never retried without the profile). Fail-closed,
//     not fallback: silently running under Docker's weaker default would
//     undo the backstop with zero signal, and a loud fallback would still
//     execute the workload under a weaker boundary — refusal forces the
//     operator onto the supported host-docker topology. Tested by
//     TestSeccompUnconfinedRejected / TestSeccompWriteFailureFailsClosed.
//
//go:embed seccomp-profile.json
var defaultSeccompProfile []byte

var (
	ErrSandboxExecutionFailed = errors.New("sandbox execution failed")
	ErrDockerNotAvailable     = errors.New("docker daemon not available")
	ErrExecutionTimeout       = errors.New("execution timed out in sandbox")
	ErrDevModeInProduction    = errors.New("KUBONA_DEV_MODE is strictly prohibited in production")
)

// DefaultRunnerImage is the single Go-side pin of the quantum-runner image.
// It MUST match the Python rail literal `kubona-quantum-runner:1.1.0` used in
// platform/kubona/sandbox/execution.py (KUBONA_SANDBOX_IMAGE default),
// platform/kubona/eval/codeexec.py, and the platform/tests expectations.
// Both rails pull the same image so a harness verified in one sandbox runs
// byte-identical in the other; bump both rails together, never one alone.
const DefaultRunnerImage = "kubona-quantum-runner:1.1.0"

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
	Runtime           string        `json:"runtime,omitempty"`
	// SeccompProfile is an operator override: absolute daemon-side path of
	// a custom seccomp profile. "" (the default) applies the embedded
	// shipped profile (Phase-2 default-on). There is deliberately no value
	// that disables seccomp: "unconfined" is rejected outright, and a run
	// that cannot materialize a profile refuses (fail closed).
	SeccompProfile string `json:"seccomp_profile,omitempty"`
}

// DefaultConfig provides production-grade NemoClaw/OpenClaw security parameters
func DefaultConfig() Config {
	runtime := os.Getenv("KUBONA_SANDBOX_RUNTIME")
	return Config{
		Image:             DefaultRunnerImage,
		Timeout:           60 * time.Second,
		CPULimit:          "2.0",
		MemoryLimit:       "4g",
		MemorySwap:        "4g",
		PidsLimit:         128,
		Network:           "none",
		User:              "1000:1000",
		AllowHostFallback: false,
		Runtime:           runtime,
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
		cfg.Image = DefaultRunnerImage
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

	return &SandboxRunner{
		cfg:       cfg,
		useDocker: useDocker,
	}
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

	// Default-on (Phase-2): every docker run carries the seccomp profile.
	// resolveSeccompProfile refuses the run when no profile can be
	// materialized — ExecuteHarness never runs profile-less (fail closed).
	profilePath, err := r.resolveSeccompProfile(tmpDir)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()

	if r.useDocker {
		args := r.dockerArgs(benchmarkDir, workspaceDir, profilePath)

		cmd := exec.CommandContext(ctx, "docker", args...)
		setProcessGroup(cmd)
		cmd.Cancel = func() error {
			return killProcessGroup(cmd)
		}
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

		runtimeTag := "docker"
		if r.cfg.Runtime != "" {
			runtimeTag = fmt.Sprintf("docker[%s]", r.cfg.Runtime)
		}
		seccompTag := "seccomp=kubona-default"
		if r.cfg.SeccompProfile != "" {
			seccompTag = "seccomp=custom"
		}
		return &ExecutionResult{
			Stdout:       stdoutBuf.String(),
			Stderr:       stderrBuf.String(),
			ExitCode:     exitCode,
			Duration:     duration,
			Sandboxed:    true,
			IsolationTag: fmt.Sprintf("%s:%s (net=%s, cpus=%s, mem=%s, %s)", runtimeTag, r.cfg.Image, r.cfg.Network, r.cfg.CPULimit, r.cfg.MemoryLimit, seccompTag),
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
	setProcessGroup(cmd)
	cmd.Cancel = func() error {
		return killProcessGroup(cmd)
	}

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

// resolveSeccompProfile returns the daemon-side seccomp profile path for
// this run: the operator override when Config.SeccompProfile is set,
// otherwise the embedded shipped profile materialized into the execution
// scratch dir. Any failure refuses the run (fail closed) — the caller
// never falls back to a profile-less invocation.
func (r *SandboxRunner) resolveSeccompProfile(tmpDir string) (string, error) {
	if r.cfg.SeccompProfile == "unconfined" {
		return "", fmt.Errorf("sandbox seccomp=unconfined is prohibited: refusing to run without the profile")
	}
	if r.cfg.SeccompProfile != "" {
		return r.cfg.SeccompProfile, nil
	}
	path := filepath.Join(tmpDir, "seccomp-profile.json")
	if err := os.WriteFile(path, defaultSeccompProfile, 0644); err != nil {
		return "", fmt.Errorf("sandbox seccomp profile unavailable: %w: refusing to run without the profile", err)
	}
	return path, nil
}

// dockerArgs builds the strict hardened container invocation compliant
// with NemoClaw / OpenClaw. profilePath is always a concrete profile file
// (see resolveSeccompProfile) — the seccomp flag is unconditional
// (Phase-2 default-on); seccomp=unconfined is never emitted.
func (r *SandboxRunner) dockerArgs(benchmarkDir, workspaceDir, profilePath string) []string {
	args := []string{
		"run", "--rm",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--security-opt", "seccomp=" + profilePath,
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

	if r.cfg.Runtime != "" {
		args = append(args, "--runtime", r.cfg.Runtime)
	}

	if r.cfg.User != "" && r.cfg.User != "root" {
		args = append(args, "--user", r.cfg.User)
	}

	return append(args, "--entrypoint", "python3", r.cfg.Image, "/benchmark/eval.py")
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
