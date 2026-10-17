package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/memory"
)

// Memory write-back: recall_memory can only find what was written, and until
// now nothing in production wrote — the memory loop stood open. After each
// completed model turn we retain one bounded experience, tenant-scoped with
// the tenant as its own readership, mirroring exactly the scoping the recall
// tool enforces (SemanticRecallScoped with principals [tenant]). A memory the
// recall path cannot admit is never written.
//
// Deliberately excluded: rewrite (a user edit, not a model turn) and refusal
// shells with no model output. Regenerate counts — each fresh answer the user
// saw is a real turn.

// maxExperienceRunes bounds a single retained experience. Memories are recall
// context, not transcripts; the prompt names the cause, the outcome names the
// result, and neither stores whole messages.
const maxExperienceRunes = 600

func boundRunes(s string, n int) string {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > n {
		t = string([]rune(t)[:n]) + "…"
	}
	return t
}

// latestEventID returns the session's newest ledger event id, or 0 when the
// session has no events yet. Used to attribute tool calls to the turn that
// just ran instead of earlier turns.
func (s *Server) latestEventID(sessionID string) int64 {
	if s.ledger == nil {
		return 0
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 1)
	if err != nil || len(events) == 0 {
		return 0
	}
	return events[0].ID
}

// toolNamesSince names the tools this turn actually dispatched, in order,
// deduped, bounded. Only tool_called events newer than beforeID count —
// attributing an earlier turn's tools to this one would write a false
// experience. Unknown on ledger error (empty, never guessed).
func (s *Server) toolNamesSince(sessionID string, beforeID int64) []string {
	if s.ledger == nil {
		return nil
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 100)
	if err != nil {
		return nil
	}
	var names []string
	seen := map[string]bool{}
	// GetRecentEvents returns newest first; walk backwards for order.
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.ID <= beforeID || e.EventType != "tool_called" {
			continue
		}
		var p struct {
			Tool string `json:"tool"`
		}
		if err := json.Unmarshal([]byte(e.Payload), &p); err != nil || p.Tool == "" {
			continue
		}
		if !seen[p.Tool] {
			seen[p.Tool] = true
			names = append(names, p.Tool)
		}
		if len(names) >= 8 {
			break
		}
	}
	return names
}

// chatOutcome classifies a turn's result from what the call site actually
// observed — never inferred. proposalJSON presence, generation error, and
// empty reply are the only inputs.
func chatOutcome(reply string, hasProposal bool, genErr error) string {
	if genErr != nil && strings.TrimSpace(reply) == "" {
		return "engine-error:" + boundRunes(genErr.Error(), 80)
	}
	if hasProposal {
		return "answered-with-proposal"
	}
	if genErr != nil {
		return "answered-despite-engine-error"
	}
	return "answered"
}

// chatImportance rates retention: proposal-bearing and tool-using turns carry
// more future value than plain answers. Values stay inside (0,1]; the store
// treats them as relative rank, not absolute truth.
func chatImportance(hasProposal, hadError, usedTools bool) float64 {
	switch {
	case hasProposal && usedTools:
		return 0.8
	case hasProposal || usedTools:
		return 0.65
	case hadError:
		return 0.4
	default:
		return 0.5
	}
}

// replanCampaignOnSettled is the job settled-hook: a COMPLETED job's
// measured result folds back into the session campaign, saved as a new
// version so plan evolution stays auditable. Sessions without a campaign,
// undecodable payloads, and folds with nothing honest to change all skip
// silently — the job_settled event already records the outcome, and a
// no-change version would be audit spam.
func (s *Server) replanCampaignOnSettled(j *jobs.Job) {
	if s.ledger == nil || j == nil || len(j.ResultJSON) == 0 {
		return
	}
	// claim_eval verdicts mint a claim record first, independent of any
	// campaign: the evaluation happened, and its record exists whether or
	// not the plan applies it. The ID then cites the replan line.
	cite := ""
	if j.JobType == "claim_eval" {
		if draft, ok := agents.EvalClaimRecord(j.ResultJSON); ok {
			evBytes, _ := json.Marshal(map[string]any{
				"job_id":            j.ID,
				"external_claim_id": draft.ExternalID,
				"delta_mha":         evalDeltaMHa(j.ResultJSON),
			})
			if rec, merr := s.ledger.MintClaimRecord(j.SessionID, "claim_eval",
				"Evaluated claim: "+draft.Statement,
				draft.Alpha, draft.Beta, draft.Gamma, draft.Verdict, string(evBytes)); merr == nil {
				cite = rec.ID
			}
		}
	}
	rawState, err := s.ledger.GetCampaignState(j.SessionID)
	if err != nil || len(rawState) == 0 {
		return
	}
	updated, changed, note := agents.ReplanWithJobResult(rawState, j.ID, j.JobType, j.ResultJSON, cite)
	if !changed {
		return
	}
	shortID := j.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	s.ledger.SaveCampaignStateWithTrigger(j.SessionID, updated, "replan:"+j.JobType+":"+shortID)
	_ = s.ledger.AppendEvent(j.SessionID, "campaign_replanned", map[string]any{
		"job":      j.ID,
		"job_type": j.JobType,
		"note":     note,
	})
}

