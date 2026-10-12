package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/tools"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// CopilotRole is the caller role for chat tool calls. Deny by default: the
// registry grants this role an explicit read-only set at registration.
const CopilotRole = "copilot"

// registerAgentTools wires the single tool registry with server-backed v1
// tools. All v1 tools are read-only observations (anchors, campaign, jobs,
// ledger tail, literature). Nothing here spends, dispatches, or mutates:
// writes keep flowing through the existing gated paths (launch, mandates).
func (s *Server) registerAgentTools() *tools.Registry {
	r := tools.NewRegistry()
	bridge := func() pb.QuantumBridgeServiceClient {
		if s.jobRunner == nil {
			return nil
		}
		return s.jobRunner.BridgeClient()
	}

	_ = r.Register(tools.Definition{
		Name:        "get_anchor",
		Description: "Compute the classical anchor (RHF/FCI energies) for a molecule at a bond distance. Use before citing any energy.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"molecule":      map[string]any{"type": "string"},
				"bond_distance": map[string]any{"type": "number"},
			},
			"required": []any{"molecule"},
		},
		Capability: "anchor:read",
		ReadOnly:   true,
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var a struct {
				Molecule     string  `json:"molecule"`
				BondDistance float64 `json:"bond_distance"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			if a.BondDistance <= 0 {
				a.BondDistance = defaultBondDistance(a.Molecule)
			}
			b := bridge()
			if b == nil {
				return nil, fmt.Errorf("bridge unavailable: no anchor computation performed")
			}
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			resp, err := b.ComputeClassicalAnchor(cctx, &pb.ClassicalAnchorRequest{
				Molecule:     strings.ToUpper(a.Molecule),
				BondDistance: a.BondDistance,
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"molecule":      a.Molecule,
				"bond_distance": a.BondDistance,
				"rhf":           resp.RhfEnergy,
				"fci":           resp.FciExactEnergy,
				"success":       resp.Success,
			}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "get_campaign",
		Description: "Read the current campaign state (target, energies, backend, spend) for a session.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"session_id": map[string]any{"type": "string"}},
		},
		Capability: "campaign:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct {
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			raw, err := s.ledger.GetCampaignState(a.SessionID)
			if err != nil || len(raw) == 0 {
				return map[string]any{"session_id": a.SessionID, "campaign": nil}, nil
			}
			var cs agents.CampaignState
			if err := json.Unmarshal(raw, &cs); err != nil {
				return nil, fmt.Errorf("stored campaign unreadable: %w", err)
			}
			return map[string]any{
				"session_id":      cs.SessionID,
				"target":          cs.TargetSystem,
				"backend":         cs.QuantumFactors.RecommendedBackend,
				"qubits":          cs.QuantumFactors.QubitCount,
				"vqe_energy":      cs.ClassicalAnchor.VqeExpectationEnergy,
				"fci_energy":      cs.ClassicalAnchor.FciExactEnergy,
				"discrepancy_mha": cs.ClassicalAnchor.DiscrepancyMHa,
			}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "list_jobs",
		Description: "List recent jobs and their statuses for a session.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"session_id": map[string]any{"type": "string"}},
		},
		Capability: "jobs:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct {
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			list, err := s.jobRunner.ListJobs(a.SessionID, 5)
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(list))
			for _, j := range list {
				out = append(out, map[string]any{
					"id":      j.ID,
					"status":  jobStatusName(j),
					"type":    j.JobType,
					"backend": jobBackend(j),
				})
			}
			return map[string]any{"jobs": out}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "get_job",
		Description: "Fetch one job with full params and result payloads by id.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"job_id": map[string]any{"type": "string"}},
			"required":   []any{"job_id"},
		},
		Capability: "jobs:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			j, err := s.jobRunner.GetJob(a.JobID)
			if err != nil {
				return nil, fmt.Errorf("job not found")
			}
			return map[string]any{
				"id":      j.ID,
				"status":  jobStatusName(j),
				"type":    j.JobType,
				"backend": jobBackend(j),
				"params":  string(j.ParamsJSON),
				"result":  string(j.ResultJSON),
			}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "recent_events",
		Description: "Read the latest ledger event types for a session (activity tail).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"limit":      map[string]any{"type": "integer"},
			},
		},
		Capability: "ledger:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct {
				SessionID string `json:"session_id"`
				Limit     int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			if a.Limit <= 0 || a.Limit > 25 {
				a.Limit = 10
			}
			events, err := s.ledger.GetRecentEvents(a.SessionID, a.Limit)
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(events))
			for _, e := range events {
				out = append(out, map[string]any{"type": e.EventType, "id": e.ID})
			}
			return map[string]any{"events": out}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "query_literature",
		Description: "Search prior art (arXiv/OpenAlex via the bridge). Returns records with sources and years.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
			"required": []any{"query"},
		},
		Capability: "literature:read",
		ReadOnly:   true,
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var a struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			if a.Limit <= 0 || a.Limit > 10 {
				a.Limit = 5
			}
			b := bridge()
			if b == nil {
				return nil, fmt.Errorf("bridge unavailable: no literature grounding performed")
			}
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			resp, err := b.GroundLiterature(cctx, &pb.LiteratureGroundingRequest{Query: a.Query, Limit: int32(a.Limit)})
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(resp.Records))
			for _, rec := range resp.Records {
				out = append(out, map[string]any{
					"title": rec.Title, "url": rec.Url,
					"year": rec.PublicationYear, "citations": rec.CitationCount,
				})
			}
			return map[string]any{"records": out, "cached": resp.Cached}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "submit_local_sim",
		Description: "Dispatch a circuit to the LOCAL simulator only ($0.00, L1 auto-allow rail). Hardware tiers refuse here — those go through proposals and the approval gate. At most one dispatch per turn; further calls in the same turn refuse.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"qasm":       map[string]any{"type": "string"},
				"shots":      map[string]any{"type": "integer"},
			},
			"required": []any{"session_id", "qasm"},
		},
		Capability: "jobs:submit-local",
		ReadOnly:   false,
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var a struct {
				SessionID string `json:"session_id"`
				QASM      string `json:"qasm"`
				Shots     int    `json:"shots"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			if strings.TrimSpace(a.QASM) == "" {
				return nil, fmt.Errorf("qasm must not be empty")
			}
			if a.Shots <= 0 {
				a.Shots = 1024
			}
			if a.Shots > 4096 {
				return nil, fmt.Errorf("shots capped at 4096 for tool dispatch (got %d)", a.Shots)
			}
			job, err := s.jobRunner.SubmitJob(a.SessionID, "circuit_run", map[string]any{
				"qasm":              a.QASM,
				"requested_backend": "local_simulator",
				"shots":             a.Shots,
				"max_cost_usd":      0.0,
				"requested_by":      "copilot-tool",
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"job_id":  job.ID,
				"status":  jobStatusName(job),
				"backend": "local_simulator",
			}, nil
		},
	})

	_ = r.Register(tools.Definition{
		Name:        "recall_memory",
		Description: "Recall prior lab experiences relevant to a query (tenant-scoped associative memory). Returns snippets with similarity scores, newest-relevant first.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"query":      map[string]any{"type": "string"},
			},
			"required": []any{"session_id", "query"},
		},
		Capability: "memory:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct {
				SessionID string `json:"session_id"`
				Query     string `json:"query"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			tenant := "default"
			if s.sessionMgr != nil {
				if sess, err := s.sessionMgr.GetSession(a.SessionID); err == nil && sess.TenantID != "" {
					tenant = sess.TenantID
				}
			}
			if s.memoryStore == nil {
				return map[string]any{"snippets": []any{}, "tenant": tenant}, nil
			}
			recallCfg := llm.DefaultBudgetConfig()
			recalled := s.memoryStore.SemanticRecallScoped(
				memory.BankExperiences, a.Query,
				recallCfg.RecallTokens, recallCfg.RecallMinSimilarity,
				tenant, []string{tenant})
			out := make([]any, 0, len(recalled))
			for _, item := range recalled {
				out = append(out, map[string]any{
					"similarity": item.Similarity,
					"content":    item.Item.Content,
				})
			}
			return map[string]any{"snippets": out, "tenant": tenant}, nil
		},
	})

	_ = r.Allow(CopilotRole,
		"get_anchor", "get_campaign", "list_jobs", "get_job",
		"recent_events", "query_literature", "submit_local_sim",
		"recall_memory",
	)
	// Tribunal specialist roles observe through the same read-only set.
	// Turns never write: submit_local_sim is copilot-only.
	for _, role := range []string{
		"ansatz-scout", "disconfirmation-officer", "device-auditor", "epistemic-scribe",
	} {
		_ = r.Allow(role,
			"get_anchor", "get_campaign", "list_jobs", "get_job",
			"recent_events", "query_literature", "recall_memory",
		)
	}
	return r
}

func defaultBondDistance(molecule string) float64 {
	switch strings.ToUpper(strings.TrimSpace(molecule)) {
	case "LIH", "LITHIUM":
		return 1.5957
	case "BEH2", "BERYLLIUM":
		return 1.3300
	default:
		return 0.7414
	}
}

func jobStatusName(j *jobs.Job) string {
	if j == nil {
		return "UNKNOWN"
	}
	return strings.TrimPrefix(j.Status.String(), "JOB_STATUS_")
}

func jobBackend(j *jobs.Job) string {
	if j == nil || len(j.ParamsJSON) == 0 {
		return "—"
	}
	var p map[string]any
	if err := json.Unmarshal(j.ParamsJSON, &p); err != nil {
		return "—"
	}
	if b, ok := p["requested_backend"].(string); ok && b != "" {
		return b
	}
	return "—"
}

// resolveWithTools runs the plan→tool→observe loop over a base history and
// returns the augmented history for the final answer call. Tool rounds use
// non-streamed completions; the visible answer still streams afterwards.
// Any failure (no tools, provider error, dispatch refusal) returns the base
// history unchanged, so the caller degrades to exactly today's behavior.
func (s *Server) resolveWithTools(ctx context.Context, sessionID string, base []llm.ChatMessage) []llm.ChatMessage {
	if s.toolRegistry == nil || s.llmClient == nil {
		return base
	}
	toolsForLLM := s.toolRegistry.DefinitionsForLLM(CopilotRole)
	if len(toolsForLLM) == 0 {
		return base
	}
	hist := append([]llm.ChatMessage(nil), base...)
	hist = append(hist, llm.ChatMessage{Role: "system", Content: toolContextPreamble(sessionID)})
	cfg := llm.DefaultBudgetConfig()
	submits := 0
	var toolChars, turnTokens int
	var lastFailTool string
	var failStreak int
loop:
	for round := 0; round < cfg.MaxToolRounds; round++ {
		text, calls, chatResp, _, err := s.llmClient.CompleteWithTools(ctx, CopilotRole, hist, toolsForLLM)
		turnTokens += usageTokens(chatResp)
		if turnTokens > cfg.MaxTurnTokens {
			// Token guardrail: provider-reported usage past the turn cap
			// stops the loop instead of burning minutes. Recorded as a
			// cap event, never as a tool failure (nothing failed).
			_ = s.ledger.AppendEvent(sessionID, "tool_loop_capped", map[string]any{
				"reason": "token_cap", "rounds": round + 1, "turn_tokens": turnTokens,
			})
			break
		}
		if err != nil {
			break
		}
		if len(calls) == 0 {
			// DSML fallback: some model/gateway pairs emit invocations as
			// markup in text instead of structured tool_calls. Same
			// validated dispatch, same observation loop — never raw text.
			calls = llm.ParseDSMLInvokes(text)
			if len(calls) == 0 {
				break
			}
		}
		hist = append(hist, llm.ChatMessage{Role: "assistant", Content: "", ToolCalls: calls})
		for _, c := range calls {
			var out any
			var derr error
			// Per-turn write bound: at most one simulator dispatch per
			// turn. Further submit calls refuse with the bound stated —
			// the model can propose, never spam the queue.
			if c.Name == "submit_local_sim" {
				if submits >= 1 {
					derr = fmt.Errorf("per-turn dispatch bound reached (1): further submits refused; batch circuits into one proposal instead")
				} else {
					submits++
				}
			}
			if derr == nil {
				out, derr = s.toolRegistry.Dispatch(ctx, CopilotRole, toRegistryCall(c))
			}
			// Audit every invocation: a tool call that is not also a
			// ledger event is unverifiable afterwards (was it executed or
			// confabulated?). Arguments stay out of the event; the name,
			// outcome, and error text are what the audit needs.
			_ = s.ledger.AppendEvent(sessionID, "tool_called", map[string]any{
				"tool":  c.Name,
				"ok":    derr == nil,
				"error": errorText(derr),
			})
			obs, _ := json.Marshal(map[string]any{
				"tool":  c.Name,
				"ok":    derr == nil,
				"error": errorText(derr),
				"data":  out,
			})
			// Observation budget: one chatty tool (a full campaign dump)
			// must not eat the turn. The cut is marked on the wire.
			obsCapped := llm.TruncateObservation(string(obs), cfg.MaxObservationChars)
			toolChars += len(obsCapped)
			hist = append(hist, llm.ChatMessage{
				Role: "tool", ToolCallID: c.ID, ToolName: c.Name,
				Content: obsCapped,
			})
			// Repeat-failure breaker: the same tool failing twice in a row
			// will fail a third time — stop the loop and let the model
			// answer with partial observations instead of burning rounds.
			if derr == nil {
				failStreak, lastFailTool = 0, ""
			} else {
				if c.Name == lastFailTool {
					failStreak++
				} else {
					failStreak, lastFailTool = 1, c.Name
				}
				if failStreak >= 2 {
					_ = s.ledger.AppendEvent(sessionID, "tool_loop_breaker", map[string]any{
						"tool": c.Name, "consecutive_failures": failStreak,
					})
					break loop
				}
			}
			if toolChars > cfg.MaxToolCharsPerTurn {
				_ = s.ledger.AppendEvent(sessionID, "tool_loop_capped", map[string]any{
					"reason": "tool_chars", "rounds": round + 1, "tool_chars": toolChars,
				})
				break loop
			}
		}
	}
	return hist
}

// usageTokens sums provider-reported usage for the accumulator. Zero when
// unreported (local providers often omit it) — the accumulator then stays
// quiet rather than guessing.
func usageTokens(resp *llm.ChatCompletionResponse) int {
	if resp == nil {
		return 0
	}
	if resp.Usage.TotalTokens > 0 {
		return resp.Usage.TotalTokens
	}
	return resp.Usage.PromptTokens + resp.Usage.CompletionTokens
}

func toolContextPreamble(sessionID string) string {
	return "Live session state (session " + sessionID + "): you have read-only tools " +
		"(get_anchor, get_campaign, list_jobs, get_job, recent_events, query_literature). " +
		"Call get_anchor before citing any energy. Call get_campaign/list_jobs before " +
		"describing campaign or job state. Never invent energies, fidelities, or job " +
		"statuses — observe them. The only write tool is submit_local_sim " +
		"(local simulator, admission-enforced, one dispatch per turn); hardware " +
		"dispatch and mandates are not tools — route those through proposals " +
		"and the approval gate as before."
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func toRegistryCall(c llm.ToolCall) tools.Call {
	return tools.Call{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
}
