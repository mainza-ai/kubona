// Codesign evaluator proxy & durable codesign_runs receipts (Phase 0 / G1).
//
// Threat model (Decision A3):
// - Stateless evaluation (no session_id): public behind limiters, bounded compute,
//   zero spend, zero persistence.
// - Receipted evaluation (session_id present): authorizes the known session,
//   executes Python kickmix as the single validator, persists a durable
//   `codesign_runs` row with `run_id` + `artifact_sha256`, and returns `run_id`.
// - Reads (`GET /api/codesign/runs`, `GET /api/codesign/runs/{id}`): require
//   `session_id` and authorize the known session (strict session-scoping).

package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

const codesignEvalMaxBody = 1 << 20

func newCodesignRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "cs_" + hex.EncodeToString(b[:])
}

func (s *Server) runCodesignOp(r *http.Request, operation string, raw []byte) ([]byte, error) {
	if out, ok := s.codesignViaBridge(r.Context(), operation, raw); ok {
		return out, nil
	}
	shim := "import json, sys\nfrom codesign_ops import run_op\nprint(json.dumps(run_op(sys.argv[1], json.loads(sys.argv[2]))))"
	out, _, err := runBridgePython(r.Context(), 120*time.Second, shim, operation, string(raw))
	return out, err
}

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
	var reqMap map[string]any
	if json.Unmarshal(raw, &reqMap) != nil {
		WriteAPIError(w, "bad_request", "invalid JSON body: refusing", nil)
		return
	}

	// Decision A3: optional session_id enables durable codesign_runs receipting.
	sessionID := ""
	if operation == "evaluate" && reqMap != nil {
		if sid, ok := reqMap["session_id"].(string); ok && strings.TrimSpace(sid) != "" {
			sessionID = strings.TrimSpace(sid)
			sess, authOK := s.authorizeKnownSession(w, r, sessionID)
			if !authOK {
				return
			}
			if sess == nil {
				WriteAPIError(w, "not_found", "unknown session", nil)
				return
			}
		}
	}

	out, err := s.runCodesignOp(r, operation, raw)
	if err != nil {
		http.Error(w, fmt.Sprintf("codesign evaluator unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}
	s.writeCodesignResultWithReceipt(w, operation, sessionID, reqMap, out)
}

func (s *Server) writeCodesignResultWithReceipt(
	w http.ResponseWriter,
	operation string,
	sessionID string,
	reqMap map[string]any,
	out []byte,
) {
	cleanOut := bytes.TrimSpace(out)
	if idx := bytes.Index(cleanOut, []byte("{")); idx >= 0 {
		cleanOut = cleanOut[idx:]
	}
	var parsed map[string]any
	if json.Unmarshal(cleanOut, &parsed) != nil || parsed == nil {
		http.Error(w, "codesign evaluator returned unparseable output: refusing", http.StatusBadGateway)
		return
	}
	okFlag, _ := parsed["ok"].(bool)
	if !okFlag {
		msg := asStringField(parsed, "error")
		if msg == "" {
			msg = "evaluation refused"
		}
		WriteAPIError(w, "bad_request", msg, nil)
		return
	}

	if operation == "evaluate" && sessionID != "" && s.ledger != nil {
		resObj, _ := parsed["result"].(map[string]any)
		artBytes, _ := json.Marshal(reqMap["artifact"])
		resBytes, _ := json.Marshal(resObj)
		runID := newCodesignRunID()
		validFlag, _ := asBoolField(resObj, "valid")
		toffoli, _ := asFloatField(resObj, "toffoli")
		peakQ, _ := asFloatField(resObj, "peak_qubits")
		tqVal, _ := asFloatField(resObj, "tq")
		epsVal, okEps := asFloatField(resObj, "eps_bound_97_5")
		if !okEps {
			epsVal = 1.0
		}
		depth, _ := asFloatField(resObj, "depth")
		tDepth, _ := asFloatField(resObj, "toffoli_depth")
		kappa, _ := asFloatField(parsed, "kappa")
		seed, _ := asFloatField(parsed, "seed")
		numTests, _ := asFloatField(parsed, "num_tests")
		maxFail, _ := asFloatField(parsed, "max_failures")

		rec := ledger.CodesignRunRecord{
			RunID:              runID,
			SessionID:          sessionID,
			Operation:          operation,
			SpecKind:           asStringField(parsed, "spec_kind"),
			SpecTableSHA256:    asStringField(parsed, "spec_table_sha256"),
			ArtifactSHA256:     asStringField(parsed, "artifact_sha256"),
			ArtifactJSON:       string(artBytes),
			Kappa:              int(kappa),
			Seed:               int(seed),
			NumTests:           int(numTests),
			MaxFailures:        int(maxFail),
			Valid:              validFlag,
			Toffoli:            int(toffoli),
			PeakQubits:         int(peakQ),
			TQ:                 int(tqVal),
			EpsBound:           epsVal,
			VerificationMethod: asStringField(resObj, "verification_method"),
			Depth:              int(depth),
			ToffoliDepth:       int(tDepth),
			ResultJSON:         string(resBytes),
		}
		if err := s.ledger.SaveCodesignRun(rec); err == nil {
			parsed["run_id"] = runID
			parsed["session_id"] = sessionID
		}
	}

	writeJSON(w, http.StatusOK, parsed)
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

// handleCodesignPackPromote promotes a scaffold pack to active status:
//
//	POST /api/codesign/packs/{name}/promote
//
// Requires non-empty anchor_ref and proof_ref; fails closed (400) if either is absent.
func (s *Server) handleCodesignPackPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required", nil)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/codesign/packs/")
	if !strings.HasSuffix(rest, "/promote") {
		WriteAPIError(w, "not_found", "expected /api/codesign/packs/{name}/promote", nil)
		return
	}
	packName := strings.TrimSpace(strings.TrimSuffix(rest, "/promote"))
	if packName == "" || strings.Contains(packName, "/") {
		WriteAPIError(w, "bad_request", "invalid pack name", nil)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, codesignEvalMaxBody))
	if err != nil {
		WriteAPIError(w, "bad_request", "body too large or unreadable: refusing", nil)
		return
	}
	var reqMap map[string]any
	if json.Unmarshal(raw, &reqMap) != nil || reqMap == nil {
		WriteAPIError(w, "bad_request", "invalid JSON body: refusing", nil)
		return
	}
	anchorRef := strings.TrimSpace(asStringField(reqMap, "anchor_ref"))
	proofRef := strings.TrimSpace(asStringField(reqMap, "proof_ref"))
	if anchorRef == "" || proofRef == "" {
		WriteAPIError(w, "bad_request", "promotion requires non-empty anchor_ref and proof_ref: refusing", nil)
		return
	}
	reqMap["name"] = packName
	reqMap["anchor_ref"] = anchorRef
	reqMap["proof_ref"] = proofRef
	fwd, err := json.Marshal(reqMap)
	if err != nil {
		WriteAPIError(w, "internal", "failed to encode promote request", nil)
		return
	}
	out, err := s.runCodesignOp(r, "pack_promote", fwd)
	if err != nil {
		http.Error(w, fmt.Sprintf("codesign evaluator unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}
	s.writeCodesignResultWithReceipt(w, "pack_promote", "", reqMap, out)
}

func (s *Server) handleCodesignQecJoint(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, "qec_joint")
}

// handleCodesignRuns serves durable session-scoped codesign receipts:
//
//	GET /api/codesign/runs?session_id=...
//	GET /api/codesign/runs/{id}?session_id=...
func (s *Server) handleCodesignRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required", nil)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id query parameter is required", nil)
		return
	}
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}

	runID := strings.TrimPrefix(r.URL.Path, "/api/codesign/runs")
	runID = strings.TrimPrefix(runID, "/")
	if runID == "" {
		runs, err := s.ledger.ListCodesignRuns(sessionID, 50)
		if err != nil {
			WriteAPIError(w, "internal", "failed to list codesign runs", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": sessionID,
			"runs":       runs,
		})
		return
	}

	run, err := s.ledger.GetCodesignRun(sessionID, runID)
	if err != nil {
		if errors.Is(err, ledger.ErrCodesignRunNotFound) {
			WriteAPIError(w, "not_found", err.Error(), nil)
			return
		}
		WriteAPIError(w, "internal", "failed to load codesign run", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
