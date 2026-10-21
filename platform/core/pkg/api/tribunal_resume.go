package api

// T-16: tribunal resume route (R4 turn-bundle flow). ExecuteTribunalResume
// existed with no HTTP route — the flow was real but unreachable.
// finishTribunalRun is the tail handleChatTribunal and the new resume
// route share (obligation ownership, close, tool audit, transcript,
// campaign refresh, claim mint, write-back, idempotent receipt) so the
// two entries cannot drift apart.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/agents"
)

func (s *Server) tribunalTurnRecorder(sessionID string) func(agents.TribunalTurn) {
	return func(turn agents.TribunalTurn) {
		s.sessionMgr.AddMessage(sessionID, string(turn.Role), fmt.Sprintf("[%s]: %s", turn.Name, turn.Content), nil)
		// C3: no handler-side sealing — the collective seals each turn
		// (event + obligation case, deduped by turn index) during the
		// run. Sealing here as well would double-seal every turn.
	}
}

func (s *Server) finishTribunalRun(w http.ResponseWriter, r *http.Request, tribIdem *idemTicket, sessionID, prompt string, eventsBefore int64, res *agents.TribunalResult, runErr error) {
	if runErr != nil {
		s.idemFinishError(w, tribIdem, http.StatusInternalServerError, runErr.Error())
		return
	}
	// R2: the verdict lives in the obligation query, not in prose. The
	// session owns the run obligation (exactly one owner); close reads
	// cases, authorities, unknowns, dependencies. Unwired runs (no
	// obligation) keep the collective's provisional verdict.
	if res.ObligationID != "" {
		if serr := s.ledger.SetObligationOwner(res.ObligationID, "session:"+sessionID); serr != nil {
			s.idemFinishError(w, tribIdem, http.StatusInternalServerError, serr.Error())
			return
		}
		closed, reasons, cerr := agents.AttemptClose(s.ledger, res.ObligationID)
		if cerr != nil {
			s.idemFinishError(w, tribIdem, http.StatusInternalServerError, cerr.Error())
			return
		}
		res.ConsensusPassed = closed
		if !closed {
			res.Synthesis = fmt.Sprintf("Multi-Agent Tribunal did NOT reach consensus for %s: obligation %s stays open (%s).",
				res.Target, res.ObligationID, strings.Join(reasons, "; "))
		}
	}
	// Tribunal tool use audits exactly like copilot tool use: one
	// tool_called event per observed dispatch, so the memory write-back
	// below can attribute this turn's tools instead of passing nil.
	s.auditTribunalTools(sessionID, res.Turns)
	// Add final assistant consensus with proposal
	assistantMsg, _ := s.sessionMgr.AddMessage(sessionID, "assistant", res.Synthesis, res.ProposalJSON)

	// Dynamically update campaign state for session
	campState, _ := agents.SynthesizeCampaignState(r.Context(), sessionID, prompt, s.llmClient, s.jobRunner.BridgeClient())
	if campState != nil {
		campBytes, _ := json.Marshal(campState)
		s.ledger.SaveCampaignStateWithTrigger(sessionID, campBytes, triggerLabel("tribunal", prompt))
	}
	tribOutcome := "consensus-failed"
	tribVerdict := "contested"
	if res.ConsensusPassed {
		tribOutcome = "consensus-passed"
		tribVerdict = "verified"
	}
	if res.ProposalJSON != nil {
		tribOutcome += ",proposal"
	}
	// The consensus is an evaluated claim: mint its record so the verdict
	// is citable. Amplitudes stay -1 (not evaluated) — the verdict alone
	// is what the tribunal measured.
	tribRecordID := ""
	evBytes, _ := json.Marshal(map[string]any{
		"turns":         len(res.Turns),
		"consensus":     res.ConsensusPassed,
		"has_proposal":  res.ProposalJSON != nil,
		"obligation_id": res.ObligationID,
	})
	if rec, merr := s.ledger.MintClaimRecord(sessionID, "tribunal",
		"Tribunal consensus: "+boundRunes(res.Synthesis, 400),
		-1, -1, -1, tribVerdict, string(evBytes)); merr == nil {
		tribRecordID = rec.ID
		tribOutcome += " [" + rec.ID + "]"
		// C4: the real proposal_generated producer — emitted where the
		// proposal is produced, beside the claim mint, in the claim's
		// evidence shape. Every claim_record_id-bearing response has a
		// matching proposal event (asserted, not assumed).
		_ = s.ledger.AppendEvent(sessionID, "proposal_generated", map[string]any{
			"claim_record_id": rec.ID,
			"obligation_id":   res.ObligationID,
			"has_proposal":    res.ProposalJSON != nil,
			"consensus":       res.ConsensusPassed,
		})
	}
	// R6 pilot: score the recorded transcript (best-effort — scoring
	// follows evidence, never fails the run). Authorship comes from the
	// obligation cases (pass == authored); unwired runs score content
	// only (authored unknown reads false — conservative, stated).
	transcriptScores := s.scoreTribunalTranscript(sessionID, res)
	s.retainTurnExperience(sessionID, "tribunal", prompt, tribOutcome,
		s.toolNamesSince(sessionID, eventsBefore), 0.7)

	s.idemFinishJSON(w, tribIdem, http.StatusOK, map[string]any{
		"tribunal":          res,
		"assistant_message": assistantMsg,
		"campaign":          campState,
		"claim_record_id":   tribRecordID,
		"obligation_id":     res.ObligationID,
		"transcript_scores": transcriptScores,
	})
}

