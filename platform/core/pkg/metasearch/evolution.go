package metasearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

var (
	ErrHumanApprovalRequired = errors.New("cannot apply meta-search modification without human sign-off")
	ErrProposalNotFound      = errors.New("meta-search proposal not found")
)

type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending_approval"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"
	ProposalApplied  ProposalStatus = "applied"
)

type MetaProposal struct {
	ID                  string         `json:"id"`
	LoopID              string         `json:"loop_id"`
	HarnessID           string         `json:"harness_id"`
	TargetFile          string         `json:"target_file"` // e.g. "search.md"
	Status              ProposalStatus `json:"status"`
	CurrentInstructions string         `json:"current_instructions"`
	ProposedDiff        string         `json:"proposed_diff"`
	Rationale           string         `json:"rationale"`
	ApprovedByHuman     bool           `json:"approved_by_human"`
	ApprovedBy          string         `json:"approved_by,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	ResolvedAt          *time.Time     `json:"resolved_at,omitempty"`
}

type MetaSearchEngine struct {
	mu        sync.RWMutex
	proposals map[string]*MetaProposal
	llmClient *llm.Client
	db        *sql.DB
}

func NewMetaSearchEngine() *MetaSearchEngine {
	return NewMetaSearchEngineWithLLM(nil)
}

func NewMetaSearchEngineWithLLM(llmClient *llm.Client) *MetaSearchEngine {
	return &MetaSearchEngine{
		proposals: make(map[string]*MetaProposal),
		llmClient: llmClient,
	}
}

// SetDB attaches SQLite persistence for proposals (K4). Without it the
// engine is memory-only: approvals vanish on restart (fail-closed, but
// the approval UX breaks across reboots). Rows are append-only history —
// rejections stay recorded, never deleted.
func (e *MetaSearchEngine) SetDB(db *sql.DB) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.db = db
	if db == nil {
		return
	}
	rows, err := db.Query(`SELECT id, proposal_json FROM metasearch_proposals`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		var p MetaProposal
		if err := rows.Scan(&id, &raw); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		p.ID = id
		e.proposals[id] = &p
	}
}

// saveLocked upserts one proposal row; caller holds mu.
func (e *MetaSearchEngine) saveLocked(p *MetaProposal) {
	if e.db == nil {
		return
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return
	}
	_, _ = e.db.Exec(
		`INSERT INTO metasearch_proposals (id, proposal_json, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET proposal_json = excluded.proposal_json, updated_at = CURRENT_TIMESTAMP`,
		p.ID, string(raw))
}

// ProposeSearchRefinement creates a versioned, diff-reviewable update to search.md
func (e *MetaSearchEngine) ProposeSearchRefinement(
	loopID string,
	harnessID string,
	targetFile string,
	current string,
	diff string,
	rationale string,
) *MetaProposal {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := &MetaProposal{
		ID:                  uuid.New().String(),
		LoopID:              loopID,
		HarnessID:           harnessID,
		TargetFile:          targetFile,
		Status:              ProposalPending,
		CurrentInstructions: current,
		ProposedDiff:        diff,
		Rationale:           rationale,
		ApprovedByHuman:     false,
		CreatedAt:           time.Now().UTC(),
	}

	e.proposals[p.ID] = p
	e.saveLocked(p)
	return p
}

// GenerateStagnationProposal automatically formulates a refinement when the loop plateaus
func (e *MetaSearchEngine) GenerateStagnationProposal(
	ctx context.Context,
	loopID string,
	harnessID string,
	currentInstructions string,
	championCode string,
	discards int,
) (*MetaProposal, error) {
	rationale := fmt.Sprintf("Autonomous stagnation detected after %d consecutive discards on harness '%s'. Search instructions revised to prevent barren plateaus and broaden exploration.", discards, harnessID)
	diff := ""

	// Check if LLM client is available to reflect on failure trace
	if e.llmClient != nil {
		prompt := fmt.Sprintf(`Section 13 Meta-Search Reflector.
Harness: %s
Consecutive Discards: %d
Current Instructions:
%s

Champion Code:
%s

The optimization loop has stalled. Formulate a unified diff to the instructions that changes search strategy, widens parameter mutation radius, or addresses barren plateaus.
Output ONLY the unified diff.`, harnessID, discards, currentInstructions, championCode)

		msgs := []llm.ChatMessage{
			{Role: "system", Content: "You are the Section 13 Meta-Search Self-Evolution Reflector for Kubona."},
			{Role: "user", Content: prompt},
		}

		respText, _, _, err := e.llmClient.Complete(ctx, "supervisor", msgs)
		if err == nil && respText != "" {
			diff = respText
		}
	}

	// Deterministic algorithmic reflection rule if LLM diff is empty
	if diff == "" {
		switch harnessID {
		case "ansatz-search":
			diff = `--- search.md (active)
+++ search.md (meta-refined)
@@ -10,3 +10,6 @@
- Strategy: Random parameter perturbations around current theta
+ Strategy: Barren plateau mitigation detected (Var(grad) < 1e-6)
+ Action 1: Switch rotation gate initialization from random to identity perturbation (theta ~ N(0, 0.01))
+ Action 2: Alternate linear CNOT topology with circular entangler rings to increase expressibility
+ Action 3: Clamp circuit depth to <= 3 layers before introducing additional 2-qubit gates`

		case "qaoa-combinatorial":
			diff = `--- search.md (active)
+++ search.md (meta-refined)
@@ -5,3 +5,6 @@
- Schedule: Linear angle scanning p=1
+ Schedule: Multi-layer QAOA interpolation (INTERP heuristic)
+ Action 1: Increase layer count p to 3 layers
+ Action 2: Seed (gamma_i, beta_i) variational angles from linear ramp schedule`

		default:
			diff = fmt.Sprintf(`--- search.md (active)
+++ search.md (meta-refined)
@@ -1,3 +1,4 @@
- Standard exploration
+ Broadened exploration radius: scaled mutation perturbation by 2.0x due to %d discards`, discards)
		}
	}

	proposal := e.ProposeSearchRefinement(loopID, harnessID, "search.md", currentInstructions, diff, rationale)
	return proposal, nil
}

// ApproveProposal records human-in-the-loop capability sign-off
func (e *MetaSearchEngine) ApproveProposal(proposalID, humanSigner string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	p, exists := e.proposals[proposalID]
	if !exists {
		return ErrProposalNotFound
	}
	if p.Status != ProposalPending {
		return fmt.Errorf("proposal %s is %s: only pending proposals can be approved", proposalID, p.Status)
	}
	p.ApprovedByHuman = true
	p.ApprovedBy = humanSigner
	p.Status = ProposalApproved
	now := time.Now().UTC()
	p.ResolvedAt = &now
	e.saveLocked(p)
	return nil
}

// RejectProposal records human rejection
func (e *MetaSearchEngine) RejectProposal(proposalID, humanSigner string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	p, exists := e.proposals[proposalID]
	if !exists {
		return ErrProposalNotFound
	}
	if p.Status != ProposalPending {
		return fmt.Errorf("proposal %s is %s: only pending proposals can be rejected", proposalID, p.Status)
	}
	p.ApprovedByHuman = false
	p.ApprovedBy = humanSigner
	p.Status = ProposalRejected
	now := time.Now().UTC()
	p.ResolvedAt = &now
	e.saveLocked(p)
	return nil
}

// ApplyProposal records an approved refinement as applied. K4 honesty:
// this writes NO files — applying a diff to live harness instructions
// is operator action until Phase-5 auto-apply, and writing
// attacker-influenced diffs from an HTTP handler would be a path and
// content hazard. The status + persisted row + ledger event (written by
// the caller) are the application record; the message says so.
func (e *MetaSearchEngine) ApplyProposal(proposalID string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	p, exists := e.proposals[proposalID]
	if !exists {
		return "", ErrProposalNotFound
	}

	if !p.ApprovedByHuman || p.Status != ProposalApproved {
		return "", ErrHumanApprovalRequired
	}

	p.Status = ProposalApplied
	e.saveLocked(p)
	return fmt.Sprintf("Recorded approved Section 13 refinement %s for %s (no files written — application is operator action)", p.ID, p.TargetFile), nil
}

func (e *MetaSearchEngine) ListProposals() []*MetaProposal {
	e.mu.RLock()
	defer e.mu.RUnlock()

	list := make([]*MetaProposal, 0, len(e.proposals))
	for _, p := range e.proposals {
		list = append(list, p)
	}
	return list
}

func (e *MetaSearchEngine) GetProposal(proposalID string) (*MetaProposal, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	p, exists := e.proposals[proposalID]
	if !exists {
		return nil, ErrProposalNotFound
	}
	return p, nil
}
