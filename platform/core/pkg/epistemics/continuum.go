package epistemics

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// EpistemicContinuumTier represents the 5-tier credibility continuum defined in Section 1.2
type EpistemicContinuumTier int

const (
	Tier5GenerativeHypothesis       EpistemicContinuumTier = iota // ρ < 0.70
	Tier4ErrorMitigatedQuantum                                    // 0.70 <= ρ < 0.90
	Tier3CertifiedClassicalBaseline                               // 0.90 <= ρ < 1.00
	Tier2ExactAnalyticalAnchor                                    // ρ -> 1.00-
	Tier1FormalDeductive                                          // ρ = 1.000000
)

func (t EpistemicContinuumTier) String() string {
	switch t {
	case Tier1FormalDeductive:
		return "TIER_1_FORMAL_DEDUCTIVE"
	case Tier2ExactAnalyticalAnchor:
		return "TIER_2_EXACT_ANALYTICAL_ANCHOR"
	case Tier3CertifiedClassicalBaseline:
		return "TIER_3_CERTIFIED_CLASSICAL_BASELINE"
	case Tier4ErrorMitigatedQuantum:
		return "TIER_4_ERROR_MITIGATED_QUANTUM"
	default:
		return "TIER_5_GENERATIVE_HYPOTHESIS"
	}
}

func (t EpistemicContinuumTier) BaseConfidence() float64 {
	switch t {
	case Tier1FormalDeductive:
		return 1.00
	case Tier2ExactAnalyticalAnchor:
		return 0.95
	case Tier3CertifiedClassicalBaseline:
		return 0.85
	case Tier4ErrorMitigatedQuantum:
		return 0.65
	default:
		return 0.20
	}
}

