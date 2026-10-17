package epistemics

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
		// W1.10: 0.90, matching the tier's documented floor (0.90 <= ρ).
		// The old 0.85 let the label promise more than the number carried.
		return 0.90
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

// ContentHash computes the canonical SHA-256 digest of the computational hypothesis tuple.
// W1.10: covers the FULL tuple (HypothesisID included) at full float
// precision — the old %.4f/%.6f rounding aliased distinct hypotheses to one
// hash, and the omitted ID let two hypotheses share it.
func (h *ComputationalHypothesis) ContentHash() string {
	raw := strings.Join([]string{
		h.HypothesisID,
		h.Lean4Statement,
		h.AnalyticalAnchorID,
		h.QIRCircuitBitcode,
		strconv.FormatFloat(h.SpendBudgetUSD, 'g', -1, 64),
		strconv.FormatFloat(h.ExpectedEntropyGain, 'g', -1, 64),
	}, ":")
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	return hex.EncodeToString(hasher.Sum(nil))
}

// FisherStoppingRuleEvaluator assesses whether E[I_Fisher] <= λ_AP2 * (dCost/dStep)
type FisherStoppingRuleEvaluator struct {
	LambdaAP2 float64 `json:"lambda_ap2"`
}

func NewFisherStoppingRuleEvaluator(lambdaAP2 float64) (*FisherStoppingRuleEvaluator, error) {
	// W1.10: a non-positive stopping weight silently became 1.0, hiding
	// misconfiguration of the thing that decides when to stop spending.
	if lambdaAP2 <= 0 {
		return nil, fmt.Errorf("lambda_ap2 must be positive, got %v: refusing", lambdaAP2)
	}
	return &FisherStoppingRuleEvaluator{LambdaAP2: lambdaAP2}, nil
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

// StartTribunal initializes a new tribunal session for a hypothesis.
// The session ID is the FULL content hash (W1.10): the old 12-hex prefix
// (48 bits) collided and silently overwrote the earlier session. An ID that
// already exists refuses instead of overwriting — two tribunals never share
// a session.
func (e *EpistemicTribunalEngine) StartTribunal(h ComputationalHypothesis) (*TribunalSession, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	id := fmt.Sprintf("tribunal-%s", h.ContentHash())
	if _, exists := e.sessions[id]; exists {
		return nil, fmt.Errorf("tribunal session %s already exists: refusing to overwrite", id)
	}
	session := &TribunalSession{
		SessionID:         id,
		Hypothesis:        h,
		CurrentPhase:      PhaseProposal,
		Votes:             make(map[string]PersonaVote),
		Verdict:           "PENDING",
		ZeroAxiomCheating: true, // Default expectation
	}
	e.sessions[session.SessionID] = session
	return session, nil
}

// requirePhase enforces the ordered state machine (W1.10): proposal →
// challenge → verification → audit → voting → verdict. Skipping phases
// used to conclude without challenge, verification, or audit.
func requirePhase(s *TribunalSession, want TribunalPhase) error {
	if s.CurrentPhase != want {
		return fmt.Errorf("tribunal %s is %s: phase %s required first", s.SessionID, s.CurrentPhase, want)
	}
	return nil
}

// AdvanceToChallenge moves to Phase 2 (Adversarial Challenge)
func (e *EpistemicTribunalEngine) AdvanceToChallenge(sessionID string, barrenPlateauSafe bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return errors.New("tribunal session not found")
	}
	if err := requirePhase(s, PhaseProposal); err != nil {
		return err
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
	if err := requirePhase(s, PhaseAdversarialChallenge); err != nil {
		return err
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
	if err := requirePhase(s, PhaseFormalVerification); err != nil {
		return err
	}
	s.AnchorConsistent = consistent
	s.CurrentPhase = PhaseAnalyticalAudit
	return nil
}

// CastVote records an agent's vote. Confidence must be in (0, 1] (W1.10):
// zero/negative confidences used to inflate to 0.5 and count toward quorum.
// First vote must arrive in the audit phase (ordered machine); later votes
// in the voting phase.
func (e *EpistemicTribunalEngine) CastVote(sessionID string, vote PersonaVote) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return errors.New("tribunal session not found")
	}
	if vote.Confidence <= 0 || vote.Confidence > 1 {
		return fmt.Errorf("vote confidence must be in (0, 1], got %v: refusing", vote.Confidence)
	}
	if vote.AgentPersona == "" {
		return errors.New("vote persona is required: refusing")
	}
	if len(s.Votes) == 0 {
		if err := requirePhase(s, PhaseAnalyticalAudit); err != nil {
			return err
		}
	} else if err := requirePhase(s, PhaseQuorumVoting); err != nil {
		return err
	}
	s.CurrentPhase = PhaseQuorumVoting
	s.Votes[vote.AgentPersona] = vote
	return nil
}

// minCommitPersonas floors commit diversity (W1.10): one persona voting
// with itself used to reach 100% quorum and COMMIT. Fewer than three
// distinct personas concludes REJECTED (honestly labeled), never committed.
const minCommitPersonas = 3

// ConcludeTribunal calculates quorum consensus and renders verdict.
// Concluding requires the voting phase; COMMIT additionally requires
// Lean4Verified (formal proof passed — the old code committed without it),
// the hygiene triple, and persona diversity.
func (e *EpistemicTribunalEngine) ConcludeTribunal(sessionID string) (*TribunalSession, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.sessions[sessionID]
	if !ok {
		return nil, errors.New("tribunal session not found")
	}
	if err := requirePhase(s, PhaseQuorumVoting); err != nil {
		return nil, err
	}

	totalVotes := len(s.Votes)
	if totalVotes == 0 {
		return nil, errors.New("cannot conclude tribunal with zero votes")
	}

	corroboratedWeighted := 0.0
	totalWeight := 0.0

	for _, v := range s.Votes {
		totalWeight += v.Confidence

		if v.Vote == VoteCorroborated {
			corroboratedWeighted += v.Confidence
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
	// Supermajority threshold: >= 75% quorum + verified formal proof +
	// strict axiomatic hygiene + no barren plateau + anchor consistency +
	// persona diversity. Anything less rejects with the reason named.
	switch {
	case totalVotes < minCommitPersonas:
		s.Verdict = "REJECTED_INSUFFICIENT_CONSENSUS"
	case !s.Lean4Verified:
		s.Verdict = "REJECTED_UNVERIFIED_FORMAL_PROOF"
	case quorum >= 75.0 && s.ZeroAxiomCheating && s.BarrenPlateauSafe && s.AnchorConsistent:
		s.Verdict = "COMMITTED_TO_LEDGER"
	case !s.ZeroAxiomCheating || !s.BarrenPlateauSafe:
		s.Verdict = "FALSIFIED_NEGATIVE_CERTIFICATE"
	default:
		s.Verdict = "REJECTED_INSUFFICIENT_CONSENSUS"
	}

	cp := *s
	cp.Votes = make(map[string]PersonaVote, len(s.Votes))
	for k, v := range s.Votes {
		cp.Votes[k] = v
	}
	return &cp, nil
}

// GetSession returns a copy: callers used to mutate sessions past the lock
// (W1.10 hygiene).
func (e *EpistemicTribunalEngine) GetSession(sessionID string) (*TribunalSession, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.sessions[sessionID]
	if !ok {
		return nil, false
	}
	cp := *s
	cp.Votes = make(map[string]PersonaVote, len(s.Votes))
	for k, v := range s.Votes {
		cp.Votes[k] = v
	}
	return &cp, true
}
