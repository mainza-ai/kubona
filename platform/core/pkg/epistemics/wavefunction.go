package epistemics

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

var (
	ErrUncollapsedClaim    = errors.New("cannot assert uncollapsed claim: P_corr < 0.75")
	ErrInsufficientSources = errors.New("cannot assert claim: requires at least 2 independent sources")
	ErrAccuracyExceeded    = errors.New("quantum-classical discrepancy exceeds chemical accuracy threshold (1.6 mHa)")
)

// ClaimWavefunction represents the tripartite credibility state
// |Ψ⟩ = α|corr⟩ + β|cont⟩ + γ|unver⟩, normalized so |α|² + |β|² + |γ|² = 1.0
type ClaimWavefunction struct {
	ClaimID                  string   `json:"claim_id"`
	Statement                string   `json:"statement"`
	AlphaCorroborated        float64  `json:"alpha_corroborated"`
	BetaContested            float64  `json:"beta_contested"`
	GammaUnverified          float64  `json:"gamma_unverified"`
	EvidenceStrength         float64  `json:"evidence_strength"`
	Sources                  []string `json:"sources"`
	ChemicalAccuracyDeltaMHa float64  `json:"delta_mha"`
	Collapsed                bool     `json:"collapsed"`
}

// ProbabilityCorroborated returns |α|²
func (cw *ClaimWavefunction) ProbabilityCorroborated() float64 {
	return cw.AlphaCorroborated * cw.AlphaCorroborated
}

// EpistemicGate enforces Ring 1 assertion gating
type EpistemicGate struct {
	mu           sync.RWMutex
	claims       map[string]*ClaimWavefunction
	totalChecks  int
	refusalCount int
}

func NewEpistemicGate() *EpistemicGate {
	return &EpistemicGate{
		claims: make(map[string]*ClaimWavefunction),
	}
}

// RegisterClaim stores or updates a scientific hypothesis claim wavefunction.
// Sources are validated against the evidence vocabulary: an unresolvable
// citation refuses registration outright, so invented evidence can never
// enter the gate. An empty source list is legal (unattested claim); every
// listed source must resolve.
//
// W1.10 domain checks: non-empty ID/statement, non-negative amplitudes
// (negative amplitudes squared into collapse used to pass), finite
// non-negative accuracy delta (a -100 mHa "discrepancy" used to collapse),
// and the stored claim is a COPY — the old code kept the caller's pointer,
// so post-registration mutation bypassed the gate.
func (g *EpistemicGate) RegisterClaim(cw *ClaimWavefunction) error {
	if cw == nil {
		return fmt.Errorf("nil claim: refusing")
	}
	if cw.ClaimID == "" || cw.Statement == "" {
		return fmt.Errorf("claim id and statement are required: refusing")
	}
	for _, a := range []float64{cw.AlphaCorroborated, cw.BetaContested, cw.GammaUnverified} {
		if math.IsNaN(a) || math.IsInf(a, 0) || a < 0 {
			return fmt.Errorf("claim amplitudes must be finite and non-negative: refusing")
		}
	}
	if math.IsNaN(cw.ChemicalAccuracyDeltaMHa) || math.IsInf(cw.ChemicalAccuracyDeltaMHa, 0) ||
		cw.ChemicalAccuracyDeltaMHa < 0 {
		return fmt.Errorf("accuracy delta must be a finite non-negative discrepancy: refusing")
	}
	for _, src := range cw.Sources {
		if !ValidSourceLabel(src) {
			return fmt.Errorf("unresolvable source label %q on claim %q: refused, not recorded", src, cw.ClaimID)
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	cp := *cw
	cp.Sources = append([]string(nil), cw.Sources...)

	// Normalize amplitudes
	norm := math.Sqrt(cp.AlphaCorroborated*cp.AlphaCorroborated +
		cp.BetaContested*cp.BetaContested +
		cp.GammaUnverified*cp.GammaUnverified)
	if norm > 1e-9 {
		cp.AlphaCorroborated /= norm
		cp.BetaContested /= norm
		cp.GammaUnverified /= norm
	} else {
		cp.GammaUnverified = 1.0
	}

	pCorr := cp.ProbabilityCorroborated()
	cp.Collapsed = (pCorr >= 0.75 && len(cp.Sources) >= 2 && cp.ChemicalAccuracyDeltaMHa <= 1.6)

	g.claims[cp.ClaimID] = &cp
	return nil
}

// CanAssert determines whether an agent is mathematically allowed to assert the claim as established truth
func (g *EpistemicGate) CanAssert(claimID string) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.totalChecks++

	cw, exists := g.claims[claimID]
	if !exists {
		g.refusalCount++
		return false, fmt.Sprintf("claim '%s' not registered in epistemic ledger", claimID)
	}

	if cw.ProbabilityCorroborated() < 0.75 {
		g.refusalCount++
		return false, fmt.Sprintf("uncollapsed claim: P_corr=%.3f < 0.75 threshold", cw.ProbabilityCorroborated())
	}

	if len(cw.Sources) < 2 {
		g.refusalCount++
		return false, fmt.Sprintf("insufficient source diversity: %d sources < 2 required", len(cw.Sources))
	}

	if cw.ChemicalAccuracyDeltaMHa > 1.6 {
		g.refusalCount++
		return false, fmt.Sprintf("chemical accuracy violated: delta=%.3f mHa > 1.6 mHa", cw.ChemicalAccuracyDeltaMHa)
	}

	return true, "claim collapsed and verified"
}

// RefusalRatio returns the fraction of assertions blocked by the gate
func (g *EpistemicGate) RefusalRatio() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.totalChecks == 0 {
		return 0.0
	}
	return float64(g.refusalCount) / float64(g.totalChecks)
}

// ComputeEpistemicCurvature computes concentration κ and literature drift d(E_canon, E_live).
// κ here is a cheap closed form over a caller-supplied variance — a
// DIAGNOSTIC, not a decision input: nothing in the codebase branches on it
// (the IPR/Fisher-Rao machinery in kubona/epistemics/curvature.py has no
// production callers). Do not gate collapses on this value without wiring
// measured variance first; the hardcoded call-site variance is a prior,
// not evidence.
//
// W1.10: non-finite variance refuses (NaN κ used to propagate silently into
// any future branch).
func ComputeEpistemicCurvature(variance float64, eCanon, eLive float64) (kappa float64, drift float64) {
	if math.IsNaN(variance) || math.IsInf(variance, 0) {
		return 0, 0
	}
	kappa = 1.0 / (1.0 + math.Abs(variance))
	drift = math.Abs(eCanon - eLive)
	return kappa, drift
}
