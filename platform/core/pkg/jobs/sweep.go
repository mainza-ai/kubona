package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

// PES sweep bounds. Steps cap the bridge bill (each point is a real PySCF
// computation, ~seconds); R bounds mirror the campaign synth's accepted
// geometry range. Both are v1 heuristics, enforced identically at submit
// (no residue) and in the worker (defense for direct SubmitJob callers).
const (
	minSweepSteps     = 2
	maxSweepSteps     = 12
	minSweepR         = 0.3
	maxSweepR         = 10.0
	sweepPointTimeout = 15 * time.Second
)

// ValidateSweepParams enforces the grid bounds pre-submit (no residue)
// and in the worker. Exported so every entry point shares one rule.
//
// PesSweepParams describes a classical potential-energy scan: one anchor
// computation per geometry across an inclusive linear grid.
type PesSweepParams struct {
	Molecule string  `json:"molecule"`
	RMin     float64 `json:"r_min"`
	RMax     float64 `json:"r_max"`
	Steps    int     `json:"steps"`
}

// PesSweepPoint is one computed geometry. Energy fields are absent (not
// zero) when that point's anchor computation failed — omitempty keeps
// "unknown" distinct from "measured zero".
type PesSweepPoint struct {
	R     float64  `json:"r"`
	RHF   *float64 `json:"rhf_energy,omitempty"`
	FCI   *float64 `json:"fci_energy,omitempty"`
	Error string   `json:"error,omitempty"`
}

// PesSweepResult is the stored job result: the grid plus every point.
type PesSweepResult struct {
	Success  bool            `json:"success"`
	Molecule string          `json:"molecule"`
	Points   []PesSweepPoint `json:"points"`
}

func ValidateSweepParams(data []byte) (*PesSweepParams, error) {
	var p PesSweepParams
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid pes_sweep params: %w", err)
	}
	if p.Molecule == "" {
		return nil, fmt.Errorf("pes_sweep requires molecule")
	}
	if p.Steps < minSweepSteps || p.Steps > maxSweepSteps {
		return nil, fmt.Errorf("pes_sweep steps must be %d..%d, got %d", minSweepSteps, maxSweepSteps, p.Steps)
	}
	if !(p.RMin >= minSweepR && p.RMax <= maxSweepR && p.RMin < p.RMax) {
		return nil, fmt.Errorf("pes_sweep needs %.1f <= r_min < r_max <= %.1f", minSweepR, maxSweepR)
	}
	return &p, nil
}

// executePesSweep runs the grid against the bridge. Points are independent:
// one failed geometry is recorded with its reason, never aborting the rest.
// All-failed is a job failure (nothing measured); anything else completes
// with per-point errors inline so partial curves stay transparent.
func (r *Runner) executePesSweep(p *PesSweepParams) ([]byte, error) {
	if r.bridgeClient == nil {
		return nil, fmt.Errorf("bridge client unavailable: no pes_sweep computation performed")
	}
	res := PesSweepResult{Success: true, Molecule: p.Molecule}
	for i := 0; i < p.Steps; i++ {
		rv := p.RMin
		if p.Steps > 1 {
			rv = p.RMin + (p.RMax-p.RMin)*float64(i)/float64(p.Steps-1)
		}
		pt := PesSweepPoint{R: rv}
		anchorCtx, cancel := context.WithTimeout(r.ctx, sweepPointTimeout)
		resp, err := r.bridgeClient.ComputeClassicalAnchor(anchorCtx, &pb.ClassicalAnchorRequest{
			Molecule:     p.Molecule,
			BondDistance: rv,
		})
		cancel()
		switch {
		case err != nil:
			pt.Error = err.Error()
		case resp == nil || !resp.Success:
			msg := "anchor reported success=false"
			if resp != nil && resp.ErrorMessage != "" {
				msg = resp.ErrorMessage
			}
			pt.Error = msg
		default:
			rhf, fci := resp.RhfEnergy, resp.FciExactEnergy
			pt.RHF, pt.FCI = &rhf, &fci
		}
		res.Points = append(res.Points, pt)
	}
	succeeded := 0
	for _, pt := range res.Points {
		if pt.Error == "" {
			succeeded++
		}
	}
	if succeeded == 0 {
		return nil, fmt.Errorf("pes_sweep: all %d geometries failed (first: %s)", len(res.Points), res.Points[0].Error)
	}
	return json.Marshal(res)
}
