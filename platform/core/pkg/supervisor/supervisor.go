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
	case LoopCompleted:
		// A tracked job settling is the normal completion path; a paused or
		// stalled loop whose work is already over may settle too.
		switch from {
		case LoopRunning, LoopPaused, LoopRescoped, LoopStalled, LoopSleeping:
			return true
		}
	case LoopAborted:
		// Failure settlement is terminal, never a completion.
		switch from {
		case LoopRunning, LoopPaused, LoopRescoped, LoopDiverged, LoopStalled, LoopSleeping:
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
	_ = s.RehydrateLoops()
	s.sweepWg.Add(1)
	go s.heartbeatWatchdogSweep()
	return s
}

// EvictTerminalLoops drops finished loops from the in-memory maps after a
// retention window. The ledger rows remain (audit); a long-lived daemon must
// not keep every loop it ever ran, and RehydrateLoops would otherwise reload
// all of them at boot.
func (s *Supervisor) EvictTerminalLoops(cutoff time.Time) int {
	if s == nil {
		return 0
	}
	terminal := map[string]struct{}{}
	for _, st := range TerminalLoopStatuses() {
		terminal[st] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, loop := range s.loops {
		if loop == nil || loop.UpdatedAt.After(cutoff) {
			continue
		}
		if _, ok := terminal[loop.Status]; !ok {
			continue
		}
		delete(s.loops, id)
		delete(s.snapshots, id)
		removed++
	}
	return removed
}

// RehydrateLoops loads persisted supervisor loops (rows without a harness_id)
// from SQLite so loops survive daemon restarts instead of vanishing into 404s.
func (s *Supervisor) RehydrateLoops() int {
	if s.ledger == nil {
		return 0
	}
	rows, err := s.ledger.ListLoopStates()
	if err != nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	loaded := 0
	for _, p := range rows {
		if p.HarnessID != "" {
			continue
		}
		if _, exists := s.loops[p.ID]; exists {
			continue
		}
		updated := p.LastActivity
		if updated.IsZero() {
			updated = time.Now().UTC()
		}
		s.loops[p.ID] = &ManagedLoop{
			ID:        p.ID,
			SessionID: p.SessionID,
			Status:    p.Status,
			StepCount: p.StepCount,
			MaxSteps:  p.MaxSteps,
			CreatedAt: updated,
			UpdatedAt: updated,
			pauseCh:   make(chan struct{}),
			killCh:    make(chan struct{}),
		}
		loaded++
	}
	return loaded
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
	sessionID := loop.SessionID
	// Serialize the loops table update under the lock so concurrent steps
	// cannot regress persisted step_count/status out of order.
	_, _ = s.ledger.DB().Exec(
		`UPDATE loops SET status = ?, step_count = ?, diverged = ?, updated_at = ? WHERE id = ?`,
		status, stepCount, diverged, updatedAt, loopID,
	)
	s.mu.Unlock()

	// C10: pending operator steers are consumed at this step boundary.
	s.applySteers(loopID, sessionID)
	partState := "done"
	if diverged {
		partState = "failed"
	}
	_, _ = s.RecordPart(sessionID, LoopPart{
		LoopID: loopID, Helper: "lead",
		Title:   fmt.Sprintf("Optimizer step %d", stepCount),
		State:   partState,
		Now:     fmt.Sprintf("‖∇‖ = %.3g", gradientNorm),
		BeganMS: updatedAt.UnixMilli(), EndedMS: updatedAt.UnixMilli(),
	})

	return status, nil
}

// PauseSessionLoops pauses every running/rescoped supervisor loop owned by
// sessionID and returns the number of loops paused.
func (s *Supervisor) PauseSessionLoops(sessionID string) int {
	if s == nil || sessionID == "" {
		return 0
	}
	s.mu.RLock()
	var ids []string
	for id, loop := range s.loops {
		if loop != nil && loop.SessionID == sessionID && (loop.Status == LoopRunning || loop.Status == LoopRescoped) {
			ids = append(ids, id)
		}
	}
	s.mu.RUnlock()
	paused := 0
	for _, id := range ids {
		if err := s.Pause(id); err == nil {
			paused++
		}
	}
	return paused
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
	// The lock is released before the ledger append + part record: RecordPart
	// takes the read lock, and a nested lock on the same RWMutex would deadlock.
	sessionID, fields, err := func() (string, map[string]any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		loop, exists := s.loops[loopID]
		if !exists {
			return "", nil, fmt.Errorf("loop not found: %s", loopID)
		}
		if loop.Status == to {
			return "", nil, nil
		}
		if !validLoopTransition(loop.Status, to) {
			return "", nil, fmt.Errorf("loop %s cannot go %s -> %s: terminal states are immutable", loopID, loop.Status, to)
		}
		loop.Status = to
		loop.UpdatedAt = time.Now().UTC()
		if _, err := s.ledger.DB().Exec(
			`UPDATE loops SET status = ?, updated_at = ? WHERE id = ?`,
			loop.Status, loop.UpdatedAt, loopID); err != nil {
			return "", nil, fmt.Errorf("loop transition persist failed: %w", err)
		}
		f := map[string]any{"loop_id": loopID}
		for k, v := range extra {
			f[k] = v
		}
		return loop.SessionID, f, nil
	}()
	if err != nil {
		return err
	}
	if fields == nil {
		return nil // same-state is an idempotent no-op: no event, no part
	}
	s.ledger.AppendEvent(sessionID, eventType, fields)
	s.recordTransitionPart(sessionID, loopID, to)
	return nil
}

// recordTransitionPart turns a lifecycle transition into one run lane item.
// Title and state come from a closed mapping — no free text.
func (s *Supervisor) recordTransitionPart(sessionID, loopID, to string) {
	title, state := "Run "+to, "running"
	switch to {
	case LoopPaused:
		title, state = "Paused by operator", "waiting"
	case LoopRunning:
		title, state = "Resumed", "running"
	case LoopRescoped:
		title, state = "Rescoped under approval", "running"
	case LoopCompleted:
		title, state = "Completed", "done"
	case LoopDiverged:
		title, state = "Diverged (guardrail)", "failed"
	case LoopStalled:
		title, state = "Stalled (guardrail)", "failed"
	case LoopAborted:
		title, state = "Aborted", "failed"
	case LoopSleeping:
		title, state = "Sleeping", "waiting"
	case LoopKilled:
		title, state = "Killed", "stopped"
	}
	_, _ = s.RecordPart(sessionID, LoopPart{
		LoopID: loopID, Helper: "lead", Title: title, State: state,
	})
}

// CompleteTrackedJob settles the supervisor loop that was created to track a
// single dispatched job. Launch creates one loop per proposal and records the
// job as its only part; without this hook the job settled while the run card
// stayed "running" forever (step 0/50, no transition event).
func (s *Supervisor) CompleteTrackedJob(sessionID, jobID, outcome string) int {
	s.mu.RLock()
	var candidates []string
	for id, loop := range s.loops {
		if loop.SessionID != sessionID {
			continue
		}
		switch loop.Status {
		case LoopRunning, LoopPaused, LoopRescoped, LoopSleeping:
			candidates = append(candidates, id)
		}
	}
	s.mu.RUnlock()
	settled := 0
	for _, loopID := range candidates {
		parts, err := s.LoopParts(loopID)
		if err != nil {
			continue
		}
		jobIDs := map[string]bool{}
		for _, part := range parts {
			if part.JobID != "" {
				jobIDs[part.JobID] = true
			}
		}
		// Only a loop whose entire tracked work is this one job can be
		// settled here; a multi-job agent run keeps its own lifecycle.
		if len(jobIDs) != 1 || !jobIDs[jobID] {
			continue
		}
		to, event := LoopCompleted, "loop_completed"
		if outcome != "completed" {
			to, event = LoopAborted, "loop_aborted"
		}
		if err := s.transitionLoop(loopID, to, event, map[string]any{
			"job_id": jobID, "outcome": outcome, "reason": "tracked_job_terminal",
		}); err == nil {
			settled++
		}
	}
	return settled
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
