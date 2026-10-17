package agents

import (
	"strings"
	"testing"
)

// J2: cloud escalation is spend-gated. Keywords alone never spend;
// the mandate/budget backing named in the explanation must exist.
// Models are never invented: caller-supplied preferred_model wins, else
// the configured fallback, else refusal (no baked-in frontier).
func TestRouteMultimodalSpendGate(t *testing.T) {
	femoco := MultimodalPayload{
		Prompt: "Optimize VQE ansatz for FeMoCo",
		Items:  []MultimodalItem{{Modality: "molecule_3d", Data: "FeMoCo"}},
	}

	// No model anywhere refuses outright — never routes a stale default.
	d := RouteMultimodal(femoco, false, "")
	if d.Ok {
		t.Errorf("missing model must refuse, got %+v", d)
	}

	// Unfunded explicit cloud model refuses (never silently downgraded).
	d = RouteMultimodal(MultimodalPayload{Prompt: "x", PreferredModel: "anthropic/claude-x"}, false, "local/m")
	if d.Ok {
		t.Errorf("unfunded cloud request must refuse, got %+v", d)
	}

	// Budget-authorized explicit cloud model routes with backing named.
	funded := MultimodalPayload{Prompt: "Optimize VQE ansatz for FeMoCo", PreferredModel: "anthropic/claude-x", SpendBudgetUSD: 5.0}
	d = RouteMultimodal(funded, false, "local/m")
	if !d.Ok || d.IsLocal {
		t.Fatalf("funded explicit cloud must route cloud, got %+v", d)
	}
	if !strings.Contains(d.Explanation, "spend budget declared") {
		t.Errorf("must name the backing, got %q", d.Explanation)
	}
	if d.Model != "anthropic/claude-x" || d.Provider != "anthropic" {
		t.Errorf("must echo the requested model, got %+v", d)
	}

	// Mandate-authorized routes cloud with the mandate named.
	funded.SpendBudgetUSD = 0
	funded.HasEd25519Mandate = true
	d = RouteMultimodal(funded, false, "local/m")
	if !d.Ok || d.IsLocal || !strings.Contains(d.Explanation, "AP2 mandate backing") {
		t.Errorf("mandated prompt must route cloud with backing named, got %+v", d)
	}

	// Air-gap enforces local for local models and refuses cloud ones.
	d = RouteMultimodal(MultimodalPayload{Prompt: "anything", PreferredModel: "local/m", SpendBudgetUSD: 99}, true, "")
	if !d.Ok || !d.IsLocal || !d.AirGapEnforced {
		t.Errorf("air-gapped local must route enforced-local, got %+v", d)
	}
	d = RouteMultimodal(MultimodalPayload{Prompt: "anything", PreferredModel: "anthropic/claude-x", SpendBudgetUSD: 99}, true, "")
	if d.Ok {
		t.Errorf("air-gapped cloud must refuse, got %+v", d)
	}

	// Fallback model fills in when the caller names none.
	d = RouteMultimodal(MultimodalPayload{Prompt: "compile this", SpendBudgetUSD: 1}, false, "local/fleet-model")
	if !d.Ok || !d.IsLocal || d.AirGapEnforced {
		t.Errorf("fallback model must route local-unenforced, got %+v", d)
	}
	if d.Model != "local/fleet-model" {
		t.Errorf("must use the configured fallback, got %q", d.Model)
	}
}
