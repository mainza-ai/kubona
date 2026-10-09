package ifc_test

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ifc"
)

func TestIFCInformationFlow(t *testing.T) {
	pub := ifc.Public()
	aliceBob := ifc.Restricted("alice", "bob")
	aliceOnly := ifc.Restricted("alice")
	carol := ifc.Restricted("carol")

	// 1. Restricted to narrowed readers is allowed
	if !aliceBob.CanFlowTo(aliceOnly) {
		t.Error("expected {alice, bob} -> {alice} to be permitted")
	}

	// 2. Restricted to widened or disjoint readers is forbidden
	if aliceBob.CanFlowTo(carol) {
		t.Error("expected {alice, bob} -> {carol} to be forbidden")
	}

	// 3. Restricted to public is forbidden
	if aliceBob.CanFlowTo(pub) {
		t.Error("expected {alice, bob} -> public to be forbidden")
	}

	// 4. Public to restricted is allowed
	if !pub.CanFlowTo(aliceOnly) {
		t.Error("expected public -> {alice} to be permitted")
	}

	// 5. Lattice Join intersects readers
	joined := aliceBob.Join(ifc.Restricted("alice", "carol"))
	if !joined.CanFlowTo(aliceOnly) || joined.CanFlowTo(ifc.Restricted("bob")) {
		t.Errorf("expected join to intersect to {alice}, got %v", joined)
	}
}

func TestJoinLabelsShrinksAudience(t *testing.T) {
	if got := ifc.JoinLabels("public", "public"); got != "public" {
		t.Fatalf("public+public must stay public, got %q", got)
	}
	if got := ifc.JoinLabels("public", "tenant/lab-a"); got != "tenant/lab-a" {
		t.Fatalf("public+tenant must narrow to tenant, got %q", got)
	}
	if got := ifc.JoinLabels("tenant/lab-a", "tenant/lab-b"); got != "deny-all" {
		t.Fatalf("mixed tenants must deny all, got %q", got)
	}
	if got := ifc.JoinLabels("tenant/lab-a", "tenant/lab-a"); got != "tenant/lab-a" {
		t.Fatalf("same tenant must stay, got %q", got)
	}
}

func TestLabelReadableBy(t *testing.T) {
	if !ifc.LabelReadableBy("public", "lab-x") {
		t.Fatalf("public must be world-readable")
	}
	if !ifc.LabelReadableBy("tenant/lab-a", "lab-a") {
		t.Fatalf("own tenant must read")
	}
	if ifc.LabelReadableBy("tenant/lab-a", "lab-b") {
		t.Fatalf("foreign tenant must not read")
	}
	if ifc.LabelReadableBy("deny-all", "lab-a") {
		t.Fatalf("deny-all must not read")
	}
	if ifc.LabelReadableBy("", "lab-a") {
		t.Fatalf("empty label must fail closed (deny)")
	}
}