// ComputationalHypothesis defines the typed scientific discovery tuple:
// H = ⟨S_Lean4, A_anchor, C_QIR, B_AP2, ΔS_target⟩
type ComputationalHypothesis struct {
	HypothesisID        string                 `json:"hypothesis_id"`
	Lean4Statement      string                 `json:"lean4_statement"`
	AnalyticalAnchorID  string                 `json:"analytical_anchor_id"`
	QIRCircuitBitcode   string                 `json:"qir_circuit_bitcode"`
	SpendBudgetUSD      float64                `json:"spend_budget_usd"`
	ExpectedEntropyGain float64                `json:"expected_entropy_gain"`
	CreatedTimestamp    time.Time              `json:"created_timestamp"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

// ContentHash computes the canonical SHA-256 digest of the computational hypothesis tuple
func (h *ComputationalHypothesis) ContentHash() string {
	raw := fmt.Sprintf("%s:%s:%s:%.4f:%.6f",
		h.Lean4Statement,
		h.AnalyticalAnchorID,
		h.QIRCircuitBitcode,
		h.SpendBudgetUSD,
		h.ExpectedEntropyGain,
	)
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	return hex.EncodeToString(hasher.Sum(nil))
}

// FisherStoppingRuleEvaluator assesses whether E[I_Fisher] <= λ_AP2 * (dCost/dStep)
type FisherStoppingRuleEvaluator struct {
	LambdaAP2 float64 `json:"lambda_ap2"`
}

func NewFisherStoppingRuleEvaluator(lambdaAP2 float64) *FisherStoppingRuleEvaluator {
	if lambdaAP2 <= 0 {
		lambdaAP2 = 1.0
	}
	return &FisherStoppingRuleEvaluator{LambdaAP2: lambdaAP2}
}

// ShouldStop returns true if the expected information gain is less than or equal to marginal cost
func (e *FisherStoppingRuleEvaluator) ShouldStop(expectedFisherInfo float64, marginalCostPerStep float64) bool {
	costThreshold := e.LambdaAP2 * marginalCostPerStep
	return expectedFisherInfo <= costThreshold
}

// NegativeCertificate permanently records a verified refutation to prune future search spaces
type NegativeCertificate struct {
	CertificateID     string                 `json:"certificate_id"`
	FailureType       string                 `json:"failure_type"` // e.g. BARREN_PLATEAU, COUNTEREXAMPLE, DIVERGENCE
	HypothesisHash    string                 `json:"hypothesis_hash"`
	TargetSystem      string                 `json:"target_system"`
	PrunedSubspace    string                 `json:"pruned_subspace"`
	DiscoveredByAgent string                 `json:"discovered_by_agent"`
	Timestamp         time.Time              `json:"timestamp"`
	Details           map[string]interface{} `json:"details,omitempty"`
}

func (nc *NegativeCertificate) Hash() string {
	raw := fmt.Sprintf("%s:%s:%s:%s", nc.FailureType, nc.HypothesisHash, nc.TargetSystem, nc.PrunedSubspace)
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	return hex.EncodeToString(hasher.Sum(nil))
}

// TribunalVoteKind represents the vote of an agent persona
type TribunalVoteKind string

const (
	VoteCorroborated TribunalVoteKind = "CORROBORATED"
	VoteContested    TribunalVoteKind = "CONTESTED"
	VoteFalsified    TribunalVoteKind = "FALSIFIED"
	VoteAbstain      TribunalVoteKind = "ABSTAIN"
)

// PersonaVote records an individual agent's vote in the tribunal
type PersonaVote struct {
	AgentPersona string           `json:"agent_persona"`
	Vote         TribunalVoteKind `json:"vote"`
	Confidence   float64          `json:"confidence"` // 0.0 to 1.0
	Rationale    string           `json:"rationale"`
	Timestamp    time.Time        `json:"timestamp"`
}

// TribunalPhase represents the 5 phases of the Epistemic Tribunal
type TribunalPhase string

const (
	PhaseProposal             TribunalPhase = "PHASE_1_PROPOSAL"
	PhaseAdversarialChallenge TribunalPhase = "PHASE_2_ADVERSARIAL_CHALLENGE"
	PhaseFormalVerification   TribunalPhase = "PHASE_3_FORMAL_VERIFICATION"
	PhaseAnalyticalAudit      TribunalPhase = "PHASE_4_ANALYTICAL_ANCHOR_AUDIT"
	PhaseQuorumVoting         TribunalPhase = "PHASE_5_QUORUM_VOTING"
	PhaseVerdictRendered      TribunalPhase = "PHASE_VERDICT_RENDERED"
)

// TribunalSession manages an adversarial tribunal proceeding
type TribunalSession struct {
	SessionID         string                  `json:"session_id"`
	Hypothesis        ComputationalHypothesis `json:"hypothesis"`
	CurrentPhase      TribunalPhase           `json:"current_phase"`
	Lean4Verified     bool                    `json:"lean4_verified"`
	ZeroAxiomCheating bool                    `json:"zero_axiom_cheating"`
	BarrenPlateauSafe bool                    `json:"barren_plateau_safe"`
	AnchorConsistent  bool                    `json:"anchor_consistent"`
	Votes             map[string]PersonaVote  `json:"votes"`
	Verdict           string                  `json:"verdict"` // COMMITTED_TO_LEDGER, REJECTED, SUSPENDED
	QuorumPercentage  float64                 `json:"quorum_percentage"`
	CompletedAt       *time.Time              `json:"completed_at,omitempty"`
}

// EpistemicTribunalEngine orchestrates the multi-agent tribunal process
type EpistemicTribunalEngine struct {
	mu       sync.RWMutex
	sessions map[string]*TribunalSession
}

func NewEpistemicTribunalEngine() *EpistemicTribunalEngine {
	return &EpistemicTribunalEngine{
		sessions: make(map[string]*TribunalSession),
	}
}

// StartTribunal initializes a new tribunal session for a hypothesis
func (e *EpistemicTribunalEngine) StartTribunal(h ComputationalHypothesis) *TribunalSession {
	e.mu.Lock()
	defer e.mu.Unlock()

	session := &TribunalSession{
		SessionID:         fmt.Sprintf("tribunal-%s", h.ContentHash()[:12]),
		Hypothesis:        h,
		CurrentPhase:      PhaseProposal,
		Votes:             make(map[string]PersonaVote),
		Verdict:           "PENDING",
		ZeroAxiomCheating: true, // Default expectation
	}
	e.sessions[session.SessionID] = session
	return session
}

// AdvanceToChallenge moves to Phase 2 (Adversarial Challenge)
func (e *EpistemicTribunalEngine) AdvanceToChallenge(sessionID string, barrenPlateauSafe bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return errors.New("tribunal session not found")
	}
	s.BarrenPlateauSafe = barrenPlateauSafe
	s.CurrentPhase = PhaseAdversarialChallenge
	return nil
}

// AdvanceToFormalVerification moves to Phase 3 (Lean 4 check)
func (e *EpistemicTribunalEngine) AdvanceToFormalVerification(sessionID string, lean4Verified, zeroCheat bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return errors.New("tribunal session not found")
	}
	s.Lean4Verified = lean4Verified
	s.ZeroAxiomCheating = zeroCheat
	s.CurrentPhase = PhaseFormalVerification
	return nil
}

// AdvanceToAnchorAudit moves to Phase 4 (Analytical Anchor verification)
func (e *EpistemicTribunalEngine) AdvanceToAnchorAudit(sessionID string, consistent bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return errors.New("tribunal session not found")
	}
	s.AnchorConsistent = consistent
	s.CurrentPhase = PhaseAnalyticalAudit
	return nil
}

// CastVote records an agent's vote in Phase 5
func (e *EpistemicTribunalEngine) CastVote(sessionID string, vote PersonaVote) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return errors.New("tribunal session not found")
	}
	s.CurrentPhase = PhaseQuorumVoting
	s.Votes[vote.AgentPersona] = vote
	return nil
}

// ConcludeTribunal calculates quorum consensus and renders verdict
func (e *EpistemicTribunalEngine) ConcludeTribunal(sessionID string) (*TribunalSession, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return nil, errors.New("tribunal session not found")
	}

	totalVotes := len(s.Votes)
	if totalVotes == 0 {
		return nil, errors.New("cannot conclude tribunal with zero votes")
	}

	corroboratedWeighted := 0.0
	totalWeight := 0.0

	for _, v := range s.Votes {
		weight := v.Confidence
		if weight <= 0 {
			weight = 0.5
		}
		totalWeight += weight

		if v.Vote == VoteCorroborated {
			corroboratedWeighted += weight
		}
	}

	quorum := 0.0
	if totalWeight > 0 {
		quorum = (corroboratedWeighted / totalWeight) * 100.0
	}
	s.QuorumPercentage = quorum

	now := time.Now()
	s.CompletedAt = &now
	s.CurrentPhase = PhaseVerdictRendered

	// Supermajority threshold: >= 75% quorum + strict axiomatic hygiene + no barren plateau
	if quorum >= 75.0 && s.ZeroAxiomCheating && s.BarrenPlateauSafe && s.AnchorConsistent {
		s.Verdict = "COMMITTED_TO_LEDGER"
	} else if !s.ZeroAxiomCheating || !s.BarrenPlateauSafe {
		s.Verdict = "FALSIFIED_NEGATIVE_CERTIFICATE"
	} else {
		s.Verdict = "REJECTED_INSUFFICIENT_CONSENSUS"
	}

	return s, nil
}

func (e *EpistemicTribunalEngine) GetSession(sessionID string) (*TribunalSession, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.sessions[sessionID]
	return s, ok
}
