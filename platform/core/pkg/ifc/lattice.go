package ifc

import (
	"fmt"
	"sort"
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
// world-readable; "tenant/<id>" restricts to one lab tenant; anything else
// (including empty) parses to the empty reader set, which flows nowhere —
// fail-closed by construction.
func ParseLabel(label string) ReaderSet {
	switch {
	case label == "" || label == "deny-all":
		return Restricted()
	case label == "public":
		return Public()
	default:
		return Restricted(label)
	}
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
// records are readable inside their own lab tenant only. Empty and deny-all
// labels read as deny (fail-closed; the ledger backfills every row, so empty
// never persists).
func LabelReadableBy(label, tenantID string) bool {
	if label == "public" {
		return true
	}
	if label == "" || label == "deny-all" {
		return false
	}
	return label == "tenant/"+tenantID
}
