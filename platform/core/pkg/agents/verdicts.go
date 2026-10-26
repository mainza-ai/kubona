package agents

import (
	"fmt"
	"strings"
)

// TurnLifecycleVerdict is the sequence state within the 4-turn tribunal.
type TurnLifecycleVerdict string

const (
	LifecycleProposed  TurnLifecycleVerdict = "PROPOSED"
	LifecycleAnchored  TurnLifecycleVerdict = "ANCHORED"
	LifecycleAudited   TurnLifecycleVerdict = "AUDITED"
	LifecycleCollapsed TurnLifecycleVerdict = "COLLAPSED"
)

// DeliberationOutcome is the decision result of tribunal debate or claim scoring.
type DeliberationOutcome string

const (
	OutcomePass    DeliberationOutcome = "pass"
	OutcomeFail    DeliberationOutcome = "fail"
	OutcomeRefuted DeliberationOutcome = "REFUTED"
)

// DisconfirmationFinding mirrors Python's DisconfirmationVerdict polarity.
type DisconfirmationFinding string

const (
	FindingSurvives     DisconfirmationFinding = "SURVIVES"
	FindingFalsified    DisconfirmationFinding = "FALSIFIED"
	FindingInconclusive DisconfirmationFinding = "INCONCLUSIVE"
)

// ParseLifecycleVerdict validates a turn lifecycle verdict string.
func ParseLifecycleVerdict(s string) (TurnLifecycleVerdict, error) {
	switch TurnLifecycleVerdict(strings.ToUpper(strings.TrimSpace(s))) {
	case LifecycleProposed:
		return LifecycleProposed, nil
	case LifecycleAnchored:
		return LifecycleAnchored, nil
	case LifecycleAudited:
		return LifecycleAudited, nil
	case LifecycleCollapsed:
		return LifecycleCollapsed, nil
	default:
		return "", fmt.Errorf("invalid lifecycle verdict %q: expected PROPOSED|ANCHORED|AUDITED|COLLAPSED", s)
	}
}

// ParseDeliberationOutcome validates a deliberation outcome string.
func ParseDeliberationOutcome(s string) (DeliberationOutcome, error) {
	switch DeliberationOutcome(strings.TrimSpace(s)) {
	case OutcomePass:
		return OutcomePass, nil
	case OutcomeFail:
		return OutcomeFail, nil
	case OutcomeRefuted:
		return OutcomeRefuted, nil
	default:
		return "", fmt.Errorf("invalid deliberation outcome %q: expected pass|fail|REFUTED", s)
	}
}

// ParseDisconfirmationFinding validates a disconfirmation finding string.
func ParseDisconfirmationFinding(s string) (DisconfirmationFinding, error) {
	switch DisconfirmationFinding(strings.ToUpper(strings.TrimSpace(s))) {
	case FindingSurvives:
		return FindingSurvives, nil
	case FindingFalsified:
		return FindingFalsified, nil
	case FindingInconclusive:
		return FindingInconclusive, nil
	default:
		return "", fmt.Errorf("invalid disconfirmation finding %q: expected SURVIVES|FALSIFIED|INCONCLUSIVE", s)
	}
}

// FindingFromPythonBool maps Python DisconfirmationOfficer output to DisconfirmationFinding.
func FindingFromPythonBool(isFalsified bool, classicalShortcutFound bool, evidenceDefectsCount int) DisconfirmationFinding {
	if isFalsified || classicalShortcutFound {
		return FindingFalsified
	}
	if evidenceDefectsCount > 0 {
		return FindingInconclusive
	}
	return FindingSurvives
}

// DeliberationFromFinding maps a DisconfirmationFinding to a DeliberationOutcome.
func DeliberationFromFinding(f DisconfirmationFinding) DeliberationOutcome {
	switch f {
	case FindingSurvives:
		return OutcomePass
	case FindingFalsified:
		return OutcomeRefuted
	case FindingInconclusive:
		return OutcomeFail
	default:
		return OutcomeFail
	}
}

// LifecycleFromFinding maps a DisconfirmationFinding to a terminal TurnLifecycleVerdict.
func LifecycleFromFinding(f DisconfirmationFinding) TurnLifecycleVerdict {
	switch f {
	case FindingSurvives:
		return LifecycleCollapsed
	case FindingFalsified, FindingInconclusive:
		return LifecycleAudited
	default:
		return LifecycleProposed
	}
}
