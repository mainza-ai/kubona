package api

// R2b settle tests: the hook closes receipt obligations on completion,
// skips cleanly without one, and forks anomaly follow-ups on
// post-closure settlements (closed history never rewritten).

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func mustOpenSettleServer(t *testing.T) (*Server, *ledger.Ledger) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/mandsettle.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return &Server{ledger: l}, l
}

func openFundedMandate(t *testing.T, l *ledger.Ledger, ref string) string {
	t.Helper()
	ob, err := l.OpenObligation("mandate", ref, []string{"settlement"}, []string{"ledger-receipt"}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.SetObligationOwner(ob.ID, "admin:abc"); err != nil {
		t.Fatalf("owner: %v", err)
	}
	return ob.ID
}

func TestRecordMandateSettlementCloses(t *testing.T) {
	s, l := mustOpenSettleServer(t)
	obID := openFundedMandate(t, l, "entryhash-1")
	s.recordMandateSettlement(&jobs.Job{ID: "job-1", MandateRef: "entryhash-1"})
	ob, _ := l.GetObligation(obID)
	if ob.Disposition != "closed" {
		t.Fatalf("settlement must close the obligation, got %+v", ob)
	}
	cases, _ := l.ListObligationCases(obID)
	found := false
	for _, c := range cases {
		if c.CaseKind == "settlement" && c.Verdict == "pass" && c.Authority == "ledger-receipt" && c.Evidence == "job-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("settlement case must be on record: %+v", cases)
	}
}

func TestRecordMandateSettlementSkips(t *testing.T) {
	s, _ := mustOpenSettleServer(t)
	s.recordMandateSettlement(nil)
	s.recordMandateSettlement(&jobs.Job{ID: "job-x"})
	s.recordMandateSettlement(&jobs.Job{ID: "job-y", MandateRef: "no-such-receipt"})
	// No obligation, no error, no panic — pre-obligation history and
	// unmandated jobs pass through silently.
}

func TestRecordMandateSettlementPostClosureForks(t *testing.T) {
	s, l := mustOpenSettleServer(t)
	obID := openFundedMandate(t, l, "entryhash-2")
	s.recordMandateSettlement(&jobs.Job{ID: "job-1", MandateRef: "entryhash-2"})
	// Second completion against the same receipt: closed history refuses
	// the case → anomaly follow-up (ownerless, depended on the original).
	s.recordMandateSettlement(&jobs.Job{ID: "job-2", MandateRef: "entryhash-2"})
	follow, err := l.GetObligationBySubject("mandate-settlement", "entryhash-2/job/job-2")
	if err != nil || follow == nil {
		t.Fatalf("post-closure settlement must fork a follow-up: %+v %v", follow, err)
	}
	if follow.Owner != "" {
		t.Fatal("anomaly follow-up stays ownerless (unclosable record, not a new promise)")
	}
	if len(follow.Dependencies) != 1 || follow.Dependencies[0] != obID {
		t.Fatalf("follow-up must depend on the original: %+v", follow)
	}
	closed, _, _ := agents.AttemptClose(l, follow.ID)
	if closed {
		t.Fatal("ownerless follow-up must never close")
	}
}
