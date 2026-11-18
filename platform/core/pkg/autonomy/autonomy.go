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
	ModeNotify  Mode = "notify"
)

// Stages is the stable order used in responses and validation errors.
var Stages = []Stage{StageHypothesize, StagePlan, StageExecute, StageCollapse}

type Policy struct {
	Stages          map[Stage]Mode `json:"stages"`
	MaxActBudgetUSD float64        `json:"max_act_budget_usd"`
	// AgenticEnabled is the session master switch. Nil means "unset" and
	// normalizes to true so a pre-switch policy keeps its existing behavior;
	// an explicit false blocks agent loops and autonomous plan execution while
	// leaving the manual experiment rail available.
	AgenticEnabled *bool `json:"agentic_enabled,omitempty"`
	// AgenticLocked is set by an operator halt. Session callers may toggle the
	// switch but cannot clear the lock; only the operator policy write can.
	AgenticLocked bool `json:"agentic_locked"`
	// ToolPermissions gates copilot tools by name: allow | ask | block.
	// Absent means allow (the tool registry's own role gate still applies).
	ToolPermissions map[string]string `json:"tool_permissions,omitempty"`
	UpdatedAt       string            `json:"updated_at,omitempty"`
}

func boolPtr(v bool) *bool { return &v }

// Agentic reports the effective master switch (default true).
func (p Policy) Agentic() bool {
	if p.AgenticEnabled == nil {
		return true
	}
	return *p.AgenticEnabled
}

// ToolPermission returns allow|ask|block for a tool name (default allow).
func (p Policy) ToolPermission(name string) string {
	if p.ToolPermissions == nil {
		return "allow"
	}
	if v, ok := p.ToolPermissions[name]; ok {
		return v
	}
	return "allow"
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
		AgenticEnabled:  boolPtr(true),
	}
}

func ValidMode(m Mode) bool {
	switch m {
	case ModeSuggest, ModeApprove, ModeAct, ModeNotify:
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
	out := Policy{
		Stages: map[Stage]Mode{}, MaxActBudgetUSD: p.MaxActBudgetUSD,
		AgenticEnabled: p.AgenticEnabled, AgenticLocked: p.AgenticLocked,
		ToolPermissions: map[string]string{}, UpdatedAt: p.UpdatedAt,
	}
	for name, permission := range p.ToolPermissions {
		name = strings.TrimSpace(name)
		permission = strings.ToLower(strings.TrimSpace(permission))
		if name == "" {
			continue
		}
		switch permission {
		case "allow", "ask", "block":
			out.ToolPermissions[name] = permission
		default:
			return Policy{}, fmt.Errorf("tool %q permission %q is not allow|ask|block", name, permission)
		}
	}
	if out.AgenticEnabled == nil {
		out.AgenticEnabled = boolPtr(true)
	}
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
	// Categories is the protocol/domain taxonomy behind a high-risk verdict:
	// bio, chem, radiological, cyber, human-subjects, animal, environmental,
	// hardware, budget. Empty for low/medium work.
	Categories []string `json:"categories,omitempty"`
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

// riskCategories is the v2 screening taxonomy. Terms are deliberately
// over-inclusive: a false positive costs an approval, a false negative costs the
// safety rail. Domain categories are about what the work could produce;
// protocol categories are about who/what it runs on. A hit always forces high
// risk and is sealed with the reason so the reviewer sees which axis fired.
type riskCategory struct {
	ID    string
	Terms []string
}

var riskCategories = []riskCategory{
	{ID: "bio", Terms: []string{
		"pathogen", "toxin", "bioweapon", "biological weapon", "virulent",
		"transmissibility", "gain-of-function", "select agent", "aerosolize",
		"aerosolise", "cultivation", "dispersal", "casualty",
	}},
	{ID: "chem", Terms: []string{
		"nerve agent", "chemical weapon", "precursor", "synthesis route",
		"explosive", "detonator",
	}},
	{ID: "radiological", Terms: []string{
		"fissile", "nuclear weapon", "radiological", "enrichment",
	}},
	{ID: "cyber", Terms: []string{
		"malware", "ransomware", "botnet", "exploit kit", "phishing",
		"credential theft",
	}},
	{ID: "human-subjects", Terms: []string{
		"human subject", "clinical trial", "informed consent", "patient data",
	}},
	{ID: "animal", Terms: []string{
		"animal subject", "animal testing", "in vivo", "primate",
	}},
	{ID: "environmental", Terms: []string{
		"environmental release", "open-air trial", "release into the environment",
	}},
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
	categories := map[string]bool{}
	for _, category := range riskCategories {
		for _, term := range category.Terms {
			if strings.Contains(text, term) {
				raise(RiskHigh)
				categories[category.ID] = true
				reasons = append(reasons, category.ID+": "+term)
			}
		}
	}
	if truthy(in.Spec["hardware"]) || truthy(in.Spec["qpu"]) ||
		strings.Contains(strings.ToLower(in.Method), "hardware") {
		raise(RiskHigh)
		categories["hardware"] = true
		reasons = append(reasons, "hardware/QPU execution")
	}
	if backend, ok := in.Spec["backend"].(string); ok && strings.TrimSpace(backend) != "" {
		raise(RiskHigh)
		categories["hardware"] = true
		reasons = append(reasons, "vendor backend requested: "+backend)
	}
	if in.MaxActBudgetUSD >= 0 && in.BudgetUSD > in.MaxActBudgetUSD {
		raise(RiskHigh)
		categories["budget"] = true
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
	labels := make([]string, 0, len(categories))
	for id := range categories {
		labels = append(labels, id)
	}
	sort.Strings(labels)
	return Risk{Level: level, Reasons: reasons, Categories: labels}
}

// Decision is what the execution path may do.
type Decision struct {
	Allowed          bool
	RequiresApproval bool
	AutoApprove      bool
	Notify           bool
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
		return Decision{Allowed: true, Notify: mode == ModeNotify, Risk: risk, Reason: "plan approved"}
	}
	if mode == ModeApprove {
		return Decision{
			Allowed: false, RequiresApproval: true, Risk: risk, Code: "plan_not_approved",
			Reason: "session autonomy is approve: the plan must be approved before execution",
		}
	}
	if mode == ModeNotify {
		return Decision{
			Allowed: true, AutoApprove: true, Notify: true, Risk: risk,
			Reason: "notify mode: low/medium-risk plan inside the act budget; the receipt is batched into the digest",
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
