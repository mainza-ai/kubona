// C5b: governed circuit-proposal rerun.
//
// The Python bridge remains a stateless evaluator. This rail owns the LLM
// call (one settings plane, one egress point), the loop, the BYOK estimate,
// and the sealed receipt. Cloud LLM is opt-in; the operator's key is billed
// by the provider, so Kubona records an estimate and enforces a local
// guardrail, never a Kubona charge. Local providers are free.
//
// Threat model: a session caller can spend the operator's provider key.
// The route is session-scoped, rate-limited, idempotent, bounded by steps/
// tokens/time/USD, and refuses before any egress when the bridge, spec,
// rate card, cloud opt-in, or air-gap posture is wrong. No bridge key moves.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

const (
	codesignRerunMaxBody          = 1 << 20
	codesignRerunDefaultSteps     = 4
	codesignRerunMaxSteps         = 12
	codesignRerunDefaultTokens    = 512
	codesignRerunMaxTokensPerStep = 1024
	codesignRerunDefaultTests     = 64
	codesignRerunMaxTests         = 512
	codesignRerunMaxOps           = 64
	codesignRerunTimeout          = 10 * time.Minute
	codesignRerunCallTimeout      = 90 * time.Second
	codesignDefaultRunBudgetUSD   = 0.50
)

type codesignRerunRequest struct {
	SessionID        string          `json:"session_id"`
	RequestID        string          `json:"request_id"`
	SpecKind         string          `json:"spec_kind"`
	SpecTable        json.RawMessage `json:"spec_table"`
	BudgetSteps      int             `json:"budget_steps"`
	Seed             int             `json:"seed"`
	NumTests         int             `json:"num_tests"`
	MaxFailures      int             `json:"max_failures"`
	Kappa            int             `json:"kappa"`
	MaxTokensPerStep int             `json:"max_tokens_per_step"`
	MaxUSD           float64         `json:"max_usd"`
}

type codesignSpecMeta struct {
	OK        bool   `json:"ok"`
	SpecKind  string `json:"spec_kind"`
	NumQubits int    `json:"num_qubits"`
	Error     string `json:"error"`
}

type codesignOp struct {
	Name   string
	Qubits []int
	Cond   []int
}

func (o codesignOp) wire() []any {
	q := make([]any, len(o.Qubits))
	for i, v := range o.Qubits {
		q[i] = v
	}
	c := make([]any, len(o.Cond))
	for i, v := range o.Cond {
		c[i] = v
	}
	return []any{o.Name, q, c}
}

func wireOps(ops []codesignOp) []any {
	out := make([]any, 0, len(ops))
	for _, o := range ops {
		out = append(out, o.wire())
	}
	return out
}

type codesignEvalResult struct {
	Valid      bool        `json:"valid"`
	Toffoli    int         `json:"toffoli"`
	PeakQubits int         `json:"peak_qubits"`
	TQ         json.Number `json:"tq"`
	EPSBound   *float64    `json:"eps_bound_97_5"`
	Reason     string      `json:"reason"`
}

type codesignTrial struct {
	Step       int         `json:"step"`
	Valid      bool        `json:"valid"`
	Toffoli    int         `json:"toffoli"`
	PeakQubits int         `json:"peak_qubits"`
	TQ         json.Number `json:"tq"`
	EPSBound   *float64    `json:"eps_bound_97_5,omitempty"`
	Reason     string      `json:"reason"`
	Candidate  string      `json:"candidate"`
}

