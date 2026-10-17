package agents

// R2 — obligation lifecycle: the rules that make closure mean something.
// Storage lives in pkg/ledger/obligation.go (dumb tables); this file
// decides. AttemptClose refuses with named reasons; callers surface them
// instead of prose.
//
// Close requires ALL of:
//  1. exactly one owner (set-once at the store; empty refuses);
//  2. every required case kind has ≥1 pass AND zero fail/contested;
//  3. no fail/contested verdict on ANY recorded kind (a contradiction
//     anywhere keeps it open — narrow passes don't outvote a refutation);
//  4. every required authority has passed ≥1 case (comparable-authority
//     rule; operator default: tribunal quorum = the four seated roles);
//  5. every dependency obligation closed (their own closure already
//     required theirs — transitivity by construction);
//  6. zero unresolved residual unknowns (P1-9 absorption);
//  7. at least one recorded case (empty input ⇒ unknown, never closure).
// A passing close records a system "closure" case (authority
// "obligation-engine") so the closed state is itself queryable evidence.

import (
	"fmt"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// AttemptClose evaluates an obligation. Returns closed=true (and flips
// disposition + seals the closure case) or closed=false with the reasons
// that blocked it. Reasons are stable strings callers surface verbatim.
func AttemptClose(l *ledger.Ledger, id string) (bool, []string, error) {
	ob, err := l.GetObligation(id)
	if err != nil {
		return false, nil, err
	}
	if ob == nil {
		return false, nil, fmt.Errorf("unknown obligation %q", id)
	}
	if ob.Disposition == "closed" {
		return true, nil, nil
	}
	var reasons []string
	if ob.Owner == "" {
		reasons = append(reasons, "no owner: close requires exactly one accountable owner")
	}
	cases, err := l.ListObligationCases(id)
	if err != nil {
		return false, nil, err
	}
	if len(cases) == 0 {
		reasons = append(reasons, "no cases recorded: empty input never closes")
	}
	byKind := map[string][]ledger.ObligationCase{}
	for _, c := range cases {
		byKind[c.CaseKind] = append(byKind[c.CaseKind], c)
	}
	for kind, cs := range byKind {
		for _, c := range cs {
			if c.Verdict == "fail" || c.Verdict == "contested" {
				reasons = append(reasons, fmt.Sprintf("contradiction on %q by %s: refutations keep it open", kind, c.Authority))
				break
			}
		}
	}
	for _, kind := range ob.RequiredCases {
		cs := byKind[kind]
		pass := false
		for _, c := range cs {
			if c.Verdict == "pass" {
				pass = true
			}
		}
		if !pass {
			reasons = append(reasons, fmt.Sprintf("required case %q has no passing verdict", kind))
		}
	}
	passedBy := map[string]bool{}
	for _, c := range cases {
		if c.Verdict == "pass" {
			passedBy[c.Authority] = true
		}
	}
	for _, auth := range ob.RequiredAuthorities {
		if !passedBy[auth] {
			reasons = append(reasons, fmt.Sprintf("required authority %q has no passing case", auth))
		}
	}
	for _, dep := range ob.Dependencies {
		d, err := l.GetObligation(dep)
		if err != nil {
			return false, nil, err
		}
		if d == nil || d.Disposition != "closed" {
			reasons = append(reasons, fmt.Sprintf("dependency %s is not closed", dep))
		}
	}
	if len(ob.Unknowns) > 0 {
		ids := make([]string, 0, len(ob.Unknowns))
		for _, u := range ob.Unknowns {
			ids = append(ids, u.ID)
		}
		reasons = append(reasons, fmt.Sprintf("residual unknowns block closure: %v", ids))
	}
	if len(reasons) > 0 {
		return false, reasons, nil
	}
	if err := l.RecordObligationCase(id, ledger.ObligationCase{
		CaseKind: "closure", Verdict: "pass", Authority: "obligation-engine",
		Evidence: fmt.Sprintf("owner=%s cases=%d", ob.Owner, len(cases)),
	}); err != nil {
		return false, nil, err
	}
	if err := l.SetObligationDisposition(id, "closed"); err != nil {
		return false, nil, err
	}
	return true, nil, nil
}
