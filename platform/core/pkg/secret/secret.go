package secret

// Secret-shape screening, one shared implementation. Used before memory
// retention (B10) and available to the cloud-LLM / ledger edges (S9): a value
// that looks like a credential is never written or echoed verbatim.

import (
	"regexp"
	"sort"
	"strings"
)

type pattern struct {
	kind string
	re   *regexp.Regexp
}

var patterns = []pattern{
	{"private_key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIV` + `ATE KEY-----[\s\S]*?-----END [A-Z ]*PRIV` + `ATE KEY-----`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	{"openai_key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`)},
	{"github_token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{"aws_access_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"slack_token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"bearer_token", regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._-]{16,}`)},
	// Assignment shapes: keep the key name, redact the value.
	{"credential_assignment", regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|passwd|secret)\b\s*[:=]\s*("[^"]{6,}"|'[^']{6,}'|[^\s,;]{6,})`)},
}

// Detect reports the first secret shape found in text.
func Detect(text string) (kind string, found bool) {
	for _, p := range patterns {
		if p.re.MatchString(text) {
			return p.kind, true
		}
	}
	return "", false
}

// Redact replaces every recognised secret with a visible marker and returns
// the kinds seen (sorted, deduped). Untouched text is returned unchanged.
func Redact(text string) (string, []string) {
	clean := text
	var kinds []string
	seen := map[string]bool{}
	for _, p := range patterns {
		if !p.re.MatchString(clean) {
			continue
		}
		if !seen[p.kind] {
			seen[p.kind] = true
			kinds = append(kinds, p.kind)
		}
		clean = p.re.ReplaceAllString(clean, "[redacted:"+p.kind+"]")
	}
	sort.Strings(kinds)
	return clean, kinds
}

// ContainsSecret is a cheap boolean form for pre-write checks.
func ContainsSecret(text string) bool {
	_, found := Detect(text)
	return found
}

// RedactionNote is the visible suffix appended when a memory had to be
// screened; it states what happened without repeating the value.
func RedactionNote(kinds []string) string {
	if len(kinds) == 0 {
		return ""
	}
	return " [secret redacted: " + strings.Join(kinds, ", ") + "]"
}
