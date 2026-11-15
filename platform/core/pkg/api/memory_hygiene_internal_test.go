package api

// B10 internal pin: automatic turn write-back screens secrets too, not only
// the explicit retain endpoint.

import (
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/memory"
)

func TestTurnWritebackRedactsSecrets(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/memory_hygiene.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("memory redaction", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	const leaked = "ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	srv.retainTurnExperience(sess.ID, "chat", "deploy with "+leaked, "answered", nil, 0.6)
	recalled := srv.memoryStore.SemanticRecallScoped(memory.BankExperiences, "deploy", 1000, 0, sess.TenantID, []string{sess.TenantID})
	if len(recalled) != 1 {
		t.Fatalf("one experience expected, got %d", len(recalled))
	}
	content := recalled[0].Item.Content
	if strings.Contains(content, leaked) {
		t.Fatalf("secret survived automatic retention: %q", content)
	}
	if !strings.Contains(content, "[redacted:github_token]") {
		t.Fatalf("redaction marker missing: %q", content)
	}
}
