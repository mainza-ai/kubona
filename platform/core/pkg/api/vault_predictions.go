package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// Vault pre-registration (P3): two-stage scientific pre-registration lifecycle.
// Stage 1 seals an immutable expectation BEFORE compute; Stage 2 evaluates a
// measurement against it. All math is documented below — no invented scores.

// stdFloorHa is the hard sigma floor from the plan: sigma >= 1e-4 prevents
// infinite-division blowups on overconfident priors.
const stdFloorHa = 1e-4

// sealedContainmentF is P(|E-mu| <= 3σ) under the sealed Gaussian:
// erf(3/sqrt(2)) ≈ 0.9973002. The Brier score below is the proper score of
// the sealed "measurement lands inside 3σ" claim — real Brier semantics
// (probabilistic forecast f, binary outcome o), not a relabeled error.
var sealedContainmentF = math.Erf(3 / math.Sqrt2)

// SealPredictionRequest seals one pre-registered expectation.
type SealPredictionRequest struct {
	SessionID    string  `json:"session_id"`
	TargetSystem string  `json:"target_system"`
	Method       string  `json:"method"`
	Quantity     string  `json:"quantity"`
	ExpectedMean float64 `json:"expected_mean"`
	ExpectedStd  float64 `json:"expected_std"`
	BudgetCapUSD float64 `json:"budget_cap_usd"`
	Author       string  `json:"author"`
	PCorrelated  float64 `json:"p_correlated"`
}

// SealPredictionResponse echoes the sealed commitment.
type SealPredictionResponse struct {
	OK              bool    `json:"ok"`
	PredictionHash  string  `json:"prediction_hash"`
	ExpectedStdUsed float64 `json:"expected_std_used"`
	StdFloorApplied bool    `json:"std_floor_applied"`
}

// EvaluatePredictionRequest scores one measurement against a sealed prior.
type EvaluatePredictionRequest struct {
	SessionID      string  `json:"session_id"`
	PredictionHash string  `json:"prediction_hash"`
	MeasuredValue  float64 `json:"measured_value"`
}

// EvaluatePredictionResponse carries the standardized surprise.
type EvaluatePredictionResponse struct {
	OK            bool    `json:"ok"`
	SurpriseZ     float64 `json:"surprise_z"`
	SurpriseRatio float64 `json:"surprise_ratio"`
	BrierScore    float64 `json:"brier_score"`
	Within3Sigma  bool    `json:"within_3sigma"`
	Level         string  `json:"level"` // normal | notable | anomalous
}

// SurpriseLevel maps |z| to the plan's three bands:
// |z| < 1 normal, 1 <= |z| <= 3 notable, |z| > 3 anomalous.
func SurpriseLevel(z float64) string {
	a := math.Abs(z)
	switch {
	case a > 3:
		return "anomalous"
	case a >= 1:
		return "notable"
	default:
		return "normal"
	}
}

// sealedPrediction is the decoded ledger record of a Stage-1 seal.
type sealedPrediction struct {
	PredictionHash string  `json:"prediction_hash"`
	TargetSystem   string  `json:"target_system"`
	Method         string  `json:"method"`
	Quantity       string  `json:"quantity"`
	ExpectedMean   float64 `json:"expected_mean"`
	ExpectedStd    float64 `json:"expected_std"`
	BudgetCapUSD   float64 `json:"budget_cap_usd"`
	Author         string  `json:"author"`
	PCorrelated    float64 `json:"p_correlated"`
}

// findSealedPrediction scans newest-first for the seal with this hash.
func (s *Server) findSealedPrediction(sessionID, hash string) (*sealedPrediction, error) {
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.EventType != "prediction_sealed" {
			continue
		}
		var p sealedPrediction
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		if p.PredictionHash == hash {
			return &p, nil
		}
	}
	return nil, nil
}

