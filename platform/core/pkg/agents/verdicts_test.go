package agents

import (
	"testing"
)

func TestVerdictTransitions(t *testing.T) {
	// Parse lifecycle
	for _, raw := range []string{"PROPOSED", "ANCHORED", "AUDITED", "COLLAPSED"} {
		v, err := ParseLifecycleVerdict(raw)
		if err != nil || string(v) != raw {
			t.Fatalf("expected valid lifecycle verdict %q, got %v (err: %v)", raw, v, err)
		}
	}
	if _, err := ParseLifecycleVerdict("UNKNOWN"); err == nil {
		t.Fatal("expected error on unknown lifecycle verdict")
	}

	// Parse deliberation
	for _, raw := range []string{"pass", "fail", "REFUTED"} {
		o, err := ParseDeliberationOutcome(raw)
		if err != nil || string(o) != raw {
			t.Fatalf("expected valid deliberation outcome %q, got %v (err: %v)", raw, o, err)
		}
	}
	if _, err := ParseDeliberationOutcome("invalid"); err == nil {
		t.Fatal("expected error on unknown deliberation outcome")
	}

	// Parse disconfirmation finding
	for _, raw := range []string{"SURVIVES", "FALSIFIED", "INCONCLUSIVE"} {
		f, err := ParseDisconfirmationFinding(raw)
		if err != nil || string(f) != raw {
			t.Fatalf("expected valid disconfirmation finding %q, got %v (err: %v)", raw, f, err)
		}
	}
	if _, err := ParseDisconfirmationFinding("invalid"); err == nil {
		t.Fatal("expected error on unknown disconfirmation finding")
	}

	// Mappings from Python disconfirmation model
	falsified := FindingFromPythonBool(true, false, 0)
	if falsified != FindingFalsified {
		t.Fatalf("expected FALSIFIED, got %v", falsified)
	}
	if DeliberationFromFinding(falsified) != OutcomeRefuted {
		t.Fatalf("falsified must map to REFUTED, got %v", DeliberationFromFinding(falsified))
	}
	if LifecycleFromFinding(falsified) != LifecycleAudited {
		t.Fatalf("falsified must map to AUDITED, got %v", LifecycleFromFinding(falsified))
	}

	shortcut := FindingFromPythonBool(false, true, 0)
	if shortcut != FindingFalsified {
		t.Fatalf("shortcut must map to FALSIFIED, got %v", shortcut)
	}

	inconclusive := FindingFromPythonBool(false, false, 2)
	if inconclusive != FindingInconclusive {
		t.Fatalf("defects must map to INCONCLUSIVE, got %v", inconclusive)
	}
	if DeliberationFromFinding(inconclusive) != OutcomeFail {
		t.Fatalf("inconclusive must map to fail, got %v", DeliberationFromFinding(inconclusive))
	}

	survives := FindingFromPythonBool(false, false, 0)
	if survives != FindingSurvives {
		t.Fatalf("clean finding must SURVIVE, got %v", survives)
	}
	if DeliberationFromFinding(survives) != OutcomePass {
		t.Fatalf("survives must map to pass, got %v", DeliberationFromFinding(survives))
	}
	if LifecycleFromFinding(survives) != LifecycleCollapsed {
		t.Fatalf("survives must collapse, got %v", LifecycleFromFinding(survives))
	}
}
