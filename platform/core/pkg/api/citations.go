package api

// B5 — citation-checked numeric claims. Deterministic, no LLM judge: every
// significant number in a copilot reply either cites the tool observation it
// came from and matches it within tolerance, or it is reported unverified.

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ToolObservation is one observation the model saw, addressable by [oN].
type ToolObservation struct {
	ID   string          `json:"id"`
	Tool string          `json:"tool"`
	Data json.RawMessage `json:"data"`
}

const (
	citationMaxClaims = 100
	citationLookahead = 12
)

type CitationClaim struct {
	Number   string `json:"number"`
	Citation string `json:"citation,omitempty"`
	Status   string `json:"status"` // verified | uncited | unknown_observation | not_in_observation
}

type CitationReport struct {
	Observations []struct {
		ID   string `json:"id"`
		Tool string `json:"tool"`
	} `json:"observations"`
	Claims     []CitationClaim `json:"claims"`
	Verified   int             `json:"verified"`
	Unverified int             `json:"unverified"`
}

var (
	citationMarkRe = regexp.MustCompile(`\[o(\d{1,6})\]`)
	numberRe       = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	fenceRe        = regexp.MustCompile("(?s)```.*?```")
)

// significantNumber skips prose counts that carry no measurement meaning:
// bare small integers (qubit counts, list indices). Decimals and counts of
// 100+ are checked; a quote of "2" cannot fail verification.
func significantNumber(raw string) bool {
	if strings.Contains(raw, ".") {
		return true
	}
	v, err := strconv.ParseFloat(raw, 64)
	return err == nil && math.Abs(v) >= 100
}

func citationAt(text string, from int) (id string, ok bool) {
	end := from + citationLookahead
	if end > len(text) {
		end = len(text)
	}
	tail := strings.TrimLeft(text[from:end], " \t\n\r")
	m := citationMarkRe.FindStringSubmatch(tail)
	if m == nil {
		return "", false
	}
	return "o" + m[1], true
}

// observationNumbers walks the observation JSON and collects every numeric
// value (including numeric strings) a model could quote.
func observationNumbers(raw json.RawMessage) []float64 {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	var out []float64
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case float64:
			out = append(out, typed)
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				out = append(out, f)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func numberMatches(reply float64, candidates []float64) bool {
	for _, candidate := range candidates {
		tolerance := 1e-6 + 0.001*math.Abs(candidate)
		if math.Abs(reply-candidate) <= tolerance {
			return true
		}
	}
	return false
}

// CheckCitations produces the persisted citation report for one reply.
func CheckCitations(reply string, observations []ToolObservation) CitationReport {
	report := CitationReport{Claims: []CitationClaim{}}
	numbersByID := make(map[string][]float64, len(observations))
	for _, obs := range observations {
		report.Observations = append(report.Observations, struct {
			ID   string `json:"id"`
			Tool string `json:"tool"`
		}{ID: obs.ID, Tool: obs.Tool})
		numbersByID[obs.ID] = observationNumbers(obs.Data)
	}
	// Fenced code is machine text (QASM), not a numeric claim.
	scanned := fenceRe.ReplaceAllString(reply, " ")
	// Mask citation marks so their digits are not read as prose numbers.
	masked := citationMarkRe.ReplaceAllStringFunc(scanned, func(mark string) string {
		return strings.Repeat(" ", len(mark))
	})
	for _, loc := range numberRe.FindAllStringIndex(masked, -1) {
		if len(report.Claims) >= citationMaxClaims {
			break
		}
		raw := masked[loc[0]:loc[1]]
		if !significantNumber(raw) {
			continue
		}
		claim := CitationClaim{Number: raw, Status: "uncited"}
		// The citation must follow the number (the rule the prompt states).
		if id, ok := citationAt(scanned, loc[1]); ok {
			claim.Citation = id
			candidates, known := numbersByID[id]
			switch {
			case !known:
				claim.Status = "unknown_observation"
			case numberMatches(parseNumber(raw), candidates):
				claim.Status = "verified"
			default:
				claim.Status = "not_in_observation"
			}
		}
		if claim.Status == "verified" {
			report.Verified++
		} else {
			report.Unverified++
		}
		report.Claims = append(report.Claims, claim)
	}
	return report
}

func parseNumber(raw string) float64 {
	v, _ := strconv.ParseFloat(raw, 64)
	return v
}

// CitationStreamFilter holds back a trailing incomplete [oN marker so a
// streamed reply never tears a citation in half. The concatenation of all
// Push results plus Flush equals the input exactly.
type CitationStreamFilter struct {
	pending string
}

func (f *CitationStreamFilter) Push(token string) string {
	data := f.pending + token
	f.pending = ""
	idx := strings.LastIndex(data, "[")
	if idx >= 0 && incompleteCitation(data[idx:]) {
		f.pending = data[idx:]
		return data[:idx]
	}
	return data
}

func (f *CitationStreamFilter) Flush() string {
	out := f.pending
	f.pending = ""
	return out
}

func incompleteCitation(tail string) bool {
	if tail == "" || tail[0] != '[' {
		return false
	}
	rest := tail[1:]
	if rest == "" {
		return true
	}
	if rest[0] != 'o' {
		return false
	}
	rest = rest[1:]
	if rest == "" {
		return true
	}
	if len(rest) > 6 {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// citationReport checks one reply and returns the persistable JSON.
func (s *Server) citationReport(reply string, observations []ToolObservation) (CitationReport, *json.RawMessage) {
	report := CheckCitations(reply, observations)
	raw, err := json.Marshal(report)
	if err != nil {
		return report, nil
	}
	msg := json.RawMessage(raw)
	return report, &msg
}

// citationPromptRule is the fixed instruction appended to the copilot prompt.
const citationPromptRule = "When you state a numeric value that came from a tool observation, put its observation id in square brackets immediately after the number (for example -1.13727 [o2]). Never cite an id that was not shown in an observation. A significant number without a citation is treated as unverified by the console."
