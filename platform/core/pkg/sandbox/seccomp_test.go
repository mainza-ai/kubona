package sandbox

// Phase-2 seccomp default-on tests (2026-10-29): the embedded profile must
// stay semantically identical to the canonical
// platform/bridge/seccomp-profile.json (parsed-JSON comparison, not bytes),
// every docker invocation must carry the seccomp flag with no opt-out, and
// any failure to materialize a profile must refuse the run (fail closed).

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// bridgeCanonicalPath locates the canonical profile independent of the test
// working directory.
func bridgeCanonicalPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// this file lives in platform/core/pkg/sandbox/; the canonical profile
	// in platform/bridge/.
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "bridge", "seccomp-profile.json")
}

type seccompProfileDoc struct {
	DefaultAction   string           `json:"defaultAction"`
	DefaultErrnoRet *int             `json:"defaultErrnoRet"`
	ArchMap         []map[string]any `json:"archMap"`
	Syscalls        []map[string]any `json:"syscalls"`
}

// fingerprintEntries reduces the syscall allowlist to a multiset of
// canonical entry fingerprints (names sorted, then JSON with sorted keys —
// encoding/json sorts map keys, so formatting drift cannot false-positive).
func fingerprintEntries(t *testing.T, doc seccompProfileDoc) map[string]int {
	t.Helper()
	out := make(map[string]int, len(doc.Syscalls))
	for _, entry := range doc.Syscalls {
		clone := make(map[string]any, len(entry))
		for k, v := range entry {
			clone[k] = v
		}
		if names, ok := clone["names"].([]any); ok {
			sorted := append([]any(nil), names...)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].(string) < sorted[j].(string)
			})
			clone["names"] = sorted
		}
		raw, err := json.Marshal(clone)
		if err != nil {
			t.Fatalf("cannot fingerprint seccomp entry: %v", err)
		}
		out[string(raw)]++
	}
	return out
}

func archSet(doc seccompProfileDoc) map[string]bool {
	out := make(map[string]bool, len(doc.ArchMap))
	for _, arch := range doc.ArchMap {
		if name, ok := arch["architecture"].(string); ok {
			out[name] = true
		}
	}
	return out
}

func TestEmbeddedProfileMatchesBridgeCanonical(t *testing.T) {
	raw, err := os.ReadFile(bridgeCanonicalPath(t))
	if err != nil {
		t.Fatalf("cannot read canonical bridge profile: %v", err)
	}
	var embedded, canonical seccompProfileDoc
	if err := json.Unmarshal(defaultSeccompProfile, &embedded); err != nil {
		t.Fatalf("embedded profile is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(raw, &canonical); err != nil {
		t.Fatalf("canonical bridge profile is not valid JSON: %v", err)
	}

	if embedded.DefaultAction != canonical.DefaultAction {
		t.Fatalf("defaultAction drift: embedded %q != canonical %q",
			embedded.DefaultAction, canonical.DefaultAction)
	}
	if (embedded.DefaultErrnoRet == nil) != (canonical.DefaultErrnoRet == nil) ||
		(embedded.DefaultErrnoRet != nil && canonical.DefaultErrnoRet != nil &&
			*embedded.DefaultErrnoRet != *canonical.DefaultErrnoRet) {
		t.Fatalf("defaultErrnoRet drift: embedded %v != canonical %v",
			embedded.DefaultErrnoRet, canonical.DefaultErrnoRet)
	}

	embArch, canArch := archSet(embedded), archSet(canonical)
	if len(embArch) != len(canArch) {
		t.Fatalf("arch list drift: embedded %v != canonical %v", embArch, canArch)
	}
	for arch := range canArch {
		if !embArch[arch] {
			t.Fatalf("arch list drift: %q missing from embedded copy (canonical %v)", arch, canArch)
		}
	}

	embFP, canFP := fingerprintEntries(t, embedded), fingerprintEntries(t, canonical)
	if len(embFP) != len(canFP) {
		t.Fatalf("syscall allow-set size drift: embedded %d entries != canonical %d",
			len(embFP), len(canFP))
	}
	for fp, count := range canFP {
		if embFP[fp] != count {
			t.Fatalf("syscall allow-set drift: canonical entry %s (x%d) has embedded count %d — "+
				"re-copy platform/bridge/seccomp-profile.json, never hand-edit the duplicate", fp, count, embFP[fp])
		}
	}
}

func TestDockerArgsApplySeccompDefaultOn(t *testing.T) {
	r := &SandboxRunner{cfg: DefaultConfig()}
	args := r.dockerArgs("/bench", "/work", "/tmp/scratch/seccomp-profile.json", "deadbeef0123")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--security-opt seccomp=/tmp/scratch/seccomp-profile.json") {
		t.Fatalf("default-on seccomp flag missing from docker args: %q", joined)
	}
	if strings.Count(joined, "seccomp=") != 1 {
		t.Fatalf("expected exactly one seccomp flag, got: %q", joined)
	}
	if strings.Contains(joined, "unconfined") {
		t.Fatalf("seccomp=unconfined must never be emitted: %q", joined)
	}
}

