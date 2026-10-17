package ledger_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func openTestLedger(t *testing.T) (*ledger.Ledger, string) {
	t.Helper()
	f, err := os.CreateTemp("", "kubona-verify-*.db")
	if err != nil {
		t.Fatalf("temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	os.Remove(path) // OpenLedger creates; path must not exist yet
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { l.Close(); os.Remove(path) })
	return l, path
}

func TestVerifyChainClean(t *testing.T) {
	l, _ := openTestLedger(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = pub
	if err := l.AppendEvent("s1", "chat_message_appended", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AppendSignedEvent("s1", "chat_cleared", map[string]int{"n": 2}, priv); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent("s2", "chat_message_appended", "other-session"); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("")
	if err != nil {
		t.Fatal(err)
	}
	if len(faults) != 0 {
		t.Fatalf("clean chain must verify, got %+v", faults)
	}
	faults, err = l.VerifyChain("s1")
	if err != nil || len(faults) != 0 {
		t.Fatalf("session-scoped audit must pass, got %+v %v", faults, err)
	}
}

func TestVerifyChainDetectsTamper(t *testing.T) {
	l, _ := openTestLedger(t)
	if err := l.AppendEvent("s1", "chat_message_appended", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent("s1", "chat_cleared", map[string]int{"n": 2}); err != nil {
		t.Fatal(err)
	}
	// Tamper with the first payload behind the ledger's back.
	if _, err := l.DB().Exec(`UPDATE ledger_events SET payload = '{"n":999}' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(faults) == 0 {
		t.Fatal("tampered payload must fault")
	}
	foundHash := false
	for _, f := range faults {
		if f.Kind == "hash" {
			foundHash = true
		}
	}
	if !foundHash {
		t.Fatalf("tamper must surface as a hash fault, got %+v", faults)
	}
}

func TestVerifyChainDetectsBrokenLink(t *testing.T) {
	l, _ := openTestLedger(t)
	if err := l.AppendEvent("s1", "chat_message_appended", "x"); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent("s1", "chat_cleared", "y"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`UPDATE ledger_events SET prev_hash = 'ff' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	foundLink := false
	for _, f := range faults {
		if f.Kind == "link" {
			foundLink = true
		}
	}
	if !foundLink {
		t.Fatalf("broken prev_hash must surface as a link fault, got %+v", faults)
	}
}

func TestVerifyChainDetectsBadSignature(t *testing.T) {
	l, _ := openTestLedger(t)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := l.AppendSignedEvent("s1", "chat_cleared", "signed", priv); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`UPDATE ledger_events SET signature_hex = '00' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	foundSig := false
	for _, f := range faults {
		if f.Kind == "signature" {
			foundSig = true
		}
	}
	if !foundSig {
		t.Fatalf("corrupt signature must surface as a signature fault, got %+v", faults)
	}
}

func TestReadOnlyHandleReadsAndRefusesWrites(t *testing.T) {
	l, path := openTestLedger(t)
	if err := l.AppendEvent("s1", "chat_message_appended", "readable"); err != nil {
		t.Fatal(err)
	}
	ro, err := ledger.OpenLedgerReadOnly(path)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	defer ro.Close()
	evs, err := ro.GetRecentEvents("s1", 10)
	if err != nil || len(evs) != 1 {
		t.Fatalf("replica must read the writer's events, got %+v %v", evs, err)
	}
	faults, err := ro.VerifyChain("")
	if err != nil || len(faults) != 0 {
		t.Fatalf("replica must verify clean, got %+v %v", faults, err)
	}
	if err := ro.AppendEvent("s1", "write-attempt", "x"); err == nil {
		t.Fatal("writes through a read-only handle must fail")
	}
	if _, err := ledger.OpenLedgerReadOnly(path + ".missing"); err == nil {
		t.Fatal("read-only open of a missing file must fail, not create")
	}
}

// W1.12: unsigned commit-class events fault (rewrite-forgeable), and the
// audit re-anchors on recomputed digests (forged links never become anchors).
func TestVerifyChainFlagsUnsignedCommitEvents(t *testing.T) {
	l, _ := openTestLedger(t)
	if err := l.AppendEvent("s1", "chat_message_appended", "plain"); err != nil {
		t.Fatal(err)
	}
	// Unsigned tribunal_commit: linkage fine, authority absent.
	if err := l.AppendEvent("s1", "tribunal_commit", map[string]string{"v": "x"}); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range faults {
		if f.Kind == "signature" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unsigned commit-class event must fault, got %+v", faults)
	}
}

// Tamper extent: a rewritten payload faults at its link AND the next link
// (which no longer matches true history) — then the chain re-anchors and
// later events verify.
func TestVerifyChainReanchorsOnTruth(t *testing.T) {
	l, _ := openTestLedger(t)
	for _, p := range []string{"a", "b", "c"} {
		if err := l.AppendEvent("s1", "chat_message_appended", p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.DB().Exec(`UPDATE ledger_events SET payload = '"forged"' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(faults) != 2 {
		t.Fatalf("one tamper must fault exactly its link + the next, got %+v", faults)
	}
	if faults[0].Kind != "hash" || faults[1].Kind != "link" {
		t.Fatalf("fault order must be hash-then-link, got %+v", faults)
	}
}
