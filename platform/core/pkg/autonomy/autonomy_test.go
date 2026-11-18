package autonomy

import (
	"strings"
	"testing"
)

func TestDefaultPolicyAndNormalize(t *testing.T) {
	def := DefaultPolicy()
	if def.ModeFor(StageExecute) != ModeApprove {
		t.Fatalf("default execute mode = %q", def.ModeFor(StageExecute))
	}
	if def.MaxActBudgetUSD != 1.0 {
		t.Fatalf("default act budget = %v", def.MaxActBudgetUSD)
	}
	// Missing stages filled, explicit stage kept.
	got, err := Normalize(Policy{Stages: map[Stage]Mode{StageExecute: ModeAct}})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got.ModeFor(StageExecute) != ModeAct || got.ModeFor(StagePlan) != ModeAct ||
		got.ModeFor(StageHypothesize) != ModeAct || got.ModeFor(StageCollapse) != ModeAct {
		t.Fatalf("filled policy = %+v", got)
	}
	for _, bad := range []Policy{
		{Stages: map[Stage]Mode{"teleport": ModeAct}},
		{Stages: map[Stage]Mode{StageExecute: "automatic"}},
		{MaxActBudgetUSD: -1},
		{MaxActBudgetUSD: 5000},
	} {
		if _, err := Normalize(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

func TestAgenticMasterSwitchDefaultsAndNormalize(t *testing.T) {
	def := DefaultPolicy()
	if !def.Agentic() || def.AgenticEnabled == nil {
		t.Fatalf("default agentic = %+v", def)
	}
	// A pre-switch policy (field absent) normalizes to enabled, never silently
	// off.
	legacy := Policy{Stages: map[Stage]Mode{StageExecute: ModeApprove}}
	got, err := Normalize(legacy)
	if err != nil {
		t.Fatalf("normalize legacy: %v", err)
	}
	if !got.Agentic() || got.AgenticEnabled == nil {
		t.Fatalf("legacy policy normalized off: %+v", got)
	}
	off := false
	disabled := Policy{Stages: map[Stage]Mode{StageExecute: ModeApprove}, AgenticEnabled: &off, AgenticLocked: true}
	got, err = Normalize(disabled)
	if err != nil {
		t.Fatalf("normalize disabled: %v", err)
	}
	if got.Agentic() || !got.AgenticLocked {
		t.Fatalf("explicit off/lock lost: %+v", got)
	}
}

func TestClassifyRisk(t *testing.T) {
	low := Classify(RiskInput{Method: "vqe", Spec: map[string]any{"molecule": "H2"}, BudgetUSD: 1, MaxActBudgetUSD: 1})
	if low.Level != RiskLow || len(low.Reasons) == 0 {
		t.Fatalf("low = %+v", low)
	}
	high := Classify(RiskInput{Method: "vqe", Spec: map[string]any{"hardware": true}, BudgetUSD: 1, MaxActBudgetUSD: 1})
	if high.Level != RiskHigh {
		t.Fatalf("hardware = %+v", high)
	}
	dual := Classify(RiskInput{Method: "vqe", Statement: "synthesize a nerve agent precursor"})
	if dual.Level != RiskHigh {
		t.Fatalf("dual use = %+v", dual)
	}
	route := Classify(RiskInput{Method: "vqe", Statement: "map a synthesis route for a precursor"})
	if route.Level != RiskHigh {
		t.Fatalf("synthesis route = %+v", route)
	}
	over := Classify(RiskInput{Method: "vqe", Spec: map[string]any{}, BudgetUSD: 5, MaxActBudgetUSD: 1})
	if over.Level != RiskHigh {
		t.Fatalf("over budget = %+v", over)
	}
	large := Classify(RiskInput{Method: "spin_chain", Spec: map[string]any{"n_sites": float64(16)}, BudgetUSD: 1, MaxActBudgetUSD: 1})
	if large.Level != RiskMedium {
		t.Fatalf("large chain = %+v", large)
	}
	full := Classify(RiskInput{Method: "vqe", Spec: map[string]any{"full_space": true}, BudgetUSD: 1, MaxActBudgetUSD: 1})
	if full.Level != RiskMedium {
		t.Fatalf("full space = %+v", full)
	}
}

func TestDecideExecuteMatrix(t *testing.T) {
	low := RiskInput{Method: "vqe", Spec: map[string]any{}, BudgetUSD: 1, MaxActBudgetUSD: 1}
	high := RiskInput{Method: "vqe", Spec: map[string]any{"hardware": true}, BudgetUSD: 1, MaxActBudgetUSD: 1}

	suggest := DefaultPolicy()
	suggest.Stages[StageExecute] = ModeSuggest
	if d := DecideExecute(suggest, "approved", low); d.Allowed || d.Code != "autonomy_suggest_only" {
		t.Fatalf("suggest allowed execution: %+v", d)
	}
	if d := DecideExecute(DefaultPolicy(), "proposed", low); d.Allowed || !d.RequiresApproval || d.Code != "plan_not_approved" {
		t.Fatalf("approve mode without approval = %+v", d)
	}
	if d := DecideExecute(DefaultPolicy(), "approved", low); !d.Allowed {
		t.Fatalf("approved plan refused = %+v", d)
	}
	act := DefaultPolicy()
	act.Stages[StageExecute] = ModeAct
	if d := DecideExecute(act, "proposed", low); !d.Allowed || !d.AutoApprove {
		t.Fatalf("act low risk = %+v", d)
	}
	if d := DecideExecute(act, "proposed", high); d.Allowed || !d.RequiresApproval {
		t.Fatalf("act high risk without approval = %+v", d)
	}
	if d := DecideExecute(act, "approved", high); !d.Allowed {
		t.Fatalf("approved high risk refused = %+v", d)
	}
	notify := DefaultPolicy()
	notify.Stages[StageExecute] = ModeNotify
	if d := DecideExecute(notify, "proposed", low); !d.Allowed || !d.Notify || !d.AutoApprove {
		t.Fatalf("notify low risk = %+v", d)
	}
	if d := DecideExecute(notify, "proposed", high); d.Allowed || !d.RequiresApproval {
		t.Fatalf("notify high risk = %+v", d)
	}
}

func TestRiskCategoriesCoverDomainAndProtocolAxes(t *testing.T) {
	cases := []struct {
		name      string
		statement string
		spec      map[string]any
		budget    float64
		cap       float64
		want      string
	}{
		{"domain chem", "synthesize a nerve agent precursor", nil, 1, 10, "chem"},
		{"domain bio", "raise pathogen transmissibility", nil, 1, 10, "bio"},
		{"domain radiological", "enrich fissile material", nil, 1, 10, "radiological"},
		{"domain cyber", "deploy ransomware against a university", nil, 1, 10, "cyber"},
		{"protocol human", "run a clinical trial with patient data", nil, 1, 10, "human-subjects"},
		{"protocol animal", "in vivo primate study", nil, 1, 10, "animal"},
		{"protocol environmental", "open-air trial with environmental release", nil, 1, 10, "environmental"},
		{"hardware", "run the circuit", map[string]any{"hardware": true}, 1, 10, "hardware"},
		{"budget", "simulate the H2 ground state", nil, 20, 10, "budget"},
	}
	for _, tc := range cases {
		risk := Classify(RiskInput{
			Method: "vqe", Statement: tc.statement, Spec: tc.spec,
			BudgetUSD: tc.budget, MaxActBudgetUSD: tc.cap,
		})
		if risk.Level != RiskHigh {
			t.Errorf("%s: level = %s, want high (%+v)", tc.name, risk.Level, risk)
		}
		found := false
		for _, category := range risk.Categories {
			if category == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: categories %v missing %q", tc.name, risk.Categories, tc.want)
		}
	}
	// A categories hit is also visible in the sealed reasons with its axis.
	chem := Classify(RiskInput{Method: "vqe", Statement: "synthesize a nerve agent precursor", BudgetUSD: 1, MaxActBudgetUSD: 10})
	joined := strings.Join(chem.Reasons, " | ")
	if !strings.Contains(joined, "chem: nerve agent") || !strings.Contains(joined, "chem: precursor") {
		t.Fatalf("categorical reasons missing: %s", joined)
	}
	for i := 1; i < len(chem.Categories); i++ {
		if chem.Categories[i-1] > chem.Categories[i] {
			t.Fatalf("categories are not sorted: %v", chem.Categories)
		}
	}
	// Benign local work stays low and uncategorized.
	benign := Classify(RiskInput{Method: "vqe", Statement: "simulate the H2 ground state", BudgetUSD: 1, MaxActBudgetUSD: 10})
	if benign.Level != RiskLow || len(benign.Categories) != 0 {
		t.Fatalf("benign = %+v", benign)
	}
}