func (s *Server) handlePredictionSeal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to seal a prediction", nil)
		return
	}
	var req SealPredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}
	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	if req.TargetSystem == "" || req.Method == "" || req.Quantity == "" || req.Author == "" {
		WriteAPIError(w, "bad_request", "target_system, method, quantity, and author are all required", nil)
		return
	}
	if math.IsNaN(req.ExpectedMean) || math.IsInf(req.ExpectedMean, 0) {
		WriteAPIError(w, "bad_request", "expected_mean must be finite", nil)
		return
	}
	if math.IsNaN(req.ExpectedStd) || math.IsInf(req.ExpectedStd, 0) || req.ExpectedStd <= 0 {
		WriteAPIError(w, "bad_request", "expected_std must be positive (floor 1e-4 applies to tiny positive values)", nil)
		return
	}
	if req.PCorrelated < 0 || req.PCorrelated > 1 || math.IsNaN(req.PCorrelated) {
		WriteAPIError(w, "bad_request", "p_correlated must lie in [0, 1]", nil)
		return
	}
	if req.BudgetCapUSD < 0 || math.IsNaN(req.BudgetCapUSD) {
		WriteAPIError(w, "bad_request", "budget_cap_usd must be non-negative", nil)
		return
	}

	stdUsed := req.ExpectedStd
	floorApplied := false
	if stdUsed < stdFloorHa {
		stdUsed = stdFloorHa
		floorApplied = true
	}

	hash, err := ledger.CanonicalHash(map[string]any{
		"author":        req.Author,
		"budget_cap":    req.BudgetCapUSD,
		"expected_mean": req.ExpectedMean,
		"expected_std":  stdUsed,
		"method":        req.Method,
		"quantity":      req.Quantity,
		"system":        req.TargetSystem,
	})
	if err != nil {
		WriteAPIError(w, "internal", "failed to hash prediction", nil)
		return
	}

	if err := s.ledger.AppendEvent(req.SessionID, "prediction_sealed", map[string]any{
		"session_id":      req.SessionID,
		"prediction_hash": hash,
		"target_system":   req.TargetSystem,
		"p_correlated":    req.PCorrelated,
		"method":          req.Method,
		"quantity":        req.Quantity,
		"expected_mean":   req.ExpectedMean,
		"expected_std":    stdUsed,
		"budget_cap_usd":  req.BudgetCapUSD,
		"author":          req.Author,
	}); err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to seal prediction: %v", err), nil)
		return
	}

	writeJSON(w, http.StatusOK, SealPredictionResponse{
		OK:              true,
		PredictionHash:  hash,
		ExpectedStdUsed: stdUsed,
		StdFloorApplied: floorApplied,
	})
}

func (s *Server) handlePredictionEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to evaluate a prediction", nil)
		return
	}
	var req EvaluatePredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}
	if req.SessionID == "" || req.PredictionHash == "" {
		WriteAPIError(w, "bad_request", "session_id and prediction_hash are required", nil)
		return
	}
	if math.IsNaN(req.MeasuredValue) || math.IsInf(req.MeasuredValue, 0) {
		WriteAPIError(w, "bad_request", "measured_value must be finite", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}

	sealed, err := s.findSealedPrediction(req.SessionID, req.PredictionHash)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read sealed predictions: %v", err), nil)
		return
	}
	if sealed == nil {
		WriteAPIError(w, "not_found", fmt.Sprintf("no sealed prediction %q in this session", req.PredictionHash), nil)
		return
	}

	z := (req.MeasuredValue - sealed.ExpectedMean) / sealed.ExpectedStd
	ratio := math.Abs(z)
	within := ratio <= 3
	outcome := 0.0
	if within {
		outcome = 1.0
	}
	brier := (sealedContainmentF - outcome) * (sealedContainmentF - outcome)

	if err := s.ledger.AppendEvent(req.SessionID, "prediction_evaluated", map[string]any{
		"session_id":      req.SessionID,
		"prediction_hash": req.PredictionHash,
		"brier_score":     brier,
		"surprise_ratio":  ratio,
		"measured_value":  req.MeasuredValue,
		"surprise_z":      z,
		"within_3sigma":   within,
		"level":           SurpriseLevel(z),
	}); err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to record evaluation: %v", err), nil)
		return
	}

	writeJSON(w, http.StatusOK, EvaluatePredictionResponse{
		OK:            true,
		SurpriseZ:     z,
		SurpriseRatio: ratio,
		BrierScore:    brier,
		Within3Sigma:  within,
		Level:         SurpriseLevel(z),
	})
}

