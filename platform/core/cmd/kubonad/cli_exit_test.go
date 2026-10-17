package main

// R5 CLI exit contract: 0 = success (verify clean counts as success;
// partial results still exit 0 — the receipt says what completed),
// 1 = incomplete (faults found, operational failure), 2 = usage error,
// 128+N = signals (Go runtime default). Change deliberately: scripts
// and the release gate branch on these codes.

import "testing"

func TestCLIExitContract(t *testing.T) {
	if got := runGrants(nil); got != 2 {
		t.Fatalf("grants without subcommand must exit 2 (usage), got %d", got)
	}
	if got := runGrants([]string{"bogus"}); got != 2 {
		t.Fatalf("grants unknown subcommand must exit 2 (usage), got %d", got)
	}
	if got := runVerify([]string{"--db", "relative/path.db"}); got != 1 {
		t.Fatalf("verify relative --db must exit 1 (incomplete), got %d", got)
	}
	if got := runVerify([]string{"--nope"}); got != 2 {
		t.Fatalf("verify bad flag must exit 2 (usage), got %d", got)
	}
}
