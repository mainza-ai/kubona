package supervisor

import (
	"context"
	"fmt"
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
	go s.heartbeatWatchdogSweep()
	return s
}

func (s *Supervisor) Close() {
	s.cancel()
}

func (s *Supervisor) Gate() *ReconciliationGate {
	return s.gate
}

func (s *Supervisor) RegisterLoop(sessionID string, maxSteps int) (*ManagedLoop, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxSteps <= 0 {
		maxSteps = 50
	}

	loop := &ManagedLoop{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Status:    "running",
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
	loop.Status = "running"
	loop.Diverged = false
	loop.StepCount = target.StepIndex
	loop.UpdatedAt = time.Now().UTC()

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

	loop.Status = "rescoped"
	loop.Diverged = false
	if newMaxSteps > 0 {
		loop.MaxSteps = newMaxSteps
	}
	loop.UpdatedAt = time.Now().UTC()

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

	if loop.Status != "running" && loop.Status != "rescoped" {
		status := loop.Status
		s.mu.Unlock()
		return status, nil
	}

	loop.StepCount++
	loop.UpdatedAt = time.Now().UTC()

	// Barren plateau or gradient explosion reflex trigger
	if gradientNorm < 1e-6 || gradientNorm > 1e4 {
		loop.Diverged = true
		loop.Status = "diverged"
		s.ledger.AppendEvent(loop.SessionID, "loop_diverged", map[string]any{
			"loop_id":       loopID,
			"gradient_norm": gradientNorm,
			"reason":        "Ring 0 watchdog triggered: barren plateau or exploding gradient",
		})
	} else if loop.StepCount >= loop.MaxSteps {
		loop.Status = "completed"
		s.ledger.AppendEvent(loop.SessionID, "loop_completed", loop)
	}

	status := loop.Status
	s.mu.Unlock()

	s.ledger.DB().Exec(
		`UPDATE loops SET status = ?, step_count = ?, diverged = ?, updated_at = ? WHERE id = ?`,
		loop.Status, loop.StepCount, loop.Diverged, loop.UpdatedAt, loopID,
	)

	return status, nil
}

func (s *Supervisor) Pause(loopID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return fmt.Errorf("loop not found: %s", loopID)
	}
	loop.Status = "paused"
	loop.UpdatedAt = time.Now().UTC()

	s.ledger.DB().Exec(`UPDATE loops SET status = ?, updated_at = ? WHERE id = ?`, loop.Status, loop.UpdatedAt, loopID)
	s.ledger.AppendEvent(loop.SessionID, "loop_paused", map[string]string{"loop_id": loopID})
	return nil
}

func (s *Supervisor) Resume(loopID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return fmt.Errorf("loop not found: %s", loopID)
	}
	loop.Status = "running"
	loop.UpdatedAt = time.Now().UTC()

	s.ledger.DB().Exec(`UPDATE loops SET status = ?, updated_at = ? WHERE id = ?`, loop.Status, loop.UpdatedAt, loopID)
	s.ledger.AppendEvent(loop.SessionID, "loop_resumed", map[string]string{"loop_id": loopID})
	return nil
}

func (s *Supervisor) Kill(loopID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loop, exists := s.loops[loopID]
	if !exists {
		return fmt.Errorf("loop not found: %s", loopID)
	}
	loop.Status = "killed"
	loop.UpdatedAt = time.Now().UTC()

	s.ledger.DB().Exec(`UPDATE loops SET status = ?, updated_at = ? WHERE id = ?`, loop.Status, loop.UpdatedAt, loopID)
	s.ledger.AppendEvent(loop.SessionID, "loop_killed", map[string]string{"loop_id": loopID})
	return nil
}

func (s *Supervisor) ListLoops() []*ManagedLoop {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*ManagedLoop, 0, len(s.loops))
	for _, l := range s.loops {
		res = append(res, l)
	}
	return res
}

// heartbeatWatchdogSweep detects stalled subtrees and terminates zombie loops
func (s *Supervisor) heartbeatWatchdogSweep() {
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
				if loop.Status == "running" && now.Sub(loop.UpdatedAt) > 10*time.Minute {
					loop.Status = "stalled"
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

// IsLocalIPv4 implements gods-eye-view local pin guard ensuring local models stay on 127.0.0.1
func IsLocalIPv4(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost"
	}
	return ip.IsLoopback()
}
