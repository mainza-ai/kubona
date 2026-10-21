package api

// T-12: the stagnation minter existed but discarded the return, so the
// proposal id never reached the operator. Extracted as a named method so
// the wiring (mint → ledger event carrying proposal_id → listable row)
// is pinnable without running a loop to 5 discards live. No new engine,
// no file-writing apply (ApplyProposal deliberately writes no files
// until Phase-5).

import (
	"context"
	"fmt"
)

func (s *Server) onLoopStagnation(sessionID, harnessID, loopID string, discards int, champion string) {
	prop, perr := s.metaSearchEngine.GenerateStagnationProposal(
		context.Background(),
		loopID,
		harnessID,
		fmt.Sprintf("Standard instructions for %s", harnessID),
		champion,
		discards,
	)
	fields := map[string]any{
		"loop_id": loopID, "discards": discards, "harness_id": harnessID,
	}
	if perr == nil && prop != nil {
		fields["proposal_id"] = prop.ID
		fields["proposal_status"] = string(prop.Status)
	} else if perr != nil {
		fields["proposal_error"] = perr.Error()
	}
	_ = s.ledger.AppendEvent(sessionID, "autoresearch_stagnation_triggered", fields)
}
