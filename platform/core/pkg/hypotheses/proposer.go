package hypotheses

// LLM proposer (AG-2b).
//
// The deterministic generator stays the evidence floor: it always produces
// grounded probes. This proposer adds model-authored candidates on request,
// under a strict contract:
//
//   - output must be one JSON object with a "hypotheses" array;
//   - every candidate must carry a statement and a falsifier;
//   - experiment_kind must be one of the known families;
//   - novelty must never be asserted by the model.
//
// Anything that fails validation is rejected wholesale rather than partially
// repaired, so a malformed reply can never smuggle an ungrounded probe into
// the ledger. The caller merges the survivors with the diagnostics and labels
// them source="llm".

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/untrusted"
)

// Completer is the narrow LLM seam. Production passes an adapter over
// llm.Client; tests pass a fake so the proposer contract is exercised without
// a provider.
type Completer interface {
	Complete(ctx context.Context, role string, messages []llm.ChatMessage) (string, error)
}

// NeutralLLMScore is the starting score for a model-proposed candidate. It is
// deliberately not higher than a grounded diagnostic: the model has not yet
// earned rank, and the tournament is where rank is decided.
const NeutralLLMScore = 0.5

// ProposeOptions is the factual context handed to the model. Everything here
// comes from the measured campaign; the proposer must not invent facts beyond
// it.
type ProposeOptions struct {
	Goal       string
	Target     string
	Measured   bool
	DeltaMHa   float64
	Hardware   bool
	Claimable  bool
	Literature []PriorArt
	Count      int
}

// knownExperimentKinds is the closed vocabulary. The first group can be
// planned and executed through the job rail; the second group is diagnostic
// work that informs a plan but is not itself a job type.
var knownExperimentKinds = map[string]bool{
	"vqe": true, "spin_chain": true, "geometry_optimization": true,
	"vqe_repeats": true, "pes": true, "qaoa": true, "excited_state": true,
	"reaction": true, "thermodynamic_limit": true, "conjecture": true,
	"lattice_dynamics": true,
	"mitigate":         true, "calibration": true, "anchor": true,
	"literature": true, "scaling": true,
}

var kindAliases = map[string]string{
	"mitigation":          "mitigate",
	"classical_anchor":    "anchor",
	"prior_art":           "literature",
	"literature_search":   "literature",
	"scaling_test":        "scaling",
	"noise_mitigation":    "mitigate",
	"error_mitigation":    "mitigate",
	"geometry":            "geometry_optimization",
	"repeat":              "vqe_repeats",
	"repeats":             "vqe_repeats",
	"excited_states":      "excited_state",
	"thermodynamic":       "thermodynamic_limit",
	"lattice":             "lattice_dynamics",
	"spin_model":          "spin_chain",
	"classical_benchmark": "anchor",
}

// NormalizeKind lower-cases and maps the accepted aliases onto the closed
// vocabulary. Unknown kinds are returned unchanged so validation can reject
// them explicitly.
func NormalizeKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	if alias, ok := kindAliases[k]; ok {
		return alias
	}
	return k
}

// NormalizeKindExecutable reports whether the kind maps to a dispatchable job
// type. The API's experimentJobType is the authority at plan time; this list
// mirrors it for proposer validation and must be changed together.
func NormalizeKindExecutable(kind string) bool {
	switch NormalizeKind(kind) {
	case "vqe", "spin_chain", "geometry_optimization", "vqe_repeats", "pes",
		"qaoa", "excited_state", "reaction", "thermodynamic_limit",
		"conjecture", "lattice_dynamics":
		return true
	}
	return false
}

type proposalEnvelope struct {
	Hypotheses []proposal `json:"hypotheses"`
}

type proposal struct {
	Statement      string `json:"statement"`
	Rationale      string `json:"rationale"`
	Falsifier      string `json:"falsifier"`
	ExperimentKind string `json:"experiment_kind"`
	ExperimentHint string `json:"experiment_hint"`
}

// Propose asks the completer for candidates and returns only those that pass
// the contract. A transport or parse failure is returned to the caller, which
// falls back to diagnostics and surfaces the error honestly.
func Propose(ctx context.Context, c Completer, opts ProposeOptions) ([]Candidate, error) {
	if c == nil {
		return nil, fmt.Errorf("no LLM completer configured")
	}
	count := opts.Count
	if count <= 0 || count > 25 {
		count = 12
	}
	system := `You propose falsifiable scientific probes for a quantum-chemistry research platform.
Reply with ONE JSON object and nothing else. Contract:
{"hypotheses":[{"statement":"...","rationale":"...","falsifier":"...","experiment_kind":"vqe","experiment_hint":"..."}]}
Rules:
- A hypothesis is a probe, never a claim. Do not say "we proved", "we discovered", or "this demonstrates".
- "statement" names the mechanism or explanation under test.
- "falsifier" names an observable outcome that would refute it. It must be checkable, not a restatement.
- "experiment_kind" is one of: vqe, spin_chain, geometry_optimization, vqe_repeats, pes, qaoa, excited_state, reaction, thermodynamic_limit, conjecture, lattice_dynamics, mitigate, calibration, anchor, literature, scaling.
- "experiment_hint" is a concrete next run for the same platform.
- Never assert novelty, citations, or measured values. Only use the facts supplied.
- Return between 1 and ` + fmt.Sprint(count) + ` items.
` + untrusted.SystemRule

	user := buildProposalPrompt(opts, count)
	reply, err := c.Complete(ctx, "copilot", []llm.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM proposal failed: %w", err)
	}
	return parseProposals(reply, count)
}