type codesignRerunResponse struct {
	OK               bool            `json:"ok"`
	RunID            string          `json:"run_id"`
	SessionID        string          `json:"session_id"`
	SpecKind         string          `json:"spec_kind"`
	BudgetSteps      int             `json:"budget_steps"`
	Seed             int             `json:"seed"`
	NumTests         int             `json:"num_tests"`
	MaxFailures      int             `json:"max_failures"`
	Kappa            int             `json:"kappa"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	Billing          string          `json:"billing"`
	Payer            string          `json:"payer"`
	EstimatedUSD     float64         `json:"estimated_usd"`
	BudgetUSD        float64         `json:"budget_usd"`
	Steps            int             `json:"steps"`
	Trials           []codesignTrial `json:"trials"`
	Best             *codesignTrial  `json:"best"`
	StoppedReason    string          `json:"stopped_reason,omitempty"`
	ProvenanceStatus string          `json:"provenance_status"`
	LedgerRef        any             `json:"ledger_ref,omitempty"`
}

func (s *Server) handleCodesignRerun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "codesign rerun is POST-only", nil)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req codesignRerunRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, codesignRerunMaxBody))
	if err != nil {
		WriteAPIError(w, "request_too_large", "body too large or unreadable: refusing", nil)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "invalid request body: "+err.Error(), nil)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		WriteAPIError(w, "bad_request", "request body has trailing data after the JSON object: refusing", nil)
		return
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.SpecKind = strings.TrimSpace(req.SpecKind)
	if strings.TrimSpace(string(req.SpecTable)) == "null" {
		req.SpecTable = nil
	}
	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	if req.RequestID == "" {
		WriteAPIError(w, "bad_request", "request_id is required on this spend rail (retries must replay the receipt)", nil)
		return
	}
	if err := validateCodesignRerun(&req); err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	// Spend/egress rail: no anonymous path, even in single-lab local mode.
	// The zero-login flow may read and create sessions, but it may not spend
	// the operator's provider key (or invoke a local model) without a
	// logged-in session or the operator bearer.
	if hasNoCredentials(r) {
		WriteAPIError(w, "unauthorized", "codesign rerun requires a logged-in session or operator token: anonymous spend is refused", nil)
		return
	}
	sess, ok := s.authorizeKnownSession(w, r, req.SessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing rerun", req.SessionID), nil)
		return
	}
	if !s.codesignLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "codesign rerun rate limit exceeded: back off and retry", nil)
		return
	}

	content := map[string]any{
		"session_id": req.SessionID, "spec_kind": req.SpecKind,
		"spec_table": req.SpecTable, "budget_steps": req.BudgetSteps,
		"seed": req.Seed, "num_tests": req.NumTests, "max_failures": req.MaxFailures,
		"kappa": req.Kappa, "max_tokens_per_step": req.MaxTokensPerStep,
		"max_usd": req.MaxUSD,
	}
	idem, iref := s.idemBegin(req.SessionID, req.RequestID, content, idemRailSlow)
	if iref != nil {
		iref.write(w)
		return
	}
	if idem.replay {
		s.idemReplay(w, idem)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), codesignRerunTimeout)
	defer cancel()

	runID := uuid.NewString()
	spec, err := s.codesignSpec(ctx, req)
	if err != nil {
		code := "bad_request"
		if strings.Contains(err.Error(), "evaluator unavailable") {
			code = "bridge_unavailable"
		}
		s.idemAPIError(w, idem, code, "spec refused: "+err.Error())
		return
	}
	prov, model, err := s.llmSettings.GetModelForRole("codesign")
	if err != nil {
		s.idemAPIError(w, idem, "llm_unavailable", "codesign role unavailable: "+err.Error())
		return
	}
	local := llm.IsLocalProvider(prov)
	budget := 0.0
	if !local {
		if os.Getenv("KUBONA_LLM_CLOUD") != "1" {
			s.idemAPIError(w, idem, "forbidden", fmt.Sprintf("cloud LLM provider %s requires KUBONA_LLM_CLOUD=1 (prompts would leave the box)", prov))
			return
		}
		ceiling, ok := llm.EstimateCeilingCostUSD(prov, model, req.MaxTokensPerStep)
		if !ok {
			s.idemAPIError(w, idem, "unpriced_model", fmt.Sprintf("role codesign model %q has no valid ceiling rate: refusing cloud spend", model))
			return
		}
		var berr error
		budget, berr = codesignRunBudget(req.MaxUSD)
		if berr != nil {
			s.idemAPIError(w, idem, "bad_request", berr.Error())
			return
		}
		if ceiling*float64(req.BudgetSteps) > budget {
			s.idemAPIError(w, idem, "ceiling_exceeded", fmt.Sprintf("worst-case estimate ~$%.4f exceeds the per-run budget $%.4f: lower steps/tokens or raise KUBONA_CODESIGN_RUN_BUDGET_USD", ceiling*float64(req.BudgetSteps), budget))
			return
		}
		if err := s.checkLLMBudget(); err != nil {
			s.idemAPIError(w, idem, "ceiling_exceeded", err.Error())
			return
		}
	}

	resp, runErr := s.runCodesignRerun(ctx, runID, req, spec, prov, model, local, budget)
	if runErr != nil {
		s.idemAPIError(w, idem, runErr.code, runErr.message)
		return
	}
	payload := map[string]any{
		"harness": "circuit_codesign_rerun", "run_id": resp.RunID,
		"spec_kind": resp.SpecKind, "spec_label": resp.SpecKind,
		"seed": resp.Seed, "budget_steps": resp.BudgetSteps,
		"best_tq": nil, "best_valid": resp.Best != nil,
		"artifact": "none", "estimated_usd": resp.EstimatedUSD,
		"provider": resp.Provider, "model": resp.Model,
		"billing": resp.Billing, "payer": resp.Payer,
		"stopped_reason": resp.StoppedReason,
	}
	if resp.Best != nil {
		payload["best_tq"] = resp.Best.TQ
		payload["artifact"] = resp.Best.Candidate
	}
	sealed, ref, lerr := s.ledger.AppendEventWithSpoolRef(req.SessionID, "trial", payload)
	if lerr != nil {
		s.idemAPIError(w, idem, "internal", fmt.Sprintf("run %s completed but provenance is unavailable (ledger and spool failed): %v", resp.RunID, lerr))
		return
	}
	if sealed {
		resp.ProvenanceStatus = "sealed"
		resp.LedgerRef = ref
	} else {
		resp.ProvenanceStatus = "pending"
	}
	s.idemFinishJSON(w, idem, http.StatusOK, resp)
}

func validateCodesignRerun(req *codesignRerunRequest) error {
	if req.SpecKind == "" && len(req.SpecTable) == 0 {
		return fmt.Errorf("spec_kind or spec_table is required")
	}
	if req.SpecKind != "" && len(req.SpecTable) > 0 {
		return fmt.Errorf("send exactly one of spec_kind or spec_table: refusing ambiguity")
	}
	if req.BudgetSteps == 0 {
		req.BudgetSteps = codesignRerunDefaultSteps
	}
	if req.BudgetSteps < 1 || req.BudgetSteps > codesignRerunMaxSteps {
		return fmt.Errorf("budget_steps must be in [1,%d]", codesignRerunMaxSteps)
	}
	if req.Seed < 0 {
		return fmt.Errorf("seed must be >= 0")
	}
	if req.NumTests == 0 {
		req.NumTests = codesignRerunDefaultTests
	}
	if req.NumTests < 1 || req.NumTests > codesignRerunMaxTests {
		return fmt.Errorf("num_tests must be in [1,%d]", codesignRerunMaxTests)
	}
	if req.MaxFailures < 0 || req.MaxFailures > req.NumTests {
		return fmt.Errorf("max_failures must be in [0,num_tests]")
	}
	if req.Kappa == 0 {
		req.Kappa = 1
	}
	if req.Kappa < 1 || req.Kappa > 3 {
		return fmt.Errorf("kappa must be in [1,3]")
	}
	if req.MaxTokensPerStep == 0 {
		req.MaxTokensPerStep = codesignRerunDefaultTokens
	}
	if req.MaxTokensPerStep < 1 || req.MaxTokensPerStep > codesignRerunMaxTokensPerStep {
		return fmt.Errorf("max_tokens_per_step must be in [1,%d]", codesignRerunMaxTokensPerStep)
	}
	if req.MaxUSD < 0 {
		return fmt.Errorf("max_usd must be >= 0")
	}
	return ValidateRequestID(req.RequestID)
}

func codesignRunBudget(reqMax float64) (float64, error) {
	if reqMax > 0 {
		return reqMax, nil
	}
	raw := strings.TrimSpace(os.Getenv("KUBONA_CODESIGN_RUN_BUDGET_USD"))
	if raw == "" {
		return codesignDefaultRunBudgetUSD, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("KUBONA_CODESIGN_RUN_BUDGET_USD must be a positive number, got %q", raw)
	}
	return v, nil
}

func (s *Server) codesignSpec(ctx context.Context, req codesignRerunRequest) (codesignSpecMeta, error) {
	body, err := json.Marshal(codesignSpecBody(req))
	if err != nil {
		return codesignSpecMeta{}, err
	}
	out, err := s.codesignOp(ctx, "spec", body)
	if err != nil {
		return codesignSpecMeta{}, err
	}
	var meta codesignSpecMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return codesignSpecMeta{}, fmt.Errorf("spec response unparseable: %w", err)
	}
	if meta.NumQubits < 1 || meta.NumQubits > 64 {
		return codesignSpecMeta{}, fmt.Errorf("spec returned num_qubits=%d outside [1,64]", meta.NumQubits)
	}
	return meta, nil
}

func codesignSpecBody(req codesignRerunRequest) map[string]any {
	if len(req.SpecTable) > 0 {
		return map[string]any{"spec_table": req.SpecTable}
	}
	return map[string]any{"spec_kind": req.SpecKind}
}

func (s *Server) codesignOp(ctx context.Context, operation string, body []byte) ([]byte, error) {
	if out, ok := s.codesignViaBridge(ctx, operation, body); ok {
		return codesignEnvelope(out)
	}
	shim := "import json, sys\nfrom codesign_ops import run_op\nprint(json.dumps(run_op(sys.argv[1], json.loads(sys.argv[2]))))"
	out, _, err := runBridgePython(ctx, 120*time.Second, shim, operation, string(body))
	if err != nil {
		return nil, fmt.Errorf("codesign evaluator unavailable: %w", err)
	}
	return codesignEnvelope(out)
}

func codesignEnvelope(out []byte) ([]byte, error) {
	clean := bytes.TrimSpace(out)
	if idx := bytes.Index(clean, []byte("{")); idx >= 0 {
		clean = clean[idx:]
	}
	var env struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(clean, &env); err != nil {
		return nil, fmt.Errorf("codesign evaluator returned unparseable output: %w", err)
	}
	if env.OK != nil && !*env.OK {
		if env.Error == "" {
			env.Error = "codesign operation refused"
		}
		return nil, fmt.Errorf("%s", env.Error)
	}
	return clean, nil
}

func (s *Server) evaluateCodesign(ctx context.Context, req codesignRerunRequest, ops []codesignOp) (codesignEvalResult, error) {
	body := codesignSpecBody(req)
	body["num_tests"] = req.NumTests
	body["max_failures"] = req.MaxFailures
	body["kappa"] = req.Kappa
	body["seed"] = req.Seed
	body["artifact"] = wireOps(ops)
	raw, err := json.Marshal(body)
	if err != nil {
		return codesignEvalResult{}, err
	}
	out, err := s.codesignOp(ctx, "evaluate", raw)
	if err != nil {
		return codesignEvalResult{}, err
	}
	var env struct {
		OK       bool               `json:"ok"`
		SpecKind string             `json:"spec_kind"`
		Result   codesignEvalResult `json:"result"`
		Error    string             `json:"error"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return codesignEvalResult{}, fmt.Errorf("evaluate response unparseable: %w", err)
	}
	if !env.OK {
		if env.Error == "" {
			env.Error = "evaluation refused"
		}
		return codesignEvalResult{}, fmt.Errorf("%s", env.Error)
	}
	return env.Result, nil
}

