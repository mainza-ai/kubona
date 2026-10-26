package ifc

import (
	"fmt"
	"sort"
	"strings"
)

// ReaderSet models who may read restricted information (buzz ifc-core pattern)
type ReaderSet struct {
	IsPublic   bool
	Principals map[string]struct{}
}

func Public() ReaderSet {
	return ReaderSet{IsPublic: true, Principals: make(map[string]struct{})}
}

func Restricted(principals ...string) ReaderSet {
	pMap := make(map[string]struct{})
	for _, p := range principals {
		if p != "" {
			pMap[p] = struct{}{}
		}
	}
	return ReaderSet{IsPublic: false, Principals: pMap}
}

// CanFlowTo determines whether information readable by source may flow to destination
// Invariant: Information readable by {Alice, Bob} may flow to {Alice}, but CANNOT flow to {Alice, Bob, Carol}
func (s ReaderSet) CanFlowTo(dest ReaderSet) bool {
	if s.IsPublic {
		return true // Public information may flow anywhere
	}
	if dest.IsPublic {
		return false // Restricted information cannot flow to public
	}
	// Destination readers must be a subset of source readers (cannot widen readers)
	for destPrincipal := range dest.Principals {
		if _, exists := s.Principals[destPrincipal]; !exists {
			return false
		}
	}
	return true
}

// Join computes the least upper bound (intersection of readers)
func (s ReaderSet) Join(other ReaderSet) ReaderSet {
	if s.IsPublic && other.IsPublic {
		return Public()
	}
	if s.IsPublic {
		return other
	}
	if other.IsPublic {
		return s
	}

	intersected := make(map[string]struct{})
	for p := range s.Principals {
		if _, exists := other.Principals[p]; exists {
			intersected[p] = struct{}{}
		}
	}
	return ReaderSet{IsPublic: false, Principals: intersected}
}

func (s ReaderSet) String() string {
	if s.IsPublic {
		return "ReaderSet:Everyone"
	}
	var list []string
	for p := range s.Principals {
		list = append(list, p)
	}
	sort.Strings(list)
	return fmt.Sprintf("ReaderSet:Only%v", list)
}

// ConfidentialityLabel tags data records with universe and reader set
type ConfidentialityLabel struct {
	Universe string    `json:"universe"`
	Readers  ReaderSet `json:"readers"`
}

func NewLabel(universe string, readers ReaderSet) ConfidentialityLabel {
	return ConfidentialityLabel{Universe: universe, Readers: readers}
}

func (l ConfidentialityLabel) CanFlowTo(dest ConfidentialityLabel) bool {
	if l.Universe != dest.Universe {
		return false
	}
	return l.Readers.CanFlowTo(dest.Readers)
}

// ParseLabel decodes a stored confidentiality label string. "public" is
// world-readable; "tenant/<id>" or "party/<id>" restricts to specified principals;
// "readers:[...]" parses composite reader sets; anything unmodelable or empty
// parses to the empty reader set (deny-all), which flows nowhere — fail-closed by construction.
func ParseLabel(label string) ReaderSet {
	label = strings.TrimSpace(label)
	switch {
	case label == "" || label == "deny-all":
		return Restricted()
	case label == "public":
		return Public()
	case strings.HasPrefix(label, "readers:[") && strings.HasSuffix(label, "]"):
		inner := strings.TrimSuffix(strings.TrimPrefix(label, "readers:["), "]")
		parts := strings.Fields(strings.ReplaceAll(inner, ",", " "))
		var principals []string
		for _, p := range parts {
			clean := strings.TrimSpace(p)
			if clean != "" {
				principals = append(principals, clean)
			}
		}
		if len(principals) == 0 {
			return Restricted()
		}
		return Restricted(principals...)
	default:
		// Single principal tag (e.g. "tenant/acme", "party/nus")
		return Restricted(label)
	}
}

// ValidateOrRefuseLabel checks if a policy tag can be safely mapped to the
// ReaderSet lattice. Non-lattice claims (e.g. "research use only", "no commercial")
// are explicitly unmodelable in a static reader set and must fail-close.
func ValidateOrRefuseLabel(label string) (string, error) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" || trimmed == "deny-all" {
		return "deny-all", nil
	}
	if trimmed == "public" {
		return "public", nil
	}
	if strings.HasPrefix(trimmed, "tenant/") || strings.HasPrefix(trimmed, "party/") {
		return trimmed, nil
	}
	if strings.HasPrefix(trimmed, "readers:[") && strings.HasSuffix(trimmed, "]") {
		rs := ParseLabel(trimmed)
		if len(rs.Principals) == 0 {
			return "deny-all", nil
		}
		for p := range rs.Principals {
			if !strings.HasPrefix(p, "tenant/") && !strings.HasPrefix(p, "party/") {
				return "", fmt.Errorf("unmodelable principal %q in reader set: refusing to admit non-lattice tag", p)
			}
		}
		return FormatLabel(rs), nil
	}
	return "", fmt.Errorf("unmodelable confidentiality policy %q: refusing to admit non-lattice tag", label)
}

// FormatLabel encodes a reader set back to its canonical label string.
func FormatLabel(rs ReaderSet) string {
	if rs.IsPublic {
		return "public"
	}
	var list []string
	for p := range rs.Principals {
		list = append(list, p)
	}
	sort.Strings(list)
	switch len(list) {
	case 0:
		return "deny-all"
	case 1:
		return list[0]
	default:
		return "readers:" + fmt.Sprintf("%v", list)
	}
}

// JoinLabels computes the derivation label for a record built from the given
// input labels: least upper bound (intersection of readers). Mixing sources
// can only shrink the audience, never widen it.
func JoinLabels(labels ...string) string {
	joined := Public()
	for _, l := range labels {
		joined = joined.Join(ParseLabel(l))
	}
	return FormatLabel(joined)
}

// LabelReadableBy reports whether a requester identified by tenantID may read
// a record carrying the label. Public records are world-readable; tenant
// records are readable inside their own lab tenant only (or multi-party sets
// containing the tenant). Empty and deny-all labels read as deny (fail-closed;
// the ledger backfills every row, so empty never persists).
func LabelReadableBy(label, tenantID string) bool {
	if label == "public" {
		return true
	}
	if label == "" || label == "deny-all" {
		return false
	}
	rs := ParseLabel(label)
	if rs.IsPublic {
		return true
	}
	if len(rs.Principals) == 0 {
		return false
	}
	if _, ok := rs.Principals["tenant/"+tenantID]; ok {
		return true
	}
	if _, ok := rs.Principals[tenantID]; ok {
		return true
	}
	return false
}
