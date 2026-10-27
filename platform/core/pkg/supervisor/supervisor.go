package supervisor

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// ReconciliationGate provides single-writer gating with lease auto-expiry (chickensoup pattern)
type ReconciliationGate struct {
	mu        sync.Mutex
	busy      bool
	owner     string
	expiresAt time.Time
	ttl       time.Duration
}

func NewReconciliationGate(ttl time.Duration) *ReconciliationGate {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &ReconciliationGate{ttl: ttl}
}

func (g *ReconciliationGate) Acquire(owner string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	if g.busy && now.Before(g.expiresAt) {
		return false
	}
	g.busy = true
	g.owner = owner
	g.expiresAt = now.Add(g.ttl)
	return true
}

func (g *ReconciliationGate) Release(owner string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.busy && (g.owner == owner || time.Now().After(g.expiresAt)) {
		g.busy = false
		g.owner = ""
	}
}

func (g *ReconciliationGate) IsBusy() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.busy && time.Now().Before(g.expiresAt)
}

// LoopSnapshot captures an immutable state checkpoint for Ring 0 rollback
type LoopSnapshot struct {
	SnapshotID string         `json:"snapshot_id"`
	LoopID     string         `json:"loop_id"`
	StepIndex  int            `json:"step_index"`
	Parameters []float64      `json:"parameters"`
	Metric     float64        `json:"metric"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Loop lifecycle states (W0.21: single source — the loops table is written
// with bare strings in several places, so every transition below goes
// through validLoopTransition; a new status defaults to blocking until
// classified here).
const (
	LoopRunning   = "running"
	LoopPaused    = "paused"
	LoopCompleted = "completed"
	LoopDiverged  = "diverged"
	LoopRescoped  = "rescoped"
	LoopKilled    = "killed"
	LoopStalled   = "stalled"
	LoopAborted   = "aborted"
	LoopSleeping  = "sleeping"
)

// TerminalLoopStatuses are over: killed/completed/aborted loops never run
// again (pkg/session builds its deletion blockers from this set — a killed
// loop must not block session deletion forever).
func TerminalLoopStatuses() []string {
	return []string{LoopCompleted, LoopAborted, LoopKilled}
}

// validLoopTransition reports whether to-state is reachable from from-state.
// Terminal states are immutable (killed/completed/aborted never revive —
// Resume used to resurrect them into spend). Same-state is a no-op success.
func validLoopTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch to {
	case LoopPaused:
		// Parking live or stuck work for inspection (running, rescoped,
		// diverged, stalled). Terminal stays terminal.
		switch from {
		case LoopRunning, LoopRescoped, LoopDiverged, LoopStalled:
			return true
		}
	case LoopRunning:
		// Resume only from parked states (W0.21). Diverged loops recover
		// via RollbackToSnapshot, not Resume; terminal never revives.
		switch from {
		case LoopPaused, LoopStalled, LoopSleeping:
			return true
		}
	case LoopKilled:
		switch from {
		case LoopRunning, LoopPaused, LoopRescoped, LoopDiverged, LoopStalled, LoopSleeping:
			return true
		}
	case LoopRescoped:
		switch from {
		case LoopRunning, LoopDiverged:
			return true
		}
	}
	return false
}

// ManagedLoop represents an active agent autonomous discovery loop
type ManagedLoop struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Status    string    `json:"status"` // "running", "paused", "completed", "diverged", "rescoped", "killed"
	StepCount int       `json:"step_count"`
	MaxSteps  int       `json:"max_steps"`
	Diverged  bool      `json:"diverged"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	LatestSnapshot *LoopSnapshot `json:"latest_snapshot,omitempty"`
	pauseCh        chan struct{}
	killCh         chan struct{}
}

type Supervisor struct {
	ledger    *ledger.Ledger
	gate      *ReconciliationGate
	mu        sync.RWMutex
	loops     map[string]*ManagedLoop
	snapshots map[string][]*LoopSnapshot
	ctx       context.Context
	cancel    context.CancelFunc
	sweepWg   sync.WaitGroup
}

func NewSupervisor(l *ledger.Ledger) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Supervisor{
		ledger:    l,
		gate:      NewReconciliationGate(15 * time.Minute),
		loops:     make(map[string]*ManagedLoop),
		snapshots: make(map[string][]*LoopSnapshot),
		ctx:       ctx,
		cancel:    cancel,
	}
	s.sweepWg.Add(1)
	go s.heartbeatWatchdogSweep()
	return s
}

func (s *Supervisor) Close() {
	s.cancel()
	// W0.21: join the sweep — cancel alone left it racing shutdown.
	s.sweepWg.Wait()
}

func (s *Supervisor) Gate() *ReconciliationGate {
	return s.gate
}

