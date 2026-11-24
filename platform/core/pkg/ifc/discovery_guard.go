package ifc

import (
	"errors"
	"fmt"
)

// DiscoveryUniverse scopes IFC confidentiality labels for autonomous discovery
// campaigns so held-out noise models, catalogs, and evaluation seeds cannot
// leak into proposer LLM prompts (Gate 5 enforcement).
const (
	DiscoveryUniverse = "discovery"
	PrincipalVerifier = "verifier"
	PrincipalProposer = "proposer"
)

// ErrHeldOutLeakage is returned when a held-out artifact attempts to flow into
// a proposer-visible context.
var ErrHeldOutLeakage = errors.New("ifc violation: held-out discovery artifact cannot flow to proposer context")

// PublicDiscoveryLabel tags frontier/public items readable by both proposer and verifier.
func PublicDiscoveryLabel() ConfidentialityLabel {
	return NewLabel(DiscoveryUniverse, Public())
}

// HeldOutDiscoveryLabel tags held-out noise models, catalogs, and seeds readable ONLY by verifier.
func HeldOutDiscoveryLabel() ConfidentialityLabel {
	return NewLabel(DiscoveryUniverse, Restricted(PrincipalVerifier))
}

// ProposerContextLabel is the destination label for any LLM proposer prompt.
func ProposerContextLabel() ConfidentialityLabel {
	return NewLabel(DiscoveryUniverse, Restricted(PrincipalProposer))
}

// VerifierContextLabel is the destination label for the deterministic verifier.
func VerifierContextLabel() ConfidentialityLabel {
	return NewLabel(DiscoveryUniverse, Restricted(PrincipalVerifier))
}

// TaggedDiscoveryItem binds an artifact name/kind to its IFC confidentiality label.
type TaggedDiscoveryItem struct {
	Name  string
	Label ConfidentialityLabel
}

// GuardProposerContext verifies every item destined for an LLM proposer prompt
// is permitted by the IFC reader-set lattice. Any held-out item fails closed.
func GuardProposerContext(items []TaggedDiscoveryItem) error {
	dest := ProposerContextLabel()
	for _, item := range items {
		if !item.Label.CanFlowTo(dest) {
			return fmt.Errorf("%w: item %q (%s -> %s)", ErrHeldOutLeakage, item.Name, item.Label.Readers, dest.Readers)
		}
	}
	return nil
}

// GuardVerifierContext verifies every item destined for the deterministic verifier
// is permitted by the IFC reader-set lattice (allows both public and held-out items).
func GuardVerifierContext(items []TaggedDiscoveryItem) error {
	dest := VerifierContextLabel()
	for _, item := range items {
		if !item.Label.CanFlowTo(dest) {
			return fmt.Errorf("ifc violation: item %q cannot flow to verifier (%s -> %s)", item.Name, item.Label.Readers, dest.Readers)
		}
	}
	return nil
}
