package api

// T-01a: chemistry over the bridge gRPC rail (prod compose has no python
// in the kubonad image, so the local exec rail 503s there by
// construction). Bridge-first with local fallback:
//   - transport failure (no client, dial error, timeout, unparseable
//     envelope) → (nil, false): the caller falls back to runBridgePython
//     (dev rail) or its 503 (prod — same message as before).
//   - domain refusal (invalid SMILES, no valid lead) → ({"error": ...},
//     true): matches the local rail shape, which prints {"error": ...}
//     and the handlers serve verbatim. No second attempt — the bridge
//     already ran the same function the fallback would run.
// Timeouts match the local rail budgets (ADMET 120s, pipeline 180s).

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

func (s *Server) admetViaBridge(ctx context.Context, smiles string) (map[string]any, bool) {
	bridge := s.jobRunner.BridgeClient()
	if bridge == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := bridge.EvaluateMolecularADMET(ctx, &pb.ChemistryADMETRequest{Smiles: smiles})
	if err != nil || resp == nil {
		return nil, false
	}
	if !resp.Success {
		return map[string]any{"error": resp.ErrorMessage}, true
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(resp.ResultJson), &parsed) != nil {
		return nil, false
	}
	return parsed, true
}

func (s *Server) discoveryViaBridge(ctx context.Context, target string, candidatesJSON []byte) (map[string]any, bool) {
	bridge := s.jobRunner.BridgeClient()
	if bridge == nil {
		return nil, false
	}
	var cands []*pb.DiscoveryCandidate
	if json.Unmarshal(candidatesJSON, &cands) != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	resp, err := bridge.EvaluateDiscoveryPipeline(ctx, &pb.DiscoveryPipelineRequest{Target: target, Candidates: cands})
	if err != nil || resp == nil {
		return nil, false
	}
	if !resp.Success {
		return map[string]any{"error": resp.ErrorMessage}, true
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(resp.ResultJson), &parsed) != nil {
		return nil, false
	}
	return parsed, true
}