func TestResolveSeccompProfileDefaultWritesEmbedded(t *testing.T) {
	r := &SandboxRunner{cfg: DefaultConfig()}
	dir := t.TempDir()
	path, err := r.resolveSeccompProfile(dir)
	if err != nil {
		t.Fatalf("default profile resolution failed: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("profile must materialize inside the execution scratch dir, got %q", path)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read materialized profile: %v", err)
	}
	if string(written) != string(defaultSeccompProfile) {
		t.Fatal("materialized profile differs from the embedded bytes")
	}
}

func TestResolveSeccompProfileHonorsOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SeccompProfile = "/custom/profile.json"
	r := &SandboxRunner{cfg: cfg}
	dir := t.TempDir()
	path, err := r.resolveSeccompProfile(dir)
	if err != nil {
		t.Fatalf("override resolution failed: %v", err)
	}
	if path != "/custom/profile.json" {
		t.Fatalf("expected override passthrough, got %q", path)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("override must not write into the scratch dir, found %v", entries)
	}
}

func TestSeccompUnconfinedRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SeccompProfile = "unconfined"
	r := &SandboxRunner{cfg: cfg}
	if _, err := r.resolveSeccompProfile(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "unconfined") {
		t.Fatalf("expected unconfined rejection, got %v", err)
	}
	// Hermetic through the full harness entrypoint (fails before docker).
	if _, err := r.ExecuteHarness("print('x')", "", ""); err == nil ||
		!strings.Contains(err.Error(), "unconfined") {
		t.Fatalf("expected harness-level unconfined rejection, got %v", err)
	}
}

func TestSeccompWriteFailureFailsClosed(t *testing.T) {
	// A scratch dir that cannot hold the profile (here: a path through a
	// regular file) must refuse the run, never run profile-less.
	r := &SandboxRunner{cfg: DefaultConfig()}
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.resolveSeccompProfile(filepath.Join(blocker, "nope")); err == nil ||
		!strings.Contains(err.Error(), "refusing to run without the profile") {
		t.Fatalf("expected fail-closed profile error, got %v", err)
	}

	// Harness level: an unwritable TMPDIR breaks scratch-dir creation, so
	// the run (and its profile write) cannot proceed.
	t.Setenv("TMPDIR", blocker)
	if _, err := r.ExecuteHarness("print('x')", "", ""); err == nil {
		t.Fatal("expected harness refusal when no scratch dir (and hence no profile) is possible")
	}
}

// ---------------------------------------------------------------------------
// Live proof (skip cleanly without a daemon; real against 1.1.0 in CI and
// local runs). Proves the NEW default path: no SeccompProfile override, the
// embedded profile materialized per-run.
// ---------------------------------------------------------------------------

func liveSeccompRunner(t *testing.T) *SandboxRunner {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Timeout = 120 * time.Second
	runner := NewSandboxRunnerWithConfig(cfg)
	if !runner.IsDockerAvailable() {
		t.Skip("docker daemon not available")
	}
	if runner.Config().Image != DefaultRunnerImage {
		t.Skipf("pinned image %s absent (runner fell back to %s): refusing implicit pull",
			DefaultRunnerImage, runner.Config().Image)
	}
	if runner.Config().SeccompProfile != "" {
		t.Fatalf("live proof must run the NEW default path (no SeccompProfile override)")
	}
	return runner
}

