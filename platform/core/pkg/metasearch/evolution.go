package metasearch

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrHumanApprovalRequired = errors.New("cannot apply meta-search modification without human sign-off")
	ErrProposalNotFound      = errors.New("meta-search proposal not found")
)

type MetaProposal struct {
	ID                  string    `json:"id"`
	HarnessID           string    `json:"harness_id"`
	TargetFile          string    `json:"target_file"` // e.g. "search.md"
	CurrentInstructions string    `json:"current_instructions"`
	ProposedDiff        string    `json:"proposed_diff"`
	Rationale           string    `json:"rationale"`
	ApprovedByHuman     bool      `json:"approved_by_human"`
	ApprovedBy          string    `json:"approved_by"`
	Applied             bool      `json:"applied"`
	CreatedAt           time.Time `json:"created_at"`
}

type MetaSearchEngine struct {
	mu        sync.RWMutex
	proposals map[string]*MetaProposal
}

func NewMetaSearchEngine() *MetaSearchEngine {
	return &MetaSearchEngine{
		proposals: make(map[string]*MetaProposal),
	}
}

// ProposeSearchRefinement creates a versioned, diff-reviewable update to search.md
func (e *MetaSearchEngine) ProposeSearchRefinement(
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
		HarnessID:           harnessID,
		TargetFile:          targetFile,
		CurrentInstructions: current,
		ProposedDiff:        diff,
		Rationale:           rationale,
		ApprovedByHuman:     false,
		Applied:             false,
		CreatedAt:           time.Now().UTC(),
	}

	e.proposals[p.ID] = p
	return p
}

// ApproveProposal records human-in-the-loop sign-off
func (e *MetaSearchEngine) ApproveProposal(proposalID, humanSigner string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	p, exists := e.proposals[proposalID]
	if !exists {
		return ErrProposalNotFound
	}
	p.ApprovedByHuman = true
	p.ApprovedBy = humanSigner
	return nil
}

// ApplyProposal applies the refinement to the active harness instruction set
func (e *MetaSearchEngine) ApplyProposal(proposalID string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	p, exists := e.proposals[proposalID]
	if !exists {
		return "", ErrProposalNotFound
	}

	if !p.ApprovedByHuman {
		return "", ErrHumanApprovalRequired
	}

	p.Applied = true
	return fmt.Sprintf("Applied meta-search refinement %s to %s", p.ID, p.TargetFile), nil
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