type codesignRunFailure struct {
	code    string
	message string
}

func (s *Server) runCodesignRerun(ctx context.Context, runID string, req codesignRerunRequest, spec codesignSpecMeta, prov llm.ProviderType, model string, local bool, budget float64) (codesignRerunResponse, *codesignRunFailure) {
	resp := codesignRerunResponse{
		OK: true, RunID: runID, SessionID: req.SessionID, SpecKind: spec.SpecKind,
		BudgetSteps: req.BudgetSteps, Seed: req.Seed, NumTests: req.NumTests,
		MaxFailures: req.MaxFailures, Kappa: req.Kappa,
		Provider: string(prov), Model: model, BudgetUSD: budget,
		Billing: "none", Payer: "local", Trials: []codesignTrial{},
	}
	if !local {
		resp.Billing = "provider"
		resp.Payer = "user-key"
	}
	current := []codesignOp{}
	var best *codesignTrial
	stopped := ""
	for step := 0; step < req.BudgetSteps; step++ {
		if !local {
			if err := s.checkLLMBudget(); err != nil {
				stopped = "global_budget"
				break
			}
			if resp.EstimatedUSD >= budget {
				stopped = "budget"
				break
			}
		}
		prompt := buildCodesignPrompt(spec.NumQubits, step, codesignRerunMaxOps, current)
		callCtx, cancel := context.WithTimeout(ctx, codesignRerunCallTimeout)
		text, chatResp, _, err := s.llmClient.CompleteCapped(callCtx, "codesign", []llm.ChatMessage{
			{Role: "system", Content: "You propose reversible Kickmix circuits. Reply with JSON only."},
			{Role: "user", Content: prompt},
		}, req.MaxTokensPerStep)
		cancel()
		if err != nil {
			resp.Trials = append(resp.Trials, codesignTrial{Step: step, Valid: false, Reason: truncateReason("llm error: " + err.Error())})
			stopped = "llm_error"
			break
		}
		if !local {
			cost, priced := codesignCallCost(prov, model, chatResp, req.MaxTokensPerStep)
			if !priced {
				resp.Trials = append(resp.Trials, codesignTrial{Step: step, Valid: false, Reason: "cloud call completed but the model could not be priced: stopping (no silent uncounted spend)"})
				stopped = "unpriced_model"
				break
			}
			resp.EstimatedUSD += cost
		} else {
			resp.EstimatedUSD = 0
		}
		candidate, perr := parseCodesignProposal(text, spec.NumQubits, codesignRerunMaxOps)
		if perr != nil {
			resp.Trials = append(resp.Trials, codesignTrial{Step: step, Valid: false, Reason: truncateReason("proposal refused: " + perr.Error())})
			continue
		}
		res, eerr := s.evaluateCodesign(ctx, req, candidate)
		if eerr != nil {
			resp.Trials = append(resp.Trials, codesignTrial{Step: step, Valid: false, Reason: truncateReason("evaluator error: " + eerr.Error()), Candidate: summarizeCodesignOps(candidate)})
			stopped = "bridge_error"
			break
		}
		trial := codesignTrial{
			Step: step, Valid: res.Valid, Toffoli: res.Toffoli,
			PeakQubits: res.PeakQubits, TQ: res.TQ, EPSBound: res.EPSBound,
			Reason: truncateReason(res.Reason), Candidate: summarizeCodesignOps(candidate),
		}
		resp.Trials = append(resp.Trials, trial)
		resp.Steps = step + 1
		if res.Valid {
			tq, _ := res.TQ.Float64()
			bestTQ := 0.0
			if best != nil {
				bestTQ, _ = best.TQ.Float64()
			}
			if best == nil || tq < bestTQ {
				b := trial
				best = &b
			}
			if best != nil && tq <= mustFloat(best.TQ) {
				current = candidate
			}
		}
		if !local && resp.EstimatedUSD >= budget {
			stopped = "budget"
			break
		}
	}
	resp.Steps = len(resp.Trials)
	resp.Best = best
	if stopped == "" {
		if best == nil {
			stopped = "no_valid_candidate"
		} else {
			stopped = "step_limit"
		}
	}
	resp.StoppedReason = stopped
	return resp, nil
}

