package disk

// S2 — disk guard with hysteresis.
//
// Threat (G2): a runaway job, HF download, or bridge artefact fills the
// state disk; previously an unmodelled outage (no measurement existed).
// The guard polls the filesystems holding Kubona state and freezes new
// work before the disk does: WARN at 2 GB (event + health only), BLOCK
// below 1 GB (503 storage_blocked on spend/new-work entries, dispatch
// frozen), RESUME above 1.5 GB. Reads, health, and recovery stay available
// throughout. Unmeasurable disks block fail-closed.
//
// Both hysteresis bands hold their state against flapping: warning clears
// only above warn*1.2, blocked clears only above resume. No process is
// ever killed here (that needs cgroup work — separate decision); the
// guard refuses admission, it does not reap.
//
// All tunables live in Thresholds (env-overridable, validated at boot).

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Levels. Warning never blocks; blocked refuses new work.
const (
	LevelOK      = "ok"
	LevelWarning = "warning"
	LevelBlocked = "blocked"
)

// Defaults (Q2 decision): warn 2 GB, block 1 GB, resume 1.5 GB. A 1 GB
// floor still covers ~200x the steady-state working set (~5 MB excl.
// models); Cloudroom-class 5 GB warns would brick a quarter of a small VPS.
const (
	DefaultWarnBytes   int64 = 2 * 1024 * 1024 * 1024
	DefaultBlockBytes  int64 = 1 * 1024 * 1024 * 1024
	DefaultResumeBytes int64 = 1536 * 1024 * 1024

	DefaultPollSeconds = 15
	MinPollSeconds     = 5
)

// Thresholds is the single source for the guard's trip points — never a
// copied literal elsewhere (production bar: drift-proof shared values).
type Thresholds struct {
	WarnBytes   int64
	BlockBytes  int64
	ResumeBytes int64
}

// DefaultThresholds returns the Q2-decided defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{WarnBytes: DefaultWarnBytes, BlockBytes: DefaultBlockBytes, ResumeBytes: DefaultResumeBytes}
}

// Validate enforces 0 < block < resume < warn. Invalid config refuses to
// boot (fail-closed config, not a runtime surprise).
func (t Thresholds) Validate() error {
	if t.BlockBytes <= 0 || t.ResumeBytes <= 0 || t.WarnBytes <= 0 {
		return fmt.Errorf("disk thresholds must all be positive (warn=%d block=%d resume=%d)", t.WarnBytes, t.BlockBytes, t.ResumeBytes)
	}
	if !(t.BlockBytes < t.ResumeBytes && t.ResumeBytes < t.WarnBytes) {
		return fmt.Errorf("disk thresholds must satisfy 0 < block < resume < warn (got block=%d resume=%d warn=%d)", t.BlockBytes, t.ResumeBytes, t.WarnBytes)
	}
	return nil
}

func envBytes(key string, fallback int64) (int64, error) {
	if raw := os.Getenv(key); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("env %s=%q is not an integer byte count: %w", key, raw, err)
		}
		return v, nil
	}
	return fallback, nil
}

// ThresholdsFromEnv applies KUBONA_DISK_{WARN,BLOCK,RESUME}_BYTES over the
// defaults. Malformed values error (boot refuses).
func ThresholdsFromEnv() (Thresholds, error) {
	var err error
	t := Thresholds{}
	if t.WarnBytes, err = envBytes("KUBONA_DISK_WARN_BYTES", DefaultWarnBytes); err != nil {
		return t, err
	}
	if t.BlockBytes, err = envBytes("KUBONA_DISK_BLOCK_BYTES", DefaultBlockBytes); err != nil {
		return t, err
	}
	if t.ResumeBytes, err = envBytes("KUBONA_DISK_RESUME_BYTES", DefaultResumeBytes); err != nil {
		return t, err
	}
	if err := t.Validate(); err != nil {
		return t, err
	}
	return t, nil
}

