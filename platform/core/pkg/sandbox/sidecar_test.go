package sandbox

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSidecarDockerArgsAreGVisorFailClosed(t *testing.T) {
	args := sidecarDockerArgs("runner:test", "runsc", "/b", "/w", "/s.json", "abcdef0123456789")
	if args == nil {
		t.Fatal("expected args")
	}
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	for _, want := range []string{"--runtime runsc", "--network none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--user 1000:1000"} {
		if !contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if sidecarDockerArgs("runner:test", "docker", "/b", "/w", "/s.json", "abcdef0123456789") != nil {
		t.Fatal("non-runsc runtime must refuse")
	}
	if sidecarDockerArgs("", "runsc", "/b", "/w", "/s.json", "abcdef0123456789") != nil {
		t.Fatal("empty image must refuse")
	}
}

func TestSidecarHealthRefusesNonGVisorRuntime(t *testing.T) {
	s := &SidecarServer{Runtime: "docker", Image: "runner:test"}
	rec := &responseRecorder{}
	s.handler().ServeHTTP(rec, mustRequest(http.MethodGet, SidecarHealthPath))
	if rec.code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.code, rec.body)
	}
	if !contains(rec.body, "runsc") {
		t.Fatalf("health did not name the gVisor requirement: %s", rec.body)
	}
}

func TestSidecarClientExecuteOverUnixSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "kubona-sidecar-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SidecarRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Benchmark != "print(1)" || req.ArtifactName != "answer.txt" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(SidecarResponse{
			Stdout: "1\n", ExitCode: 0, Sandboxed: true,
			IsolationTag: "gvisor-runsc", DurationMS: 12,
		})
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	client := NewSidecarClient(socket)
	res, err := client.Execute(context.Background(), SidecarRequest{
		Benchmark: "print(1)", ArtifactName: "answer.txt", Artifact: "42",
		TimeoutSeconds: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "1\n" || !res.Sandboxed || res.ExitCode != 0 || res.Duration != 12*time.Millisecond {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestSidecarClientRefusalIsNotSuccess(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "kubona-sidecar-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "runsc missing"})
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	_, err = NewSidecarClient(socket).Execute(context.Background(), SidecarRequest{Benchmark: "print(1)"})
	if err == nil || !contains(err.Error(), "runsc missing") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

type responseRecorder struct {
	code int
	body string
}

func (r *responseRecorder) Header() http.Header  { return http.Header{} }
func (r *responseRecorder) WriteHeader(code int) { r.code = code }
func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body += string(b)
	return len(b), nil
}

func mustRequest(method, path string) *http.Request {
	req, _ := http.NewRequest(method, path, nil)
	return req
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func TestSidecarClientHealth(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "kubona-sidecar-health-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"available": true, "runtime": "runsc", "image": "runner:test",
		})
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	ok, reason := NewSidecarClient(socket).Health(context.Background())
	if !ok || reason != "" {
		t.Fatalf("health = %v %q", ok, reason)
	}
}

func TestSidecarRunOneUsesFixedArgsAndCapturesOutput(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "kubona-sidecar-run-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >&2\necho sandbox-ok\n"
	if err := os.WriteFile(fake, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	s := &SidecarServer{Image: "runner:test", Runtime: "runsc", ScratchRoot: filepath.Join(dir, "scratch")}
	res := s.runOne(context.Background(), SidecarRequest{
		Benchmark: "print(1)", ArtifactName: "a.txt", Artifact: "x",
		TimeoutSeconds: 5,
	}, 5*time.Second)
	if res.ExitCode != 0 || res.Stdout != "sandbox-ok\n" || !res.Sandboxed {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !contains(res.Stderr, "--runtime") || !contains(res.Stderr, "runsc") {
		t.Fatalf("fixed runtime not passed: %s", res.Stderr)
	}
	if !contains(res.Stderr, "--network") || !contains(res.Stderr, "none") {
		t.Fatalf("fixed network not passed: %s", res.Stderr)
	}
}

func TestSidecarSlotsAreBounded(t *testing.T) {
	s := &SidecarServer{MaxConcurrent: 1}
	if !s.acquireSlot() {
		t.Fatal("first slot must acquire")
	}
	if s.acquireSlot() {
		t.Fatal("second slot must refuse")
	}
	s.releaseSlot()
	if !s.acquireSlot() {
		t.Fatal("released slot must acquire")
	}
	s.releaseSlot()
}