func codesignCallCost(prov llm.ProviderType, model string, resp *llm.ChatCompletionResponse, maxTokens int) (float64, bool) {
	if resp != nil && resp.Usage.TotalTokens > 0 {
		if cost, ok := llm.EstimateCostUSD(prov, model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens); ok {
			return cost, true
		}
	}
	if cost, ok := llm.EstimateCeilingCostUSD(prov, model, maxTokens); ok {
		return cost, true
	}
	return 0, false
}

func mustFloat(n json.Number) float64 {
	v, _ := n.Float64()
	return v
}

func truncateReason(s string) string {
	if len(s) > 280 {
		return s[:280]
	}
	return s
}

func summarizeCodesignOps(ops []codesignOp) string {
	names := make([]string, 0, len(ops))
	for i, op := range ops {
		if i >= 8 {
			break
		}
		names = append(names, op.Name)
	}
	return fmt.Sprintf("%dops:%s", len(ops), strings.Join(names, ","))
}

func buildCodesignPrompt(numQubits, step, maxOps int, current []codesignOp) string {
	gates := make([]string, 0, len(codesignGateArity))
	for g := range codesignGateArity {
		gates = append(gates, g)
	}
	sort.Strings(gates)
	raw, _ := json.Marshal(wireOps(current))
	return fmt.Sprintf("Complete the reversible Kickmix circuit. Allowed gates and qubit arity: %s. "+
		"Only diagonal gates may carry a cond list of measurement-record ids. "+
		"num_qubits=%d; step=%d; max_ops=%d. Current ops: %s. "+
		`Reply with JSON only: {"ops": [[name, [qubits], [cond]], ...]}.`,
		strings.Join(gates, ", "), numQubits, step, maxOps, string(raw))
}

