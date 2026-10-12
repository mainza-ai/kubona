package agents

import (
	"strings"
	"testing"
)

// J2: cloud escalation is spend-gated. Keywords alone never spend;
// the mandate/budget backing named in the explanation must exist.
func TestRouteMultimodalSpendGate(t *testing.T) {
	femoco := MultimodalPayload{
		Prompt: "Optimize VQE ansatz for FeMoco",
		Items:  []MultimodalItem{{Modality: "molecule_3d", Data: "FeMoCo"}},
	}

	// Unfunded complex prompt routes local, with no mandate claim.
	d := RouteMultimodal(femoco, false)
	if !d.IsLocal || d.AirGapEnforced {
		t.Errorf("unfunded cloud-style prompt must route local-default, got %+v", d)
	}
	if strings.Contains(d.Explanation, "mandate") {
		t.Errorf("must not claim mandate backing without one: %q", d.Explanation)
	}

	// Budget-authorized routes cloud.
	femoco.SpendBudgetUSD = 5.0
	d = RouteMultimodal(femoco, false)
	if d.IsLocal {
		t.Errorf("funded complex prompt must route cloud, got %+v", d)
	}

	// Mandate-authorized routes cloud with the mandate named.
	femoco.SpendBudgetUSD = 0
	femoco.HasEd25519Mandate = true
	d = RouteMultimodal(femoco, false)
	if d.IsLocal || !strings.Contains(d.Explanation, "AP2 mandate backing") {
		t.Errorf("mandated prompt must route cloud with backing named, got %+v", d)
	}

	// Air-gap is the only enforced branch, and it says so.
	d = RouteMultimodal(MultimodalPayload{Prompt: "anything", SpendBudgetUSD: 99}, true)
	if !d.IsLocal || !d.AirGapEnforced {
		t.Errorf("air-gapped must route enforced-local, got %+v", d)
	}

	// Plain default never claims enforcement.
	d = RouteMultimodal(MultimodalPayload{Prompt: "compile this", SpendBudgetUSD: 1}, false)
	if !d.IsLocal || d.AirGapEnforced {
		t.Errorf("plain default must be local-unenforced, got %+v", d)
	}
}