// PollSecondsFromEnv reads KUBONA_DISK_POLL_SECONDS (default 15, min 5 —
// statfs storms help nobody).
func PollSecondsFromEnv() (int, error) {
	raw := os.Getenv("KUBONA_DISK_POLL_SECONDS")
	if raw == "" {
		return DefaultPollSeconds, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < MinPollSeconds {
		return 0, fmt.Errorf("env KUBONA_DISK_POLL_SECONDS=%q must be an integer >= %d", raw, MinPollSeconds)
	}
	return v, nil
}

// MeasureAvail returns free bytes for the filesystem holding path
// (Bavail excludes root-reserved blocks — the honest number for an
// unprivileged daemon).
func MeasureAvail(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q failed: %w", path, err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// Snapshot is the guard's current reading.
type Snapshot struct {
	Level      string
	AvailBytes uint64
	Paths      []string
	Reason     string
	CheckedAt  time.Time
}

// Guard polls watch paths and latches hysteresis bands. Use PollOnce in
// tests and Start in production; both go through the same transition.
type Guard struct {
	mu           sync.RWMutex
	cfg          Thresholds
	paths        []string
	measure      func(path string) (uint64, error)
	onTransition func(Snapshot)
	snap         Snapshot
}

// NewGuard builds a guard. A nil measurer uses MeasureAvail. A nil
// onTransition disables transition callbacks (tests).
func NewGuard(cfg Thresholds, paths []string, measure func(path string) (uint64, error), onTransition func(Snapshot)) *Guard {
	if measure == nil {
		measure = MeasureAvail
	}
	cp := append([]string{}, paths...)
	return &Guard{cfg: cfg, paths: cp, measure: measure, onTransition: onTransition,
		snap: Snapshot{Level: LevelOK, Paths: cp}}
}

// PollOnce measures every watch path (minimum wins — guarding / while state
// lives elsewhere is fiction) and advances the hysteresis bands.
func (g *Guard) PollOnce() {
	avail := ^uint64(0)
	reason := ""
	for _, p := range g.paths {
		free, err := g.measure(p)
		if err != nil {
			// Fail-closed: an unmeasurable disk is a blocked disk. The
			// reason names the path so the operator knows where to look.
			g.transition(LevelBlocked, 0, fmt.Sprintf("disk measurement failed for %s: %v", p, err))
			return
		}
		if free < avail {
			avail = free
		}
	}
	var next string
	g.mu.RLock()
	cur := g.snap.Level
	g.mu.RUnlock()
	b, r, w := uint64(g.cfg.BlockBytes), uint64(g.cfg.ResumeBytes), uint64(g.cfg.WarnBytes)
	switch cur {
	case LevelBlocked:
		// Blocked clears only above resume (never at the boundary).
		if avail > r {
			next = LevelOK
			reason = fmt.Sprintf("disk space recovered: %d bytes free", avail)
		} else {
			next = LevelBlocked
			reason = fmt.Sprintf("disk still below resume threshold: %d bytes free", avail)
		}
	case LevelWarning:
		if avail < b {
			next = LevelBlocked
			reason = fmt.Sprintf("disk space critical: %d bytes free", avail)
		} else if avail >= uint64(float64(w)*1.2) {
			next = LevelOK
			reason = fmt.Sprintf("disk space recovered: %d bytes free", avail)
		} else {
			next = LevelWarning
			reason = fmt.Sprintf("disk space low: %d bytes free", avail)
		}
	default:
		if avail < b {
			next = LevelBlocked
			reason = fmt.Sprintf("disk space critical: %d bytes free", avail)
		} else if avail < w {
			next = LevelWarning
			reason = fmt.Sprintf("disk space low: %d bytes free", avail)
		} else {
			next = LevelOK
		}
	}
	g.transition(next, avail, reason)
}

func (g *Guard) transition(next string, avail uint64, reason string) {
	g.mu.Lock()
	changed := g.snap.Level != next
	g.snap = Snapshot{Level: next, AvailBytes: avail, Paths: append([]string{}, g.paths...), Reason: reason, CheckedAt: time.Now().UTC()}
	cb := g.onTransition
	g.mu.Unlock()
	if changed && cb != nil {
		cb(g.Snapshot())
	}
}

// Start polls until ctx ends. One poller per daemon (single-writer flock
// guarantees one daemon per state dir, so one poller per guard).
func (g *Guard) Start(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			g.PollOnce()
		}
	}
}

// Blocks reports whether new work must be refused.
func (g *Guard) Blocks() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.snap.Level == LevelBlocked
}

// Snapshot returns the current reading.
func (g *Guard) Snapshot() Snapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.snap
}