func (s *Server) handlePredictionLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required to read the latest prediction", nil)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id query parameter is required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", sessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read predictions: %v", err), nil)
		return
	}
	var latestSeal map[string]any
	var latestEval map[string]any
	var latestAssert map[string]any
	for _, e := range events {
		if e.EventType == "prediction_sealed" && latestSeal == nil {
			var p map[string]any
			if err := json.Unmarshal(e.Payload, &p); err == nil {
				latestSeal = p
			}
		} else if e.EventType == "prediction_evaluated" && latestEval == nil {
			var p map[string]any
			if err := json.Unmarshal(e.Payload, &p); err == nil {
				latestEval = p
			}
		} else if e.EventType == "prediction_asserted" && latestAssert == nil {
			var p map[string]any
			if err := json.Unmarshal(e.Payload, &p); err == nil {
				latestAssert = p
			}
		}
		if latestSeal != nil && latestEval != nil && latestAssert != nil {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"sealed":     latestSeal,
		"evaluation": latestEval,
		"assertion":  latestAssert,
	})
}

// AssertPredictionRequest mints a formal assertion from a sealed prior and
// its evaluation. rho and evidence_refs are caller-supplied JUDGMENT — the
// server validates shape and lineage, never invents credibility.
type AssertPredictionRequest struct {
	SessionID      string   `json:"session_id"`
	PredictionHash string   `json:"prediction_hash"`
	Rho            float64  `json:"rho"`
	EvidenceRefs   []string `json:"evidence_refs"`
}

func (s *Server) handlePredictionAssert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to assert a prediction", nil)
		return
	}
	var req AssertPredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "malformed JSON request", nil)
		return
	}
	if req.SessionID == "" || req.PredictionHash == "" {
		WriteAPIError(w, "bad_request", "session_id and prediction_hash are required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	if math.IsNaN(req.Rho) || req.Rho <= 0 || req.Rho > 1 {
		WriteAPIError(w, "bad_request", "rho must lie in (0, 1] — state your credibility, the server will not invent it", nil)
		return
	}
	if len(req.EvidenceRefs) == 0 {
		WriteAPIError(w, "bad_request", "evidence_refs must be non-empty: no evidence-free assertions", nil)
		return
	}
	sealed, err := s.findSealedPrediction(req.SessionID, req.PredictionHash)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read sealed predictions: %v", err), nil)
		return
	}
	if sealed == nil {
		WriteAPIError(w, "not_found", fmt.Sprintf("no sealed prediction %q in this session", req.PredictionHash), nil)
		return
	}
	evaluated, err := s.findEvaluation(req.SessionID, req.PredictionHash)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read evaluations: %v", err), nil)
		return
	}
	if evaluated == nil {
		WriteAPIError(w, "bad_request", "prediction has no evaluation yet: measure before asserting", nil)
		return
	}

	if err := s.ledger.AppendEvent(req.SessionID, "prediction_asserted", map[string]any{
		"session_id":      req.SessionID,
		"prediction_hash": req.PredictionHash,
		"rho":             req.Rho,
		"evidence_refs":   req.EvidenceRefs,
	}); err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to record assertion: %v", err), nil)
		return
	}
	// R-09: mint the formal claim record beside the assertion event, same
	// convention as tribunal consensus (sentinel -1 alpha/beta/gamma, rho
	// carried in the statement + rho field is caller-stated, never computed).
	evMap := map[string]any{
		"evidence_refs":   req.EvidenceRefs,
		"prediction_hash": req.PredictionHash,
		"target_system":   sealed.TargetSystem,
		"method":          sealed.Method,
		"quantity":        sealed.Quantity,
		"expected_mean":   sealed.ExpectedMean,
		"expected_std":    sealed.ExpectedStd,
		"author":          sealed.Author,
		"rho":             req.Rho,
	}
	if mv, ok := evaluated["measured_value"]; ok {
		evMap["measured_value"] = mv
	}
	if sz, ok := evaluated["surprise_z"]; ok {
		evMap["surprise_z"] = sz
	}
	if bs, ok := evaluated["brier_score"]; ok {
		evMap["brier_score"] = bs
	}
	if sr, ok := evaluated["surprise_ratio"]; ok {
		evMap["surprise_ratio"] = sr
	}
	if w3, ok := evaluated["within_3sigma"]; ok {
		evMap["within_3sigma"] = w3
	}
	evBytes, _ := json.Marshal(evMap)
	claimRec, merr := s.ledger.MintClaimRecord(req.SessionID, "vault_assert",
		fmt.Sprintf("Vault assertion: prediction %s (%s %s) asserted at rho=%.3f", req.PredictionHash, sealed.TargetSystem, sealed.Quantity, req.Rho),
		-1, -1, -1, "asserted", string(evBytes))
	if merr != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to mint claim record: %v", merr), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "prediction_hash": req.PredictionHash, "rho": req.Rho,
		"claim_record_id": claimRec.ID,
	})
}

