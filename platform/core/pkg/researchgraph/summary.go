package researchgraph

// Context-budgeted research-graph summaries (AG-4b).
//
// An agent resuming after a restart cannot be handed the whole projection: it
// needs the objective, what is still open, what is running, what was decided,
// and what failed — in that order, within a byte budget. This package renders
// that view deterministically from the graph. It never invents a status: every
// line comes from a ledger row, and anything that does not fit is reported as
// omitted instead of silently dropped.
//
// The graph itself is a ledger projection (AG-4a), so restart resume is
// reconstruction plus a cursor — no second store to keep warm.

import (
	"fmt"
	"sort"
	"strings"
)

// Hypothesis is the summary view of one hypothesis.
type Hypothesis struct {
	ID        string
	Statement string
	Rationale string
	Falsifier string
	Status    string
	Source    string
	Score     float64
	ClaimID   string
}

// Plan is the summary view of one pre-registered experiment.
type Plan struct {
	ID               string
	HypothesisID     string
	Statement        string
	Status           string
	Observable       string
	AcceptanceMetric string
	Accepted         *bool
}

// Run is the summary view of one job.
type Run struct {
	ID        string
	Type      string
	Status    string
	CreatedAt string
}

// Claim is the summary view of one claim record.
type Claim struct {
	ID        string
	Statement string
	Verdict   string
}

// Graph is the minimal projection the summarizer needs.
type Graph struct {
	SessionID       string
	Goal            string
	Target          string
	DeltaMHa        *float64
	Claimable       *bool
	Hypotheses      []Hypothesis
	Plans           []Plan
	Runs            []Run
	Claims          []Claim
	ManuscriptCount int
}

// Report states what the budget allowed and what it did not.
type Report struct {
	BudgetBytes int      `json:"budget_bytes"`
	Bytes       int      `json:"bytes"`
	Included    []string `json:"included"`
	Omitted     []string `json:"omitted"`
	Truncated   bool     `json:"truncated"`
}

const (
	defaultBudget = 4096
	minBudget     = 512
	maxBudget     = 65536
)

// Summarize renders the priority-ordered summary. The returned string never
// exceeds the effective budget.
func Summarize(g Graph, budgetBytes int) (string, Report) {
	budget := budgetBytes
	if budget <= 0 {
		budget = defaultBudget
	}
	if budget < minBudget {
		budget = minBudget
	}
	if budget > maxBudget {
		budget = maxBudget
	}
	rep := Report{BudgetBytes: budget, Included: []string{}, Omitted: []string{}}

	sections := []struct {
		name string
		text string
	}{
		{"objective", objectiveSection(g)},
		{"hypotheses", hypothesesSection(g.Hypotheses)},
		{"plans", plansSection(g.Plans)},
		{"runs", runsSection(g.Runs)},
		{"claims", claimsSection(g.Claims)},
		{"negatives", negativesSection(g.Hypotheses, g.Plans)},
		{"manuscripts", manuscriptsSection(g.ManuscriptCount)},
	}
	var out strings.Builder
	for _, s := range sections {
		if s.text == "" {
			continue
		}
		remaining := budget - out.Len()
		if remaining <= 0 {
			rep.Omitted = append(rep.Omitted, s.name)
			rep.Truncated = true
			continue
		}
		if len(s.text) <= remaining {
			out.WriteString(s.text)
			rep.Included = append(rep.Included, s.name)
			continue
		}
		// A section that does not fit whole is cut at a line boundary: a
		// partial view is more useful to a resuming agent than a silent gap,
		// and the report records the truncation.
		if prefix := fitPrefix(s.text, remaining); prefix != "" {
			out.WriteString(prefix)
			rep.Included = append(rep.Included, s.name)
			rep.Truncated = true
		} else {
			rep.Omitted = append(rep.Omitted, s.name)
			rep.Truncated = true
		}
	}
	text := out.String()
	if len(text) > budget {
		text = text[:budget]
		rep.Truncated = true
	}
	rep.Bytes = len(text)
	return text, rep
}

func objectiveSection(g Graph) string {
	var b strings.Builder
	b.WriteString("# Research graph — " + fallback(g.SessionID, "unknown session") + "\n")
	b.WriteString("- goal: " + fallback(strings.TrimSpace(g.Goal), "not stated") + "\n")
	b.WriteString("- target: " + fallback(strings.TrimSpace(g.Target), "not stated") + "\n")
	if g.DeltaMHa != nil {
		fmt.Fprintf(&b, "- measured discrepancy: %.3f mHa\n", *g.DeltaMHa)
	} else {
		b.WriteString("- measured discrepancy: none recorded\n")
	}
	if g.Claimable != nil {
		fmt.Fprintf(&b, "- claimable: %t\n", *g.Claimable)
	}
	return b.String()
}

