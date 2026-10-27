package autoresearch

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

type fakeSelfReported struct{ measuredHarness }

func (fakeSelfReported) ID() string         { return "fake-self-reported" }
func (fakeSelfReported) SelfReported() bool { return true }

type measuredHarness struct{}

func (measuredHarness) ID() string                          { return "measured" }
func (measuredHarness) FixedSetup() string                  { return "" }
func (measuredHarness) MutableArtifact() string             { return "" }
func (measuredHarness) Metric(string) (float64, error)      { return 0, nil }
func (measuredHarness) ValidityCheck(string) (bool, string) { return true, "" }
func (measuredHarness) Evaluate(*sandbox.SandboxRunner, string) (*HarnessResult, error) {
	return &HarnessResult{HarnessID: "measured", Valid: true}, nil
}

func TestSelfReportedHarnessRefusedInProduction(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_AUTORESEARCH_ALLOW_SELF_REPORTED", "")
	if err := refuseSelfReportedInProduction(fakeSelfReported{}); err == nil {
		t.Fatal("self-reported harness must refuse in production")
	}
	// The ansatz and QAOA harnesses are measured (artifact stage separate
	// from the trusted evaluator), so they are allowed even in production.
	if err := refuseSelfReportedInProduction(&AnsatzSearchHarness{}); err != nil {
		t.Fatalf("measured ansatz harness must not be blocked: %v", err)
	}
	if err := refuseSelfReportedInProduction(&QAOACombinatorialHarness{}); err != nil {
		t.Fatalf("measured QAOA harness must not be blocked: %v", err)
	}
	t.Setenv("KUBONA_AUTORESEARCH_ALLOW_SELF_REPORTED", "1")
	if err := refuseSelfReportedInProduction(fakeSelfReported{}); err != nil {
		t.Fatalf("explicit research opt-in must allow: %v", err)
	}
	if err := refuseSelfReportedInProduction(measuredHarness{}); err != nil {
		t.Fatalf("measured harness must not be blocked: %v", err)
	}
}

func TestSelfReportedHarnessAllowedInDev(t *testing.T) {
	t.Setenv("KUBONA_ENV", "")
	if err := refuseSelfReportedInProduction(fakeSelfReported{}); err != nil {
		t.Fatalf("dev must keep self-reported harnesses available: %v", err)
	}
}