// findEvaluation returns the newest evaluation for a hash, or nil.
func (s *Server) findEvaluation(sessionID, hash string) (map[string]any, error) {
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.EventType != "prediction_evaluated" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		if h, _ := p["prediction_hash"].(string); h == hash {
			return p, nil
		}
	}
	return nil, nil
}

// handlePredictionCalibration aggregates per-author calibration from sealed
// evaluations: counts, mean Brier, mean |z|. Read-only feed for the
// (human-gated) question of whether calibration should tighten budgets.
func (s *Server) handlePredictionCalibration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required to read calibration", nil)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id query parameter is required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", sessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read predictions: %v", err), nil)
		return
	}
	type agg struct {
		Sealed    int     `json:"sealed"`
		Evaluated int     `json:"evaluated"`
		MeanBrier float64 `json:"mean_brier_score,omitempty"`
		MeanAbsZ  float64 `json:"mean_abs_z,omitempty"`
	}
	byAuthor := map[string]*agg{}
	authorOf := map[string]string{}
	var sumBrier, sumAbsZ float64
	var nEval int
	// Two passes: GetRecentEvents is newest-first, so an evaluation usually
	// precedes its seal in iteration order. Index authors first, then score.
	for _, e := range events {
		if e.EventType != "prediction_sealed" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		h, _ := p["prediction_hash"].(string)
		a, _ := p["author"].(string)
		if h == "" {
			continue
		}
		authorOf[h] = a
		if byAuthor[a] == nil {
			byAuthor[a] = &agg{}
		}
		byAuthor[a].Sealed++
	}
	for _, e := range events {
		if e.EventType != "prediction_evaluated" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		h, _ := p["prediction_hash"].(string)
		a := authorOf[h]
		if a == "" {
			a = "unknown"
		}
		if byAuthor[a] == nil {
			byAuthor[a] = &agg{}
		}
		byAuthor[a].Evaluated++
		if b, ok := p["brier_score"].(float64); ok {
			byAuthor[a].MeanBrier = (byAuthor[a].MeanBrier*float64(byAuthor[a].Evaluated-1) + b) / float64(byAuthor[a].Evaluated)
			sumBrier += b
		}
		if z, ok := p["surprise_z"].(float64); ok {
			az := z
			if az < 0 {
				az = -az
			}
			byAuthor[a].MeanAbsZ = (byAuthor[a].MeanAbsZ*float64(byAuthor[a].Evaluated-1) + az) / float64(byAuthor[a].Evaluated)
			sumAbsZ += az
		}
		nEval++
	}
	out := map[string]any{}
	for a, g := range byAuthor {
		out[a] = g
	}
	resp := map[string]any{"ok": true, "authors": out, "evaluations": nEval}
	if nEval > 0 {
		resp["mean_brier_score"] = sumBrier / float64(nEval)
		resp["mean_abs_z"] = sumAbsZ / float64(nEval)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePredictionHistory lists a session's sealed/evaluated/asserted
// predictions newest-first for the replay scrubber. Bounded by limit.
func (s *Server) handlePredictionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required to read prediction history", nil)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id query parameter is required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", sessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n >= 1 && n <= 200 {
			limit = n
		}
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", fmt.Sprintf("failed to read predictions: %v", err), nil)
		return
	}
	var out []map[string]any
	for _, e := range events {
		switch e.EventType {
		case "prediction_sealed", "prediction_evaluated", "prediction_asserted":
		default:
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		p["event_type"] = e.EventType
		out = append(out, p)
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(out), "predictions": out})
}
