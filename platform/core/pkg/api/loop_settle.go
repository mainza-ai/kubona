package api

// T-10b: settle-and-archive for restart-orphan loops (D-10).
//
// Restarts flip live rows to sleeping but rehydrate no engine, so
// sleeping rows with no driver pin their session forever (sleeping is
// deliberately non-terminal, the action handler 404s without an engine,
// the sweeper skips sleeping, delete refuses "live loops").
// Settle gives them a terminal disposition: release both reservation
// rails idempotently, flip to aborted (terminal in both vocabularies),
// and seal a ledger event per loop. Silent UPDATEs would break the
// hash-chain audit contract — every other loop transition appends, so
// settle appends too.
//
// Fail-closed: only sleeping rows settle; any non-sleeping (possibly
// live) loop in the session refuses the whole call with 409. Ghost
// sessions 404. Caller-vs-session binding via authorizeKnownSession.

import (
	"log"
	"net/http"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func (s *Server) handleLoopSettle(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	rows, err := s.ledger.ListLoopStates()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var orphans []ledger.LoopPersist
	for _, p := range rows {
		if p.SessionID != sessionID {
			continue
		}
		if p.Status != "sleeping" {
			http.Error(w, "live loop "+p.ID+" is not a restart orphan: stop it first, settle covers sleeping rows only", http.StatusConflict)
			return
		}
		orphans = append(orphans, p)
	}
	settled := []string{}
	for _, p := range orphans {
		// Release both rails first (idempotent conditional zero — a kill
		// racing settle releases exactly once), then flip.
		s.releaseLoopSpend(p.ID, "settle-orphan")
		st, gerr := s.ledger.GetLoopState(p.ID)
		if gerr != nil {
			http.Error(w, gerr.Error(), http.StatusInternalServerError)
			return
		}
		if st == nil {
			continue
		}
		st.Status = "aborted"
		st.Drain = false
		if serr := s.ledger.SaveLoopState(*st); serr != nil {
			http.Error(w, serr.Error(), http.StatusInternalServerError)
			return
		}
		// The receipt is load-bearing (S6 closed taxonomy caught an
		// unregistered type during development — never discard). If the
		// seal fails after the flip, say so honestly: retry is safe
		// (aborted rows are no longer orphans, so a retry settles
		// nothing twice).
		if aerr := s.ledger.AppendEvent(sessionID, "loop_settled", map[string]any{
			"loop_id": p.ID, "previous_status": p.Status,
			"cause": "restart orphan: in-memory engine gone, archived terminal",
		}); aerr != nil {
			log.Printf("[Kubona Core] loop_settled receipt failed for %s: %v", p.ID, aerr)
			http.Error(w, "loop "+p.ID+" settled terminal but the receipt failed: "+aerr.Error(), http.StatusInternalServerError)
			return
		}
		settled = append(settled, p.ID)
	}
	if settled == nil {
		settled = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"settled": settled, "session_id": sessionID})
}
