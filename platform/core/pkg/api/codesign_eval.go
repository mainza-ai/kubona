// Codesign evaluator proxy (Phase 1 migration, ux-plan slice 5).
//
// The Python console_api.py module is unserved (no listener, no compose
// target); these handlers migrate its STATELESS compute routes behind the
// Go rail via runBridgePython, keeping Python as the single validator and
// the single implementation (no second copy of kickmix/TQ/frontier/budget
// math anywhere). Stateful duplicates (sessions/chat/loops/...) stay
// retired — this file migrates compute only.
//
// Threat model: same as the existing /api/codesign/* reads — read-only
// scoring, no spend, no jobs, no secrets. Public behind limiters; bounded
// compute (artifact caps + num_tests HTTP bound + ctx timeout); validity
// stays Python-owned (Go maps transport + status, never verdicts).
//
// Paths: /api/codesign/evaluate (single artifact + optional factory
// diagnostics), /api/codesign/evaluate-batch (artifacts[] with frontier),
// /api/codesign/budget (union-bound math). The existing
// /api/codesign/frontier (pre-scored entries) is untouched — different
// contract (no collision): entries there, artifacts here.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const codesignEvalMaxBody = 1 << 20

// proxyCodesign is the shared rail: method gate, body cap, forward raw,
// map outer ok flag to status. Python owns all validation (single
// validator, now in bridge/codesign_ops.py); Go owns transport +
// timeout + status mapping. Bridge rail first (prod); the dev fallback
// runs the same ops module through a 6-line shim — the five retired
// string constants are gone, not forked.
func (s *Server) proxyCodesign(w http.ResponseWriter, r *http.Request, operation string, allowGet ...bool) {
	getOK := len(allowGet) > 0 && allowGet[0]
	if r.Method != http.MethodPost && !(getOK && r.Method == http.MethodGet) {
		http.Error(w, "method not allowed: refusing", http.StatusMethodNotAllowed)
		return
	}
	raw := []byte("{}")
	if r.Method == http.MethodPost {
		var err error
		raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, codesignEvalMaxBody))
		if err != nil {
			WriteAPIError(w, "bad_request", "body too large or unreadable: refusing", nil)
			return
		}
	}
	var probe any
	if json.Unmarshal(raw, &probe) != nil {
		WriteAPIError(w, "bad_request", "invalid JSON body: refusing", nil)
		return
	}
	if out, ok := s.codesignViaBridge(r.Context(), operation, raw); ok {
		s.writeCodesignResult(w, out)
		return
	}
	shim := "import json, sys\nfrom codesign_ops import run_op\nprint(json.dumps(run_op(sys.argv[1], json.loads(sys.argv[2]))))"
	out, _, err := runBridgePython(r.Context(), 120*time.Second, shim, operation, string(raw))
	if err != nil {
		http.Error(w, fmt.Sprintf("codesign evaluator unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}
	s.writeCodesignResult(w, out)
}

// writeCodesignResult maps the outer ok flag to status (unchanged
// contract): unparseable is a 502, ok:false is a 400 naming the refusal,
// ok:true serves the full document.
func (s *Server) writeCodesignResult(w http.ResponseWriter, out []byte) {
	cleanOut := bytes.TrimSpace(out)
	if idx := bytes.Index(cleanOut, []byte("{")); idx >= 0 {
		cleanOut = cleanOut[idx:]
	}
	var parsed struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if json.Unmarshal(cleanOut, &parsed) != nil {
		http.Error(w, "codesign evaluator returned unparseable output: refusing", http.StatusBadGateway)
		return
	}
	if !parsed.OK {
		msg := parsed.Error
		if msg == "" {
			msg = "evaluation refused"
		}
		WriteAPIError(w, "bad_request", msg, nil)
		return
	}
	var full any
	_ = json.Unmarshal(cleanOut, &full)
	writeJSON(w, http.StatusOK, full)
}

func (s *Server) handleCodesignEvaluate(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, "evaluate")
}

func (s *Server) handleCodesignEvaluateBatch(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, "batch")
}

func (s *Server) handleCodesignFamilies(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, "families")
}

func (s *Server) handleCodesignBudget(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, "budget")
}

func (s *Server) handleCodesignPacks(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, "packs", true)
}