var codesignGateArity = map[string]int{
	"x": 1, "cx": 2, "ccx": 3, "swap": 2,
	"z": 1, "cz": 2, "ccz": 3, "global_flip": 0, "hmr": 1,
	"alloc_marker": 1, "free_marker": 1,
}

func parseCodesignProposal(text string, numQubits, maxOps int) ([]codesignOp, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty proposal")
	}
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.Trim(raw, "`")
		if strings.HasPrefix(raw, "json") {
			raw = raw[4:]
		}
		raw = strings.TrimSpace(raw)
	}
	start := -1
	for _, tok := range []string{"{", "["} {
		if i := strings.Index(raw, tok); i >= 0 && (start < 0 || i < start) {
			start = i
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("proposal contains no JSON object/array")
	}
	dec := json.NewDecoder(strings.NewReader(raw[start:]))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("proposal is not valid JSON: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("proposal has trailing data after the JSON value: refusing")
	}
	var opsRaw any
	switch v := doc.(type) {
	case map[string]any:
		opsRaw = v["ops"]
	case []any:
		opsRaw = v
	default:
		return nil, fmt.Errorf("proposal must be an object with ops or a bare array")
	}
	ops, ok := opsRaw.([]any)
	if !ok || len(ops) == 0 {
		return nil, fmt.Errorf("proposal must contain a non-empty ops list")
	}
	if len(ops) > maxOps {
		return nil, fmt.Errorf("proposal has %d ops, cap is %d", len(ops), maxOps)
	}
	out := make([]codesignOp, 0, len(ops))
	for idx, rawOp := range ops {
		name, qubits, cond, err := parseCodesignOp(rawOp)
		if err != nil {
			return nil, fmt.Errorf("op %d: %w", idx, err)
		}
		arity, ok := codesignGateArity[name]
		if !ok {
			return nil, fmt.Errorf("op %d has unknown gate %q", idx, name)
		}
		if len(qubits) != arity {
			return nil, fmt.Errorf("op %d gate %q needs %d qubit(s), got %d", idx, name, arity, len(qubits))
		}
		for _, q := range qubits {
			if q < 0 || q >= numQubits {
				return nil, fmt.Errorf("op %d qubit %d is outside [0,%d)", idx, q, numQubits)
			}
		}
		for _, c := range cond {
			if c < 0 {
				return nil, fmt.Errorf("op %d cond record %d is invalid", idx, c)
			}
		}
		out = append(out, codesignOp{Name: name, Qubits: qubits, Cond: cond})
	}
	return out, nil
}

