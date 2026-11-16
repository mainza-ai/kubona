package api

import (
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestProposalDismissalContextConsumesReasons(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/proposal-feedback.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	const sid = "sess-proposal-feedback"
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, 'p', 'default', ?, ?, 'active', 0, 0)`, sid, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent(sid, "proposal_dismissed", map[string]any{"proposal_id": "pr_1", "reason": "too expensive for the question"}); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent(sid, "proposal_dismissed", map[string]any{"proposal_id": "pr_2", "reason": "wrong backend"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{ledger: l}
	ctx := s.proposalDismissalContext(sid)
	if !strings.Contains(ctx, "too expensive for the question") || !strings.Contains(ctx, "wrong backend") {
		t.Fatalf("dismissal reasons missing from prompt context: %q", ctx)
	}
	if strings.Contains(ctx, "score") {
		t.Fatalf("context must not invent a ranking score: %q", ctx)
	}
}
