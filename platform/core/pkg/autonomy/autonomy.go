package autonomy

// Stage-level autonomy and risk screening (AG-7).
//
// Tenant autonomy (L0–L3) is the outer bound: L0 refuses all dispatch. This
// package adds the finer rail the agent loop needs — a per-session mode for
// each research stage, a risk classification for a concrete plan, and one
// decision function the execution path must call.
//
//   suggest  the agent may propose; a human must execute through the manual
//            experiment rail.
//   approve  the agent may execute only an approved plan (the platform default,
//            matching the existing plan-approval gate).
//   act      low/medium-risk plans inside the act budget execute autonomously;
//            high-risk or over-budget plans still require approval.
//
// High risk always requires approval in every mode: "act" is bounded autonomy,
// not an override. The classifier is a v1 heuristic and says so in its
// reasons; it is not a safety certification.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type Stage string

const (
	StageHypothesize Stage = "hypothesize"
	StagePlan        Stage = "plan"
	StageExecute     Stage = "execute"
	StageCollapse    Stage = "collapse"
)

type Mode string

const (
	ModeSuggest Mode = "suggest"
	ModeApprove Mode = "approve"
	ModeAct     Mode = "act"
)

// Stages is the stable order used in responses and validation errors.
var Stages = []Stage{StageHypothesize, StagePlan, StageExecute, StageCollapse}

type Policy struct {
	Stages          map[Stage]Mode `json:"stages"`
	MaxActBudgetUSD float64        `json:"max_act_budget_usd"`
	UpdatedAt       string         `json:"updated_at,omitempty"`
}

// DefaultPolicy approximates the platform's existing posture: free stages are
// open, execution requires approval.
func DefaultPolicy() Policy {
	return Policy{
		Stages: map[Stage]Mode{
			StageHypothesize: ModeAct,
			StagePlan:        ModeAct,
			StageExecute:     ModeApprove,
			StageCollapse:    ModeAct,
		},
		MaxActBudgetUSD: 1.0,
	}
}

func ValidMode(m Mode) bool {
	switch m {
	case ModeSuggest, ModeApprove, ModeAct:
		return true
	}
	return false
}

func ValidStage(s Stage) bool {
	for _, known := range Stages {
		if known == s {
			return true
		}
	}
	return false
}

// Normalize fills missing stages from the default, validates every mode, and
// bounds the act budget. Unknown stages are refused rather than ignored.
func Normalize(p Policy) (Policy, error) {
	def := DefaultPolicy()
	out := Policy{Stages: map[Stage]Mode{}, MaxActBudgetUSD: p.MaxActBudgetUSD, UpdatedAt: p.UpdatedAt}
	if out.MaxActBudgetUSD == 0 {
		out.MaxActBudgetUSD = def.MaxActBudgetUSD
	}
	if math.IsNaN(out.MaxActBudgetUSD) || math.IsInf(out.MaxActBudgetUSD, 0) ||
		out.MaxActBudgetUSD < 0 || out.MaxActBudgetUSD > 1000 {
		return Policy{}, fmt.Errorf("max_act_budget_usd must be finite in [0,1000]")
	}
	for stage, mode := range p.Stages {
		if !ValidStage(stage) {
			return Policy{}, fmt.Errorf("unknown autonomy stage %q", stage)
		}
		if !ValidMode(mode) {
			return Policy{}, fmt.Errorf("stage %q mode %q is not suggest|approve|act", stage, mode)
		}
		out.Stages[stage] = mode
	}
	for _, stage := range Stages {
		if _, ok := out.Stages[stage]; !ok {
			out.Stages[stage] = def.Stages[stage]
		}
	}
	return out, nil
}

func (p Policy) ModeFor(stage Stage) Mode {
	if m, ok := p.Stages[stage]; ok {
		return m
	}
	return DefaultPolicy().Stages[stage]
}

const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

type Risk struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons,omitempty"`
}

// RiskInput is the concrete plan being classified. Spec values come from the
// job spec JSON.
type RiskInput struct {
	Method          string
	Spec            map[string]any
	BudgetUSD       float64
	Statement       string
	Target          string
	MaxActBudgetUSD float64
}

