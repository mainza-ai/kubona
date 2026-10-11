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
	"github.com/mainza-ai/kubona/core/pkg/tools"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// CopilotRole is the caller role for chat tool calls. Deny by default: the
// registry grants this role an explicit read-only set at registration.
const CopilotRole = "copilot"

// maxToolRounds bounds model→tool→observation cycles per turn. Enough for
// anchor-then-answer; the loop always terminates with a plain answer.
const maxToolRounds = 3

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

	_ = r.Allow(CopilotRole,
		"get_anchor", "get_campaign", "list_jobs", "get_job",
		"recent_events", "query_literature",
	)
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
	for round := 0; round < maxToolRounds; round++ {
		text, calls, _, _, err := s.llmClient.CompleteWithTools(ctx, CopilotRole, hist, toolsForLLM)
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
			out, derr := s.toolRegistry.Dispatch(ctx, CopilotRole, toRegistryCall(c))
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
			hist = append(hist, llm.ChatMessage{
				Role: "tool", ToolCallID: c.ID, ToolName: c.Name,
				Content: string(obs),
			})
		}
	}
	return hist
}

func toolContextPreamble(sessionID string) string {
	return "Live session state (session " + sessionID + "): you have read-only tools " +
		"(get_anchor, get_campaign, list_jobs, get_job, recent_events, query_literature). " +
		"Call get_anchor before citing any energy. Call get_campaign/list_jobs before " +
		"describing campaign or job state. Never invent energies, fidelities, or job " +
		"statuses — observe them. Writes (dispatch, mandates) are not tools; route " +
		"those through proposals and the approval gate as before."
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