func buildProposalPrompt(opts ProposeOptions, count int) string {
	var b strings.Builder
	b.WriteString("Research context:\n")
	b.WriteString("- goal: " + fallback(strings.TrimSpace(opts.Goal), "not stated") + "\n")
	b.WriteString("- target system: " + fallback(strings.TrimSpace(opts.Target), "not stated") + "\n")
	if opts.Measured {
		fmt.Fprintf(&b, "- measured discrepancy vs the classical anchor: %.3f mHa\n", opts.DeltaMHa)
	} else {
		b.WriteString("- no measured campaign point exists yet\n")
	}
	fmt.Fprintf(&b, "- hardware run: %t; claimable: %t\n", opts.Hardware, opts.Claimable)
	if len(opts.Literature) > 0 {
		b.WriteString("- retrieved prior art (titles only; do not invent citations):\n")
		var litBuf strings.Builder
		for i, ref := range opts.Literature {
			if i >= 8 {
				break
			}
			title := strings.TrimSpace(ref.Title)
			if title == "" {
				title = ref.ID
			}
			litBuf.WriteString("- " + truncate(title, 180) + "\n")
		}
		b.WriteString(untrusted.Wrap("literature", litBuf.String()) + "\n")
	} else {
		b.WriteString("- no prior art was retrieved; do not assert novelty\n")
	}
	fmt.Fprintf(&b, "\nReturn at most %d hypotheses as the contract JSON.\n", count)
	return b.String()
}

// parseProposals extracts and validates the contract. It rejects the whole
// reply when anything is structurally wrong: partial acceptance would let a
// malformed candidate hide among valid ones.
func parseProposals(reply string, want int) ([]Candidate, error) {
	body, err := extractJSONObject(reply)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	var env proposalEnvelope
	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("LLM proposal is not the contract JSON: %w (reply %d bytes)", err, len(body))
	}
	if len(env.Hypotheses) == 0 {
		return nil, fmt.Errorf("LLM proposal returned no hypotheses")
	}
	out := make([]Candidate, 0, len(env.Hypotheses))
	for i, p := range env.Hypotheses {
		if want > 0 && len(out) >= want {
			break
		}
		statement := strings.TrimSpace(p.Statement)
		falsifier := strings.TrimSpace(p.Falsifier)
		if statement == "" || falsifier == "" {
			return nil, fmt.Errorf("LLM hypothesis %d is missing a statement or falsifier", i)
		}
		if len(statement) > 400 || len(falsifier) > 400 {
			return nil, fmt.Errorf("LLM hypothesis %d exceeds 400 characters", i)
		}
		kind := NormalizeKind(p.ExperimentKind)
		if !knownExperimentKinds[kind] {
			return nil, fmt.Errorf("LLM hypothesis %d has unknown experiment_kind %q", i, p.ExperimentKind)
		}
		out = append(out, Candidate{
			Statement:      statement,
			Rationale:      truncate(strings.TrimSpace(p.Rationale), 800),
			Falsifier:      falsifier,
			ExperimentKind: kind,
			ExperimentHint: truncate(strings.TrimSpace(p.ExperimentHint), 300),
			Literature:     []string{},
			Score:          NeutralLLMScore,
			Source:         "llm",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("LLM proposal produced no usable candidates")
	}
	return out, nil
}

// extractJSONObject accepts a bare object or one wrapped in a markdown fence.
// It never "repairs" arbitrary prose: the first and last braces must delimit
// the object, and the decoder still has to accept it.
func extractJSONObject(reply string) (string, error) {
	s := strings.TrimSpace(reply)
	// Strip reasoning model <think>...</think> preamble before locating JSON braces.
	for {
		low := strings.ToLower(s)
		openIdx := strings.Index(low, "<think>")
		if openIdx < 0 {
			break
		}
		closeIdx := strings.Index(low[openIdx:], "</think>")
		if closeIdx < 0 {
			s = s[:openIdx]
			break
		}
		s = s[:openIdx] + s[openIdx+closeIdx+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return "", fmt.Errorf("LLM proposal contains no JSON object")
	}
	return s[start : end+1], nil
}

// Merge combines grounded diagnostics with model proposals: sorted by score,
// de-duplicated by statement similarity, capped at max. The higher-scoring
// version of a near-duplicate survives, so a model rewording of a grounded
// probe cannot outrank or duplicate it.
func Merge(primary, secondary []Candidate, max int) []Candidate {
	all := make([]Candidate, 0, len(primary)+len(secondary))
	all = append(all, primary...)
	all = append(all, secondary...)
	sortCandidates(all)
	out := dedupe(all)
	if max <= 0 || len(out) <= max {
		return out
	}
	// Model proposals are for diversity, not only rank: reserve up to half the
	// slots for the best secondary (LLM) candidates that survived dedupe, then
	// fill the rest by score. Without this, a diagnostic set that fills `max`
	// silently drops every model proposal.
	reserved := max / 2
	if reserved < 1 {
		reserved = 1
	}
	chosen := make([]Candidate, 0, max)
	chosenStatements := map[string]bool{}
	for _, c := range out {
		if len(chosen) >= reserved {
			break
		}
		if c.Source == "llm" {
			chosen = append(chosen, c)
			chosenStatements[c.Statement] = true
		}
	}
	for _, c := range out {
		if len(chosen) >= max {
			break
		}
		if chosenStatements[c.Statement] {
			continue
		}
		chosen = append(chosen, c)
		chosenStatements[c.Statement] = true
	}
	sortCandidates(chosen)
	return chosen
}

func sortCandidates(c []Candidate) {
	// Stable insertion sort keeps the diagnostic candidate first on ties.
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].Score > c[j-1].Score; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

func fallback(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