// handleTribunalResume continues a tribunal from an exported turn bundle.
// Same gates as a fresh tribunal (disk, LLM budget, ghost-session 404,
// autonomy 403, slow-rail idempotency, bundle parses); the bundle seeds
// ancestor evidence and the resumed run opens a FRESH obligation (closed
// history is never rewritten — linkage lives in evidence parents).
func (s *Server) handleTribunalResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	if err := s.checkLLMBudget(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	var req struct {
		SessionID string          `json:"session_id"`
		Prompt    string          `json:"prompt"`
		Bundle    json.RawMessage `json:"bundle"`
		RequestID string          `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}
	if req.Prompt == "" {
		req.Prompt = "Investigate H2 molecular ground state energy at equilibrium bond distance 0.7414 A"
	}
	if len(req.Bundle) == 0 {
		WriteAPIError(w, "bad_request", "resume requires {\"bundle\": {...}}: export a turn bundle from a completed run first", nil)
		return
	}
	if _, perr := agents.ParseTribunalBundle(req.Bundle); perr != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("bundle refused: %v", perr), nil)
		return
	}
	if req.SessionID != "" {
		if _, gerr := s.sessionMgr.GetSession(req.SessionID); gerr != nil {
			WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: tribunal refuses to attach elsewhere", req.SessionID), nil)
			return
		}
	} else {
		http.Error(w, "missing session_id: tribunal calls must name an existing session", http.StatusNotFound)
		return
	}
	if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	tribIdem, tribRef := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
		"session_id": req.SessionID, "prompt": req.Prompt, "bundle": string(req.Bundle),
	}, idemRailSlow)
	if tribRef != nil {
		tribRef.write(w)
		return
	}
	if tribIdem.replay {
		s.idemReplay(w, tribIdem)
		return
	}
	tribEventsBefore := s.latestEventID(req.SessionID)
	res, err := s.collective.ExecuteTribunalResume(r.Context(), s.llmClient, s.jobRunner.BridgeClient(), s.memoryStore, req.SessionID, req.Prompt, req.Bundle, s.tribunalTurnRecorder(req.SessionID))
	s.finishTribunalRun(w, r, tribIdem, req.SessionID, req.Prompt, tribEventsBefore, res, err)
}