func TestLiveSeccompGoldenThroughDefaultPath(t *testing.T) {
	runner := liveSeccompRunner(t)
	res, err := runner.ExecutePython(
		"import math, numpy, scipy.linalg\n" +
			"print(f'GOLDEN={math.sqrt(16.0)},{numpy.pi:.5f},{scipy.linalg.norm([3.0, 4.0])}')")
	if err != nil {
		t.Fatalf("live golden failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "GOLDEN=4.0,3.14159,5.0") {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
	if !strings.Contains(res.IsolationTag, "seccomp=kubona-default") {
		t.Fatalf("isolation tag must name the applied profile, got: %q", res.IsolationTag)
	}
}

func TestLiveSeccompDeniesInetAtSyscall(t *testing.T) {
	runner := liveSeccompRunner(t)
	res, err := runner.ExecutePython(
		"import socket\n" +
			"socket.socket(socket.AF_INET, socket.SOCK_STREAM)\n" +
			"print('INET_CREATED')")
	if err != nil {
		t.Fatalf("live inet probe failed: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("AF_INET socket() must not succeed under the profile: %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "Operation not permitted") {
		t.Fatalf("expected EPERM at the syscall, stderr: %q", res.Stderr)
	}
	if strings.Contains(res.Stdout, "INET_CREATED") {
		t.Fatalf("socket was created despite the profile")
	}

	res, err = runner.ExecutePython(
		"import socket\n" +
			"s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)\n" +
			"s.close()\nprint('UNIX_OK')")
	if err != nil {
		t.Fatalf("live unix probe failed: %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "UNIX_OK") {
		t.Fatalf("AF_UNIX must keep working (stdout %q, stderr %q)", res.Stdout, res.Stderr)
	}
}

func TestDockerArgsNameAndLabelContainers(t *testing.T) {
	r := &SandboxRunner{cfg: DefaultConfig()}
	args := r.dockerArgs("/bench", "/work", "/tmp/scratch/seccomp-profile.json", "aabbccddeeff0011")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--name kubona-sandbox-aabbccdd") {
		t.Fatalf("container must be named kubona-sandbox-<8hex>: %q", joined)
	}
	if !strings.Contains(joined, "--label kubona.managed=sandbox") {
		t.Fatalf("container must carry the managed label: %q", joined)
	}
	if !strings.Contains(joined, "--label kubona.run-id=aabbccddeeff0011") {
		t.Fatalf("container must carry the run id label: %q", joined)
	}
	if r.dockerArgs("/bench", "/work", "/tmp/scratch/seccomp-profile.json", "") != nil {
		t.Fatal("empty run id must refuse the build (no unnamed containers)")
	}
}

func managedNames(t *testing.T) map[string]bool {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-a", "--filter",
		"label=kubona.managed=sandbox", "--format", "{{.Names}}").Output()
	if err != nil {
		t.Fatalf("docker ps failed: %v", err)
	}
	names := map[string]bool{}
	for _, n := range strings.Fields(string(out)) {
		names[n] = true
	}
	return names
}

func managedNamesSince(t *testing.T, before map[string]bool) map[string]bool {
	t.Helper()
	now := managedNames(t)
	for n := range before {
		delete(now, n)
	}
	return now
}

func waitGone(names map[string]bool, timeout time.Duration) map[string]bool {
	deadline := time.Now().Add(timeout)
	remaining := map[string]bool{}
	for k := range names {
		remaining[k] = true
	}
	for len(remaining) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Second)
		cur := managedNamesRaw()
		next := map[string]bool{}
		for k := range remaining {
			if cur[k] {
				next[k] = true
			}
		}
		remaining = next
	}
	return remaining
}

func managedNamesRaw() map[string]bool {
	out, err := exec.Command("docker", "ps", "-a", "--filter",
		"label=kubona.managed=sandbox", "--format", "{{.Names}}").Output()
	if err != nil {
		return map[string]bool{}
	}
	names := map[string]bool{}
	for _, n := range strings.Fields(string(out)) {
		names[n] = true
	}
	return names
}

func TestTimeoutKillsContainerNotJustClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = 8 * time.Second
	runner := &SandboxRunner{cfg: cfg, useDocker: true}
	if out, err := exec.Command("docker", "image", "inspect", cfg.Image).CombinedOutput(); err != nil {
		t.Skipf("pinned image %s absent: %s", cfg.Image, strings.TrimSpace(string(out)))
	}
	before := managedNames(t)
	res, err := runner.ExecutePython("while True:\n pass")
	if err == nil {
		t.Fatal("infinite loop must time out")
	}
	if !errors.Is(err, ErrExecutionTimeout) {
		t.Fatalf("timeout must surface ErrExecutionTimeout, got: %v", err)
	}
	if res == nil || res.ExitCode != 124 {
		t.Fatalf("timeout must report exit 124, got: %+v", res)
	}
	if leaked := waitGone(managedNamesSince(t, before), 30*time.Second); len(leaked) != 0 {
		names := []string{}
		for n := range leaked {
			names = append(names, n)
		}
		t.Fatalf("orphaned sandbox containers: %v", names)
	}
}

func TestReaperSweepsLabelScopedOrphansOnly(t *testing.T) {
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not available: %s", strings.TrimSpace(string(out)))
	}
	name := "kubona-sandbox-reaptest001"
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--label", "kubona.managed=sandbox",
		"--label", "kubona.run-id=reaptest001",
		"--name", name, "--entrypoint", "sleep",
		DefaultRunnerImage, "300").CombinedOutput()
	if err != nil {
		t.Skipf("cannot plant sleeper (image %s?): %s", DefaultRunnerImage, strings.TrimSpace(string(out)))
	}
	defer exec.Command("docker", "rm", "-f", name).Run()
	// Let the planted container age past the threshold (a fresh container
	// younger than maxAge is correctly left alone).
	time.Sleep(1500 * time.Millisecond)
	reaped := ReapStaleSandboxes(time.Second)
	found := false
	for _, n := range reaped {
		if n == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("reaper missed the planted orphan: %v", reaped)
	}
	if left := waitGone(map[string]bool{name: true}, 30*time.Second); len(left) != 0 {
		t.Fatalf("reaped container still present: %v", left)
	}
}
