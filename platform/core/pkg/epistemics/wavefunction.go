package epistemics

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

var (
	ErrUncollapsedClaim   = errors.New("cannot assert uncollapsed claim: P_corr < 0.75")
	ErrInsufficientSources = errors.New("cannot assert claim: requires at least 2 independent sources")
	ErrAccuracyExceeded   = errors.New("quantum-classical discrepancy exceeds chemical accuracy threshold (1.6 mHa)")
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

// RegisterClaim stores or updates a scientific hypothesis claim wavefunction
func (g *EpistemicGate) RegisterClaim(cw *ClaimWavefunction) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Normalize amplitudes
	norm := math.Sqrt(cw.AlphaCorroborated*cw.AlphaCorroborated +
		cw.BetaContested*cw.BetaContested +
		cw.GammaUnverified*cw.GammaUnverified)
	if norm > 1e-9 {
		cw.AlphaCorroborated /= norm
		cw.BetaContested /= norm
		cw.GammaUnverified /= norm
	} else {
		cw.GammaUnverified = 1.0
	}

	pCorr := cw.ProbabilityCorroborated()
	cw.Collapsed = (pCorr >= 0.75 && len(cw.Sources) >= 2 && cw.ChemicalAccuracyDeltaMHa <= 1.6)

	g.claims[cw.ClaimID] = cw
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

// ComputeEpistemicCurvature computes concentration κ and literature drift d(E_canon, E_live)
func ComputeEpistemicCurvature(variance float64, eCanon, eLive float64) (kappa float64, drift float64) {
	kappa = 1.0 / (1.0 + math.Abs(variance))
	drift = math.Abs(eCanon - eLive)
	return kappa, drift
}