// evalDeltaMHa extracts the evaluated discrepancy for claim evidence.
// Absent or undecodable means unknown — never zero-filled.
func evalDeltaMHa(resultJSON []byte) *float64 {
	var res map[string]json.RawMessage
	if err := json.Unmarshal(resultJSON, &res); err != nil {
		return nil
	}
	raw, ok := res["delta_mha"]
	if !ok {
		return nil
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return &v
}

// auditTribunalTools records tribunal observe-round dispatches as
// tool_called ledger events — the same shape as the copilot path, so
// tribunal tool use is attributable instead of silent. Arguments stay out
// of the event; name, outcome, and error text are what the audit needs.
func (s *Server) auditTribunalTools(sessionID string, turns []agents.TribunalTurn) {
	if s.ledger == nil {
		return
	}
	for _, turn := range turns {
		for _, c := range turn.Observed {
			_ = s.ledger.AppendEvent(sessionID, "tool_called", map[string]any{
				"tool":      c.Name,
				"ok":        c.OK,
				"error":     c.Error,
				"source":    c.Source,
				"untrusted": c.Untrusted,
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Hindsight over HTTP (B1-2): the in-process loop (recall_memory tool +
// turn write-back) is joined by two session-scoped routes sharing the exact
// recall path and scoping. Read-only recall needs no new trust: it returns
// what the tenant's own agent turns could already read.
// ---------------------------------------------------------------------------

// maxRetainChars bounds operator-retained memories. Recall budgets and bank
// eviction bound reads; this bounds writes against pasting a corpus.
const maxRetainChars = 4000

func memoryBankOrDefault(raw string) (memory.MemoryBank, bool) {
	switch memory.MemoryBank(strings.TrimSpace(raw)) {
	case "", memory.BankExperiences:
		return memory.BankExperiences, true
	case memory.BankWorldFacts:
		return memory.BankWorldFacts, true
	case memory.BankMentalModels:
		return memory.BankMentalModels, true
	default:
		return "", false
	}
}

func (s *Server) resolveMemoryTenant(sessionID string) (string, error) {
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	if sess.TenantID != "" {
		return sess.TenantID, nil
	}
	return "default", nil
}

// recallParams parses the shared recall query contract: bank (validated,
// empty/all = every bank), token budget, similarity floor, and count cap.
// One parser serves the route now and any future recall caller — no
// per-caller drift in bounds or defaults. Tool parity: the recall_memory
// tool queries experiences at 250 tokens / 0.35 floor; those are the
// defaults here too, overridable per call.
type recallParams struct {
	bank     memory.MemoryBank
	matchAll bool
	tokens   int
	minSim   float64
	limit    int
}

func parseRecallParams(r *http.Request) (recallParams, error) {
	cfg := llm.DefaultBudgetConfig()
	p := recallParams{tokens: cfg.RecallTokens, minSim: cfg.RecallMinSimilarity, limit: 10}
	rawBank := strings.TrimSpace(r.URL.Query().Get("bank"))
	if rawBank == "" || rawBank == "all" {
		p.matchAll = true
	} else if bank, ok := memoryBankOrDefault(rawBank); ok {
		p.bank = bank
	} else {
		return p, fmt.Errorf("bank must be all, experiences, world-facts, or mental-models")
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		p.limit = n
	}
	if p.limit > 50 {
		p.limit = 50
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("tokens")); err == nil && n > 0 {
		p.tokens = n
	}
	if p.tokens > 2000 {
		p.tokens = 2000
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("min_similarity")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 || v > 1 {
			return p, fmt.Errorf("min_similarity must be within 0..1")
		}
		p.minSim = v
	}
	return p, nil
}

// handleMemoryRecall serves GET /api/memory/recall?session_id=&q=&bank=
// &limit=&tokens=&min_similarity=. Same SemanticRecallScoped path as the
// recall_memory tool — the HTTP surface observes, never widens.
func (s *Server) handleMemoryRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"snippets": []any{}})
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	tenant, err := s.resolveMemoryTenant(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	params, err := parseRecallParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bank := params.bank
	if params.matchAll {
		bank = ""
	}
	query := r.URL.Query().Get("q")
	out := make([]any, 0)
	if strings.TrimSpace(query) == "" {
		// Inventory mode: an empty query embeds to the zero vector, so
		// semantic search would return nothing meaningful. List newest
		// instead, same budget and boundary, similarity null (no match
		// was computed — never a fabricated score).
		for _, item := range s.memoryStore.ListRecent(bank, tenant, params.tokens) {
			if len(out) >= params.limit {
				break
			}
			out = append(out, map[string]any{
				"content":    item.Content,
				"bank":       string(item.Bank),
				"similarity": nil,
				"created_at": item.CreatedAt,
			})
		}
	} else {
		recalled := s.memoryStore.SemanticRecallScoped(
			bank, query, params.tokens, params.minSim, tenant, []string{tenant})
		for _, item := range recalled {
			if len(out) >= params.limit {
				break
			}
			out = append(out, map[string]any{
				"content":    item.Item.Content,
				"bank":       string(item.Item.Bank),
				"similarity": item.Similarity,
				"created_at": item.Item.CreatedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snippets": out, "tenant": tenant, "session_id": sessionID,
	})
}

// handleMemoryRetain serves POST /api/memory/retain
// {session_id, content, importance?, bank?}. Tenant-scoped with the tenant
// as its own readership — identical to turn write-back, so retained rows
// are recallable through the same IFC check and invisible elsewhere.
func (s *Server) handleMemoryRetain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		http.Error(w, "memory store unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		SessionID  string   `json:"session_id"`
		Content    string   `json:"content"`
		Importance *float64 `json:"importance"`
		Bank       string   `json:"bank"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	tenant, err := s.resolveMemoryTenant(req.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		http.Error(w, "content must not be empty", http.StatusBadRequest)
		return
	}
	if len([]rune(content)) > maxRetainChars {
		http.Error(w, "content exceeds 4000 characters", http.StatusBadRequest)
		return
	}
	bank, ok := memoryBankOrDefault(req.Bank)
	if !ok {
		http.Error(w, "bank must be experiences, world-facts, or mental-models", http.StatusBadRequest)
		return
	}
	// Pointer distinguishes absent (default 0.5) from explicit 0: an
	// explicitly unimportant memory is legal and must survive as 0.
	importance := 0.5
	if req.Importance != nil {
		importance = *req.Importance
	}
	if importance < 0 {
		importance = 0
	}
	if importance > 1 {
		importance = 1
	}
	item := s.memoryStore.RetainScoped(tenant, tenant, bank, content, importance, nil)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": item.ID, "bank": string(item.Bank), "importance": item.Importance,
		"tenant": item.TenantID, "created_at": item.CreatedAt,
	})
}

// retainTurnExperience stores one bounded experience for a completed model
// turn. Importance is caller-rated: tool-using and proposal-bearing turns
// outrank plain answers. Never fails the turn on memory error — retention is
// best-effort, the reply already went out.
func (s *Server) retainTurnExperience(sessionID, kind, prompt, outcome string, tools []string, importance float64) {
	if s.memoryStore == nil {
		return
	}
	prompt = boundRunes(prompt, 400)
	outcome = boundRunes(outcome, 120)
	if prompt == "" && outcome == "" {
		return
	}
	var b strings.Builder
	b.WriteString(kind)
	b.WriteString(" turn | user: ")
	b.WriteString(prompt)
	b.WriteString(" | outcome: ")
	b.WriteString(outcome)
	if len(tools) > 0 {
		b.WriteString(" | tools: ")
		b.WriteString(strings.Join(tools, ","))
	}
	content := boundRunes(b.String(), maxExperienceRunes)

	// W2.20: fail-closed skip — an unknown session previously fell back to
	// the "default" tenant and wrote one lab's turn into another's memory
	// bank. Ghost turns now write nothing (the reply already went out;
	// retention is best-effort, never cross-tenant).
	tenant := ""
	if s.sessionMgr != nil {
		sess, err := s.sessionMgr.GetSession(sessionID)
		if err != nil || sess.TenantID == "" {
			return
		}
		tenant = sess.TenantID
	} else {
		return
	}
	s.memoryStore.RetainScoped(tenant, tenant, memory.BankExperiences, content, importance, nil)
}