func hypothesesSection(hs []Hypothesis) string {
	open := make([]Hypothesis, 0, len(hs))
	for _, h := range hs {
		if h.Status == "rejected" || h.Status == "collapsed" {
			continue
		}
		open = append(open, h)
	}
	if len(open) == 0 {
		return ""
	}
	sort.SliceStable(open, func(i, j int) bool {
		if open[i].Score == open[j].Score {
			return open[i].ID < open[j].ID
		}
		return open[i].Score > open[j].Score
	})
	var b strings.Builder
	fmt.Fprintf(&b, "## Open hypotheses (%d)\n", len(open))
	for i, h := range open {
		if i >= 5 {
			fmt.Fprintf(&b, "- … %d more\n", len(open)-5)
			break
		}
		fmt.Fprintf(&b, "- [%.2f] %s (%s)\n", h.Score, clip(h.Statement, 160), h.Status)
		if h.Falsifier != "" {
			b.WriteString("  falsified if: " + clip(h.Falsifier, 140) + "\n")
		}
	}
	return b.String()
}

func plansSection(ps []Plan) string {
	if len(ps) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Plans (%d)\n", len(ps))
	for i, p := range ps {
		if i >= 5 {
			fmt.Fprintf(&b, "- … %d more\n", len(ps)-5)
			break
		}
		verdict := ""
		if p.Accepted != nil {
			if *p.Accepted {
				verdict = " accepted"
			} else {
				verdict = " not accepted"
			}
		}
		fmt.Fprintf(&b, "- %s [%s] %s — %s vs %s%s\n", p.ID, p.Status,
			clip(p.Statement, 120), p.Observable, p.AcceptanceMetric, verdict)
	}
	return b.String()
}

func runsSection(rs []Run) string {
	if len(rs) == 0 {
		return ""
	}
	sorted := append([]Run(nil), rs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt > sorted[j].CreatedAt })
	var b strings.Builder
	fmt.Fprintf(&b, "## Recent runs (%d)\n", len(rs))
	for i, r := range sorted {
		if i >= 5 {
			fmt.Fprintf(&b, "- … %d more\n", len(rs)-5)
			break
		}
		fmt.Fprintf(&b, "- %s %s %s\n", r.ID, r.Type, strings.TrimPrefix(r.Status, "JOB_STATUS_"))
	}
	return b.String()
}

func claimsSection(cs []Claim) string {
	if len(cs) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, c := range cs {
		counts[fallback(c.Verdict, "unknown")]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "## Claims (%d)\n", len(cs))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	b.WriteString("- " + strings.Join(parts, " ") + "\n")
	for i, c := range cs {
		if i >= 3 {
			break
		}
		if c.Verdict == "verified" {
			fmt.Fprintf(&b, "- %s verified: %s\n", c.ID, clip(c.Statement, 120))
		}
	}
	return b.String()
}

func negativesSection(hs []Hypothesis, ps []Plan) string {
	rejected := 0
	lines := []string{}
	for _, h := range hs {
		if h.Status == "rejected" {
			rejected++
			if len(lines) < 2 {
				lines = append(lines, "- hypothesis rejected: "+clip(h.Statement, 120))
			}
		}
	}
	failed := 0
	for _, p := range ps {
		if p.Status == "failed" || (p.Accepted != nil && !*p.Accepted) {
			failed++
			if len(lines) < 4 {
				lines = append(lines, "- plan not accepted: "+p.ID+" "+clip(p.Statement, 100))
			}
		}
	}
	if rejected == 0 && failed == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Negatives (rejected hypotheses=%d, unaccepted plans=%d)\n", rejected, failed)
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	return b.String()
}

func manuscriptsSection(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("## Manuscripts\n- %d in this session\n", n)
}

// fitPrefix returns the longest whole-line prefix of text within limit. It
// returns "" when even the first line does not fit.
func fitPrefix(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	cut := text[:limit]
	idx := strings.LastIndex(cut, "\n")
	if idx < 0 {
		return ""
	}
	return cut[:idx+1]
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func fallback(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
