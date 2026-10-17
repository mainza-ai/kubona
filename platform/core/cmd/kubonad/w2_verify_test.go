package main

import (
	"testing"
)

// W2.20: verify refuses relative ledger paths — verifying whatever the CWD
// happens to hold proves nothing about the intended ledger.
func TestW220VerifyRequiresAbsolute(t *testing.T) {
	if code := runVerify([]string{"--db", "relative/kubona.db"}); code == 0 {
		t.Error("relative --db must refuse (non-zero exit)")
	}
}