func parseCodesignOp(raw any) (string, []int, []int, error) {
	switch v := raw.(type) {
	case map[string]any:
		name, _ := v["name"].(string)
		q, err := numberList(v["qubits"], "qubits")
		if err != nil {
			return "", nil, nil, err
		}
		c, err := numberList(v["cond"], "cond")
		if err != nil {
			return "", nil, nil, err
		}
		return name, q, c, nil
	case []any:
		if len(v) != 3 {
			return "", nil, nil, fmt.Errorf("must be {name,qubits,cond} or [name,qubits,cond]")
		}
		name, _ := v[0].(string)
		q, err := numberList(v[1], "qubits")
		if err != nil {
			return "", nil, nil, err
		}
		c, err := numberList(v[2], "cond")
		if err != nil {
			return "", nil, nil, err
		}
		return name, q, c, nil
	default:
		return "", nil, nil, fmt.Errorf("must be {name,qubits,cond} or [name,qubits,cond]")
	}
}

func numberList(raw any, label string) ([]int, error) {
	if raw == nil {
		return []int{}, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", label)
	}
	out := make([]int, 0, len(list))
	for _, v := range list {
		n, ok := v.(json.Number)
		if !ok {
			return nil, fmt.Errorf("%s item %v is not an integer", label, v)
		}
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("%s item %v is not an integer", label, v)
		}
		out = append(out, int(i))
	}
	return out, nil
}

func (s *Server) idemAPIError(w http.ResponseWriter, t *idemTicket, code, message string) {
	e, err := NewAPIError(code, message, nil)
	if err != nil {
		s.idemFinishError(w, t, http.StatusInternalServerError, message)
		return
	}
	s.idemFinishJSON(w, t, e.HTTPStatus, e)
}