func (s *Supervisor) RegisterLoop(sessionID string, maxSteps int) (*ManagedLoop, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// W0.21: maxSteps bounds spend — callers must state it (the silent
	// 50-default hid caller errors on the spend bound). Ghost sessions
	// refuse: unattributed loops are another lab's bill.
	if maxSteps <= 0 {
		return nil, fmt.Errorf("maxSteps must be positive (spend bound): refusing")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID is required: ghost loops refuse")
	}

	loop := &ManagedLoop{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Status:    LoopRunning,
		StepCount: 0,
		MaxSteps:  maxSteps,
		Diverged:  false,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		pauseCh:   make(chan struct{}),
		killCh:    make(chan struct{}),
	}

	query := `INSERT INTO loops (id, session_id, status, step_count, max_steps, diverged, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.ledger.DB().Exec(query, loop.ID, loop.SessionID, loop.Status, loop.StepCount, loop.MaxSteps, loop.Diverged, loop.CreatedAt, loop.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to register loop: %w", err)
	}

	s.loops[loop.ID] = loop
	s.ledger.AppendEvent(sessionID, "loop_registered", loop)
	return loop, nil
}

// SaveSnapshot checkpoints loop parameters for Ring 0 reflex rollback
func (s *Supervisor) SaveSnapshot(loopID string, params []float64, metric float64) (*LoopSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return nil, fmt.Errorf("loop not found: %s", loopID)
	}

	snap := &LoopSnapshot{
		SnapshotID: uuid.New().String(),
		LoopID:     loopID,
		StepIndex:  loop.StepCount,
		Parameters: append([]float64{}, params...),
		Metric:     metric,
		CreatedAt:  time.Now().UTC(),
	}

	s.snapshots[loopID] = append(s.snapshots[loopID], snap)
	loop.LatestSnapshot = snap
	s.ledger.AppendEvent(loop.SessionID, "loop_snapshot_saved", snap)
	return snap, nil
}

// RollbackToSnapshot restores the last known good snapshot if divergence occurs
func (s *Supervisor) RollbackToSnapshot(loopID string) (*LoopSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return nil, fmt.Errorf("loop not found: %s", loopID)
	}

	snaps := s.snapshots[loopID]
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no snapshots recorded for loop %s", loopID)
	}

	target := snaps[len(snaps)-1]
	// Recovery path, not resurrection: rollback runs only from live states
	// (diverged/stalled/paused/running/rescoped) — completed/killed/aborted
	// history is immutable. The transition persists (W0.21): the old code
	// left the loops table behind memory.
	if loop.Status == LoopCompleted || loop.Status == LoopKilled || loop.Status == LoopAborted {
		return nil, fmt.Errorf("loop %s is %s: rollback refuses terminal history", loopID, loop.Status)
	}
	loop.Status = LoopRunning
	loop.Diverged = false
	loop.StepCount = target.StepIndex
	loop.UpdatedAt = time.Now().UTC()
	if _, err := s.ledger.DB().Exec(
		`UPDATE loops SET status = ?, step_count = ?, diverged = ?, updated_at = ? WHERE id = ?`,
		loop.Status, loop.StepCount, loop.Diverged, loop.UpdatedAt, loopID); err != nil {
		return nil, fmt.Errorf("rollback persist failed: %w", err)
	}

	s.ledger.AppendEvent(loop.SessionID, "loop_rollback_executed", map[string]any{
		"loop_id":     loopID,
		"snapshot_id": target.SnapshotID,
		"step_index":  target.StepIndex,
	})
	return target, nil
}

// RescopeLoop automatically tightens parameter bounds and resets optimizer on divergence
func (s *Supervisor) RescopeLoop(loopID string, newMaxSteps int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return fmt.Errorf("loop not found: %s", loopID)
	}
	if !validLoopTransition(loop.Status, LoopRescoped) && loop.Status != LoopRescoped {
		return fmt.Errorf("loop %s cannot go %s -> rescoped: terminal states are immutable", loopID, loop.Status)
	}

	loop.Status = LoopRescoped
	loop.Diverged = false
	if newMaxSteps > 0 {
		loop.MaxSteps = newMaxSteps
	}
	loop.UpdatedAt = time.Now().UTC()
	if _, err := s.ledger.DB().Exec(
		`UPDATE loops SET status = ?, diverged = ?, max_steps = ?, updated_at = ? WHERE id = ?`,
		loop.Status, loop.Diverged, loop.MaxSteps, loop.UpdatedAt, loopID); err != nil {
		return fmt.Errorf("rescope persist failed: %w", err)
	}

	s.ledger.AppendEvent(loop.SessionID, "loop_rescoped", map[string]any{
		"loop_id":       loopID,
		"new_max_steps": loop.MaxSteps,
		"reason":        "Ring 0 watchdog automatic re-scoping triggered",
	})
	return nil
}

func (s *Supervisor) Step(loopID string, gradientNorm float64) (string, error) {
	s.mu.Lock()
	loop, exists := s.loops[loopID]
	if !exists {
		s.mu.Unlock()
		return "", fmt.Errorf("loop not found: %s", loopID)
	}

	if loop.Status != LoopRunning && loop.Status != LoopRescoped {
		status := loop.Status
		s.mu.Unlock()
		return status, nil
	}

	loop.StepCount++
	loop.UpdatedAt = time.Now().UTC()

	// Barren plateau or gradient explosion reflex trigger
	if math.IsNaN(gradientNorm) || math.IsInf(gradientNorm, 0) || gradientNorm < 1e-6 || gradientNorm > 1e4 {
		loop.Diverged = true
		loop.Status = LoopDiverged
		s.ledger.AppendEvent(loop.SessionID, "loop_diverged", map[string]any{
			"loop_id":       loopID,
			"gradient_norm": gradientNorm,
			"reason":        "Ring 0 watchdog triggered: barren plateau or exploding gradient",
		})
	} else if loop.StepCount >= loop.MaxSteps {
		loop.Status = LoopCompleted
		s.ledger.AppendEvent(loop.SessionID, "loop_completed", loop)
	}

	status := loop.Status
	stepCount := loop.StepCount
	diverged := loop.Diverged
	updatedAt := loop.UpdatedAt
	s.mu.Unlock()

	s.ledger.DB().Exec(
		`UPDATE loops SET status = ?, step_count = ?, diverged = ?, updated_at = ? WHERE id = ?`,
		status, stepCount, diverged, updatedAt, loopID,
	)

	return status, nil
}

// transitionLoop applies a lifecycle transition with the state machine
// enforced and every change persisted (W0.21): transitions used to mutate
// memory only (stall sweep, rescope, rollback) while the loops table said
// "running" forever. Same-state is a no-op success (idempotent operators).
func (s *Supervisor) Pause(loopID string) error {
	return s.transitionLoop(loopID, LoopPaused, "loop_paused", nil)
}

func (s *Supervisor) Resume(loopID string) error {
	return s.transitionLoop(loopID, LoopRunning, "loop_resumed", nil)
}

func (s *Supervisor) Kill(loopID string) error {
	return s.transitionLoop(loopID, LoopKilled, "loop_killed", nil)
}

func (s *Supervisor) transitionLoop(loopID, to string, eventType string, extra map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return fmt.Errorf("loop not found: %s", loopID)
	}
	if loop.Status == to {
		return nil
	}
	if !validLoopTransition(loop.Status, to) {
		return fmt.Errorf("loop %s cannot go %s -> %s: terminal states are immutable", loopID, loop.Status, to)
	}
	loop.Status = to
	loop.UpdatedAt = time.Now().UTC()
	if _, err := s.ledger.DB().Exec(
		`UPDATE loops SET status = ?, updated_at = ? WHERE id = ?`,
		loop.Status, loop.UpdatedAt, loopID); err != nil {
		return fmt.Errorf("loop transition persist failed: %w", err)
	}
	fields := map[string]any{"loop_id": loopID}
	for k, v := range extra {
		fields[k] = v
	}
	s.ledger.AppendEvent(loop.SessionID, eventType, fields)
	return nil
}

// GetLoop returns a copy of one loop (nil,false when unknown). Callers use
// it to bind the loop's session to the requester before acting.
func (s *Supervisor) GetLoop(loopID string) (*ManagedLoop, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.loops[loopID]
	if !ok {
		return nil, false
	}
	cp := *l
	if l.LatestSnapshot != nil {
		snap := *l.LatestSnapshot
		cp.LatestSnapshot = &snap
	}
	return &cp, true
}

func (s *Supervisor) ListLoops() []*ManagedLoop {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// W0.21: copies, never live pointers — callers mutated loop state past
	// the lock (and the race detector's reach).
	res := make([]*ManagedLoop, 0, len(s.loops))
	for _, l := range s.loops {
		cp := *l
		if l.LatestSnapshot != nil {
			snap := *l.LatestSnapshot
			cp.LatestSnapshot = &snap
		}
		res = append(res, &cp)
	}
	return res
}

// heartbeatWatchdogSweep detects stalled subtrees and terminates zombie loops
func (s *Supervisor) heartbeatWatchdogSweep() {
	defer s.sweepWg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now().UTC()
			for _, loop := range s.loops {
				if loop.Status == LoopRunning && now.Sub(loop.UpdatedAt) > 10*time.Minute {
					loop.Status = LoopStalled
					loop.UpdatedAt = now
					// W0.21: the stall persists — the old sweep left the
					// table saying "running" forever.
					_, _ = s.ledger.DB().Exec(
						`UPDATE loops SET status = ?, updated_at = ? WHERE id = ?`,
						loop.Status, loop.UpdatedAt, loop.ID)
					s.ledger.AppendEvent(loop.SessionID, "loop_stalled_timeout", map[string]any{
						"loop_id": loop.ID,
						"timeout": "10m",
					})
				}
			}
			s.mu.Unlock()
		}
	}
}

// IsLocalIPv4 implements gods-eye-view local pin guard ensuring local models stay on 127.0.0.1.
// Host:port forms split first (W0.21): the old code fed "127.0.0.1:8787" to
// ParseIP, got nil, and answered false for a loopback endpoint.
func IsLocalIPv4(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost"
	}
	return ip.IsLoopback()
}
