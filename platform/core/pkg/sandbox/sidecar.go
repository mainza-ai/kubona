package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sidecar API contract. The dispatcher (kubonad) sends only the benchmark
// script and the mutable artifact; image, runtime, mounts, and security
// flags are fixed by the sidecar's boot configuration. The sidecar is the
// only process in the supported topology with Docker-socket access.
const (
	SidecarSocketEnv  = "KUBONA_SANDBOX_SOCKET"
	SidecarRunPath    = "/v1/run"
	SidecarHealthPath = "/health"
	maxSidecarBody    = 8 << 20
)

// SidecarRequest is the complete request surface exposed to kubonad.
type SidecarRequest struct {
	Benchmark      string `json:"benchmark"`
	ArtifactName   string `json:"artifact_name,omitempty"`
	Artifact       string `json:"artifact,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// SidecarResponse is deliberately shaped like ExecutionResult so the API
// and auditors see the same fields on both rails.
type SidecarResponse struct {
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ExitCode     int    `json:"exit_code"`
	DurationMS   int64  `json:"duration_ms"`
	Sandboxed    bool   `json:"sandboxed"`
	IsolationTag string `json:"isolation_tag"`
	Error        string `json:"error,omitempty"`
}

// SidecarClient calls the private Unix-socket dispatcher.
type SidecarClient struct {
	socket string
	client *http.Client
}

func NewSidecarClient(socket string) *SidecarClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return &SidecarClient{socket: socket, client: &http.Client{Transport: transport}}
}

func (c *SidecarClient) Execute(ctx context.Context, req SidecarRequest) (*ExecutionResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode sandbox request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://sandboxd"+SidecarRunPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sandboxd unavailable at %s: %w", c.socket, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		if msg, _ := payload["error"].(string); msg != "" {
			return nil, fmt.Errorf("sandboxd refused: %s", msg)
		}
		return nil, fmt.Errorf("sandboxd HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out SidecarResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode sandboxd response: %w", err)
	}
	result := &ExecutionResult{
		Stdout:       out.Stdout,
		Stderr:       out.Stderr,
		ExitCode:     out.ExitCode,
		Duration:     time.Duration(out.DurationMS) * time.Millisecond,
		Sandboxed:    out.Sandboxed,
		IsolationTag: out.IsolationTag,
		Error:        out.Error,
	}
	if out.Error != "" && out.ExitCode != 0 {
		return result, errors.New(out.Error)
	}
	return result, nil
}

func (c *SidecarClient) Health(ctx context.Context) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://sandboxd"+SidecarHealthPath, nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("sandboxd unavailable at %s: %v", c.socket, err)
	}
	defer resp.Body.Close()
	var payload struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason"`
		Runtime   string `json:"runtime"`
		Image     string `json:"image"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&payload)
	if resp.StatusCode == http.StatusOK && payload.Available {
		return true, ""
	}
	if payload.Reason != "" {
		return false, payload.Reason
	}
	return false, fmt.Sprintf("sandboxd health HTTP %d", resp.StatusCode)
}

// SidecarServer is the narrow Docker-socket broker. It intentionally has no
// arbitrary-image, arbitrary-command, or arbitrary-mount API.
type SidecarServer struct {
	Socket        string
	Image         string
	Runtime       string
	ScratchRoot   string
	SeccompPath   string
	MaxConcurrent int

	slotsOnce sync.Once
	slots     chan struct{}
}

func (s *SidecarServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(SidecarHealthPath, s.handleHealth)
	mux.HandleFunc(SidecarRunPath, s.handleRun)
	return http.MaxBytesHandler(mux, maxSidecarBody)
}

func (s *SidecarServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	available, reason := s.available()
	status := http.StatusOK
	if !available {
		status = http.StatusServiceUnavailable
	}
	writeSidecarJSON(w, status, map[string]any{
		"available": available,
		"reason":    reason,
		"runtime":   s.Runtime,
		"image":     s.Image,
	})
}

func (s *SidecarServer) available() (bool, string) {
	if strings.TrimSpace(s.Runtime) != "runsc" {
		return false, "sandbox runtime must be runsc (gVisor): configure --runtime runsc"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .Runtimes}}").Output()
	if err != nil {
		return false, "Docker daemon is not reachable from sandboxd"
	}
	if !bytes.Contains(out, []byte(`"runsc"`)) {
		return false, "Docker daemon does not advertise the runsc runtime (install gVisor and register runsc)"
	}
	return true, ""
}

func (s *SidecarServer) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeSidecarError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if ok, reason := s.available(); !ok {
		writeSidecarError(w, http.StatusServiceUnavailable, reason)
		return
	}
	var req SidecarRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeSidecarError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Benchmark) == "" {
		writeSidecarError(w, http.StatusBadRequest, "benchmark is required")
		return
	}
	if req.ArtifactName != "" {
		base := filepath.Base(req.ArtifactName)
		if base != req.ArtifactName || strings.Contains(req.ArtifactName, "..") {
			writeSidecarError(w, http.StatusBadRequest, "artifact_name must be a basename")
			return
		}
	}
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if timeout > 180*time.Second {
		timeout = 180 * time.Second
	}

	// A bounded semaphore prevents a client from exhausting Docker or the
	// sidecar's scratch filesystem with concurrent workloads.
	if !s.acquireSlot() {
		writeSidecarError(w, http.StatusTooManyRequests, "sandboxd concurrency limit reached")
		return
	}
	defer s.releaseSlot()

	res := s.runOne(r.Context(), req, timeout)
	writeSidecarJSON(w, http.StatusOK, res)
}

func (s *SidecarServer) acquireSlot() bool {
	s.slotsOnce.Do(func() {
		limit := s.MaxConcurrent
		if limit <= 0 {
			limit = 2
		}
		s.slots = make(chan struct{}, limit)
	})
	select {
	case s.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *SidecarServer) releaseSlot() {
	select {
	case <-s.slots:
	default:
	}
}

func (s *SidecarServer) runOne(parent context.Context, req SidecarRequest, timeout time.Duration) SidecarResponse {
	root := s.ScratchRoot
	if root == "" {
		root = os.TempDir()
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return SidecarResponse{Sandboxed: false, Error: "create scratch root: " + err.Error(), ExitCode: 1}
	}
	tmp, err := os.MkdirTemp(root, "run-*")
	if err != nil {
		return SidecarResponse{Sandboxed: false, Error: "create scratch: " + err.Error(), ExitCode: 1}
	}
	defer os.RemoveAll(tmp)
	benchDir := filepath.Join(tmp, "benchmark")
	workDir := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(benchDir, 0755); err != nil {
		return SidecarResponse{Sandboxed: false, Error: err.Error(), ExitCode: 1}
	}
	if err := os.MkdirAll(workDir, 0777); err != nil {
		return SidecarResponse{Sandboxed: false, Error: err.Error(), ExitCode: 1}
	}
	if err := os.WriteFile(filepath.Join(benchDir, "eval.py"), []byte(req.Benchmark), 0644); err != nil {
		return SidecarResponse{Sandboxed: false, Error: err.Error(), ExitCode: 1}
	}
	if req.ArtifactName != "" && req.Artifact != "" {
		if err := os.WriteFile(filepath.Join(workDir, req.ArtifactName), []byte(req.Artifact), 0666); err != nil {
			return SidecarResponse{Sandboxed: false, Error: err.Error(), ExitCode: 1}
		}
	}
	profile := s.SeccompPath
	if profile == "" {
		profile = filepath.Join(tmp, "seccomp.json")
		if err := os.WriteFile(profile, defaultSeccompProfile, 0644); err != nil {
			return SidecarResponse{Sandboxed: false, Error: "seccomp profile unavailable: " + err.Error(), ExitCode: 1}
		}
	}
	runID := newRunID()
	if runID == "" {
		return SidecarResponse{Sandboxed: false, Error: "cannot mint run id", ExitCode: 1}
	}
	args := sidecarDockerArgs(s.Image, s.Runtime, benchDir, workDir, profile, runID)
	if args == nil {
		return SidecarResponse{Sandboxed: false, Error: "cannot build sandbox invocation", ExitCode: 1}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	errText := ""
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			stopSandboxContainer(sandboxName(runID))
			exitCode = 124
			errText = ErrExecutionTimeout.Error()
		} else if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
			errText = ee.Error()
		} else {
			exitCode = 1
			errText = runErr.Error()
		}
	}
	return SidecarResponse{
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		ExitCode:     exitCode,
		DurationMS:   duration.Milliseconds(),
		Sandboxed:    true,
		IsolationTag: fmt.Sprintf("gvisor-runsc:%s (net=none, readonly, cap-drop=ALL)", s.Image),
		Error:        errText,
	}
}

func sidecarDockerArgs(image, runtime, benchmarkDir, workspaceDir, profilePath, runID string) []string {
	if runID == "" || strings.TrimSpace(image) == "" || runtime != "runsc" {
		return nil
	}
	return []string{
		"run", "--rm",
		"--name", sandboxName(runID),
		"--label", sandboxManagedKey + "=" + sandboxManagedVal,
		"--label", sandboxRunIDKey + "=" + runID,
		"--runtime", runtime,
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--security-opt", "seccomp=" + profilePath,
		"--network", "none",
		"--memory", "4g",
		"--memory-swap", "4g",
		"--cpus", "2.0",
		"--pids-limit=128",
		"-v", benchmarkDir + ":/benchmark:ro",
		"-v", workspaceDir + ":/workspace:rw",
		"-w", "/workspace",
		"-e", "PYTHONPATH=/workspace",
		"-e", "PYTHONUNBUFFERED=1",
		"--user", "1000:1000",
		"--entrypoint", "python3", image, "/benchmark/eval.py",
	}
}

func writeSidecarJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSidecarError(w http.ResponseWriter, status int, message string) {
	writeSidecarJSON(w, status, map[string]any{"error": message})
}

// ServeSidecar runs the Unix-socket dispatcher until ctx is cancelled.
func ServeSidecar(ctx context.Context, s *SidecarServer) error {
	if s.Socket == "" {
		return fmt.Errorf("sandboxd socket path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Socket), 0700); err != nil {
		return err
	}
	_ = os.Remove(s.Socket)
	ln, err := net.Listen("unix", s.Socket)
	if err != nil {
		return err
	}
	defer ln.Close()
	// The daemon runs as UID 1000; the sidecar is root to reach the Docker
	// socket. Hand the socket to the daemon's UID so 0600 is sufficient and no
	// broad group permission is needed.
	if os.Geteuid() == 0 {
		if err := os.Chown(s.Socket, 1000, 1000); err != nil {
			return err
		}
	}
	if err := os.Chmod(s.Socket, 0600); err != nil {
		return err
	}
	srv := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.Serve(ln)
}
