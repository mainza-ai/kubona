package verify

// Numeric-claim verification (AG-5).
//
// A manuscript's prose is scanned for numbers that look like measured results
// (unit-bearing values, percentages, and decimals with at least three
// fractional digits). Each is compared with numbers actually recorded in the
// ledger — job results, claim evidence, and the campaign snapshot — within the
// rounding tolerance implied by how the number was written.
//
// This is deliberately conservative in one direction: it can only say
// "matched a recorded value" or "no recorded value matches". It never declares
// a number physically correct. A number that is not recorded anywhere is
// flagged, because publication must not present an unrecorded measurement.

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Document is one JSON evidence source (a job result, a claim's evidence, a
// campaign snapshot). The ID is surfaced on a match so a reviewer can follow it.
type Document struct {
	ID   string
	JSON string
}

// Evidence is the recorded state a manuscript is checked against.
type Evidence struct {
	Documents []Document
}

// EvidenceValue is one numeric leaf with its path and source.
type EvidenceValue struct {
	Path   string
	Value  float64
	Source string
}

// NumericFinding is one number found in the manuscript.
type NumericFinding struct {
	Text         string   `json:"text"`
	Value        float64  `json:"value"`
	Unit         string   `json:"unit,omitempty"`
	Status       string   `json:"status"`
	MatchedPath  string   `json:"matched_path,omitempty"`
	MatchedValue *float64 `json:"matched_value,omitempty"`
	SourceID     string   `json:"source_id,omitempty"`
	Tolerance    float64  `json:"tolerance"`
}

const (
	NumberMatched      = "matched"
	NumberUnmatched    = "unmatched"
	NumberUnverifiable = "unverifiable"
)

var unitNumber = regexp.MustCompile(`(?i)([-+]?\d+(?:\.\d+)?)\s*(mha|kcal/mol|angstroms?|seconds?|milliseconds?|qubits?|shots?|ha|ev|ms|å|%)`)

// The sign must be captured with the digits: a leading \b before an optional
// sign would match "1.13" inside "-1.13" and silently drop the minus.
var bareDecimal = regexp.MustCompile(`[-+]?\b\d+\.\d{3,}\b`)

// ExtractedNumber is one candidate measurement in text.
type ExtractedNumber struct {
	Raw      string
	Value    float64
	Unit     string
	Decimals int
	Start    int
	End      int
}

// ExtractNumbers finds measurement-like numbers in text. Unit-bearing values
// and decimals with at least three fractional digits qualify; years, small
// enumerations, and reference markers do not.
func ExtractNumbers(text string) []ExtractedNumber {
	out := []ExtractedNumber{}
	seen := [][2]int{}
	for _, m := range unitNumber.FindAllStringSubmatchIndex(text, -1) {
		raw := text[m[2]:m[3]]
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		unit := strings.ToLower(strings.TrimSpace(text[m[4]:m[5]]))
		out = append(out, ExtractedNumber{
			Raw: raw, Value: v, Unit: unit, Decimals: decimals(raw),
			Start: m[0], End: m[1],
		})
		seen = append(seen, [2]int{m[0], m[1]})
	}
	for _, m := range bareDecimal.FindAllStringIndex(text, -1) {
		if overlapsAny(m, seen) {
			continue
		}
		raw := text[m[0]:m[1]]
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		// A bare decimal still needs a non-trivial fractional part to be a
		// measurement claim; the regex already enforces three digits.
		out = append(out, ExtractedNumber{
			Raw: raw, Value: v, Decimals: decimals(raw), Start: m[0], End: m[1],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// VerifyNumbers checks every extracted number against the recorded evidence.
func VerifyNumbers(text string, ev Evidence) []NumericFinding {
	extracted := ExtractNumbers(text)
	if len(extracted) == 0 {
		return []NumericFinding{}
	}
	values := ev.Values()
	findings := make([]NumericFinding, 0, len(extracted))
	for _, n := range extracted {
		f := NumericFinding{
			Text: n.Raw, Value: n.Value, Unit: n.Unit,
			Tolerance: toleranceFor(n.Decimals),
		}
		if len(values) == 0 {
			f.Status = NumberUnverifiable
			findings = append(findings, f)
			continue
		}
		best, bestDiff := EvidenceValue{}, math.Inf(1)
		for _, v := range values {
			diff := math.Abs(v.Value - n.Value)
			if diff < bestDiff {
				best, bestDiff = v, diff
			}
		}
		limit := f.Tolerance + 1e-9*math.Abs(best.Value)
		if bestDiff <= limit {
			f.Status = NumberMatched
			f.MatchedPath, f.SourceID = best.Path, best.Source
			matched := best.Value
			f.MatchedValue = &matched
		} else {
			f.Status = NumberUnmatched
		}
		findings = append(findings, f)
	}
	return findings
}

// Values flattens every document into numeric leaves. Arrays use [i] paths so
// a reviewer can locate the value; the traversal is bounded and deterministic.
func (e Evidence) Values() []EvidenceValue {
	out := []EvidenceValue{}
	for _, doc := range e.Documents {
		if strings.TrimSpace(doc.JSON) == "" {
			continue
		}
		var v any
		dec := json.NewDecoder(strings.NewReader(doc.JSON))
		if err := dec.Decode(&v); err != nil {
			continue
		}
		flatten(doc.ID, "", v, 0, &out)
		if len(out) > 5000 {
			out = out[:5000]
			break
		}
	}
	return out
}

func flatten(source, prefix string, v any, depth int, out *[]EvidenceValue) {
	if depth > 12 || len(*out) > 5000 {
		return
	}
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			flatten(source, path, val[k], depth+1, out)
		}
	case []any:
		for i, item := range val {
			flatten(source, prefix+"["+strconv.Itoa(i)+"]", item, depth+1, out)
		}
	case float64:
		*out = append(*out, EvidenceValue{Path: prefix, Value: val, Source: source})
	case json.Number:
		if f, err := val.Float64(); err == nil {
			*out = append(*out, EvidenceValue{Path: prefix, Value: f, Source: source})
		}
	}
}

func toleranceFor(decimals int) float64 {
	if decimals <= 0 {
		return 0.5
	}
	return 0.5 * math.Pow(10, float64(-decimals))
}

func decimals(raw string) int {
	if i := strings.Index(raw, "."); i >= 0 {
		return len(raw) - i - 1
	}
	return 0
}

func overlapsAny(span []int, seen [][2]int) bool {
	for _, s := range seen {
		if span[0] < s[1] && s[0] < span[1] {
			return true
		}
	}
	return false
}