// dualUseTerms is a v1 screening list. It is deliberately over-inclusive: a
// false positive costs an approval, a false negative costs the safety rail.
var dualUseTerms = []string{
	"pathogen", "toxin", "nerve agent", "chemical weapon", "bioweapon",
	"biological weapon", "weaponize", "weaponise", "explosive", "fissile",
	"nuclear weapon", "radiological", "gain-of-function", "select agent",
	"aerosolize", "aerosolise", "casualty",
}

// Classify returns low/medium/high with the reasons that produced the level.
func Classify(in RiskInput) Risk {
	reasons := []string{}
	level := RiskLow
	raise := func(l string) {
		order := map[string]int{RiskLow: 0, RiskMedium: 1, RiskHigh: 2}
		if order[l] > order[level] {
			level = l
		}
	}
	text := strings.ToLower(strings.Join([]string{in.Statement, in.Target, fmt.Sprint(in.Spec)}, " "))
	for _, term := range dualUseTerms {
		if strings.Contains(text, term) {
			raise(RiskHigh)
			reasons = append(reasons, "dual-use term: "+term)
		}
	}
	if truthy(in.Spec["hardware"]) || truthy(in.Spec["qpu"]) ||
		strings.Contains(strings.ToLower(in.Method), "hardware") {
		raise(RiskHigh)
		reasons = append(reasons, "hardware/QPU execution")
	}
	if backend, ok := in.Spec["backend"].(string); ok && strings.TrimSpace(backend) != "" {
		raise(RiskHigh)
		reasons = append(reasons, "vendor backend requested: "+backend)
	}
	if in.MaxActBudgetUSD >= 0 && in.BudgetUSD > in.MaxActBudgetUSD {
		raise(RiskHigh)
		reasons = append(reasons, fmt.Sprintf("budget $%.2f exceeds the act cap $%.2f", in.BudgetUSD, in.MaxActBudgetUSD))
	}
	if n, ok := number(in.Spec["n_sites"]); ok && n >= 12 {
		raise(RiskMedium)
		reasons = append(reasons, fmt.Sprintf("large chain (n_sites=%d)", int(n)))
	}
	if truthy(in.Spec["full_space"]) {
		raise(RiskMedium)
		reasons = append(reasons, "full-space simulation")
	}
	if n, ok := number(in.Spec["max_iterations"]); ok && n > 500 {
		raise(RiskMedium)
		reasons = append(reasons, fmt.Sprintf("large optimizer budget (%d iterations)", int(n)))
	}
	if level == RiskLow {
		reasons = append(reasons, "local simulator work inside the act cap")
	}
	sort.Strings(reasons)
	return Risk{Level: level, Reasons: reasons}
}

// Decision is what the execution path may do.
type Decision struct {
	Allowed          bool
	RequiresApproval bool
	AutoApprove      bool
	Code             string
	Reason           string
	Risk             Risk
}

// DecideExecute applies the session policy and the plan's approval status.
func DecideExecute(policy Policy, planStatus string, in RiskInput) Decision {
	mode := policy.ModeFor(StageExecute)
	risk := Classify(in)
	approved := planStatus == "approved"

	if mode == ModeSuggest {
		return Decision{
			Allowed: false, RequiresApproval: true, Risk: risk, Code: "autonomy_suggest_only",
			Reason: "session autonomy is suggest for the execute stage: a human must execute this plan through the manual experiment rail",
		}
	}
	if risk.Level == RiskHigh && !approved {
		return Decision{
			Allowed: false, RequiresApproval: true, Risk: risk, Code: "risk_approval_required",
			Reason: "high-risk plan requires explicit human approval: " + strings.Join(risk.Reasons, "; "),
		}
	}
	if approved {
		return Decision{Allowed: true, Risk: risk, Reason: "plan approved"}
	}
	if mode == ModeApprove {
		return Decision{
			Allowed: false, RequiresApproval: true, Risk: risk, Code: "plan_not_approved",
			Reason: "session autonomy is approve: the plan must be approved before execution",
		}
	}
	return Decision{
		Allowed: true, AutoApprove: true, Risk: risk,
		Reason: "act mode: low/medium-risk plan inside the act budget",
	}
}

func truthy(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		s := strings.ToLower(strings.TrimSpace(b))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return b != 0
	}
	return false
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
