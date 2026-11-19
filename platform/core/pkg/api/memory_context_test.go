package api

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

func TestRecallContextForTurnInjectsBoundedMemory(t *testing.T) {
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sess, err := sm.CreateSession("ctx", 1, "default")
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewPersistentHindsightStore(l.DB())
	s := &Server{sessionMgr: sm, memoryStore: store}
	text := "the alpha campaign used ZNE with three scale factors"
	store.RetainScoped("default", "default", memory.BankExperiences, text, 0.9, nil)

	out := s.recallContextForTurn(sess.ID, text)
	if len(out) != 1 || out[0].Role != "system" {
		t.Fatalf("recall context=%+v want one system message", out)
	}
	if !strings.Contains(out[0].Content, "alpha campaign") {
		t.Fatalf("memory missing from context: %q", out[0].Content)
	}
	merged := withRecalledMemory([]llm.ChatMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}, out)
	if len(merged) != 3 || merged[1].Role != "system" || merged[2].Role != "user" {
		t.Fatalf("merge order=%+v", merged)
	}

	t.Setenv("KUBONA_AUTO_RECALL", "0")
	if got := s.recallContextForTurn(sess.ID, text); len(got) != 0 {
		t.Fatalf("auto recall opt-out ignored: %+v", got)
	}
}
