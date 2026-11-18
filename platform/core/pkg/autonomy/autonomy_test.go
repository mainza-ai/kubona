package autonomy

import "testing"

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
}
