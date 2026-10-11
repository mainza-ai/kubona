package api

import (
	"errors"
	"strings"
	"testing"
)

var errTest = errors.New("boom")

// Golden fixture: the actual leaked reply stored 2026-10-11 (934 chars,
// truncated mid-sentence by the 250-token clamp, zero answer delivered).
const leakedReplyFixture = `We need answer user: "Map the H2 ground state potential energy surface across bond distances from 0.5 to 2.0 Angstrom". Need likely provide scientific reasoning and if wants simulate/compute molecule/circuit, MUST provide AP2 Proposal JSON block at very end enclosed in json_proposal. Need real valid OPENQASM 2.0 circuits and real classical anchors. Need maybe map PES. We can provide table of H2 ground state energies vs R. Need choose method? H2 ground state potential energy surface. Could use PySCF STO-3G FCI classical anchor? But mapping across distances: need values. We need be accurate. We can compute approximate? Need real classical anchors. We can state using PySCF STO-3G FCI (or maybe exact FCI in minimal basis) energies. Need values from 0.5 to 2.0 A. Need maybe include equilibrium ~0.7414 A, energy -1.13727 Ha? Wait PySCF STO-3G FCI H2 at R=0.7414? Let's recall. H2 minimal basis (STO-3G) FCI energy at equilibrium`

func TestSanitizeRemovesLeak(t *testing.T) {
	out := sanitizeCopilotReply(leakedReplyFixture)
	for _, marker := range []string{"json_proposal", "MUST", "We need answer user", "Need likely", "We can provide table"} {
		if strings.Contains(out, marker) {
			t.Errorf("leak marker survives sanitation: %q\noutput:\n%s", marker, out)
		}
	}
}

func TestSanitizeKeepsGoodAnswer(t *testing.T) {
	good := "The H2 ground state at 0.7414 Angstrom is -1.13727 Ha (PySCF STO-3G FCI). " +
		"Stretching the bond raises the energy toward dissociation."
	out := sanitizeCopilotReply(good)
	if !strings.Contains(out, "-1.13727 Ha") || !strings.Contains(out, "dissociation") {
		t.Errorf("good answer mangled:\n%s", out)
	}
}

func TestSanitizeExcisesProposalBlock(t *testing.T) {
	raw := "Here is the run plan.\n```json_proposal\n{\"target\": \"H2\"}\n```\nLaunch when ready."
	out := sanitizeCopilotReply(raw)
	if strings.Contains(out, "json_proposal") || strings.Contains(out, `"target"`) {
		t.Errorf("proposal block survives in display text:\n%s", out)
	}
	if !strings.Contains(out, "Here is the run plan.") || !strings.Contains(out, "Launch when ready.") {
		t.Errorf("surrounding prose lost:\n%s", out)
	}
}

func TestIsTruncated(t *testing.T) {
	if !isTruncated("stop", "length") {
		t.Error("expected length finish to report truncated")
	}
	if isTruncated("stop") {
		t.Error("stop finish must not report truncated")
	}
}

func TestEngineNoticeNamesProvider(t *testing.T) {	err := engineNotice("opencode", "deepseek-v4.1-flash", errTest)
	if strings.Contains(err, "oMLX") {
		t.Errorf("cloud failure mislabeled as oMLX:\n%s", err)
	}
	for _, want := range []string{"opencode", "deepseek-v4.1-flash", "balance"} {
		if !strings.Contains(err, want) {
			t.Errorf("notice missing %q:\n%s", want, err)
		}
	}
	local := engineNotice("omlx", "router", errTest)
	if !strings.Contains(local, "running") {
		t.Errorf("local failure needs a daemon hint:\n%s", local)
	}
}

func TestExtractProposalShared(t *testing.T) {
	raw := "Plan first.\n```json_proposal\n{\"target\": \"H2\"}\n```\nDone."
	display, prop := extractProposal(raw)
	if prop == nil {
		t.Fatal("expected proposal payload")
	}
	if strings.Contains(display, "json_proposal") {
		t.Errorf("block survives in display:\n%s", display)
	}
	if !strings.Contains(display, "Plan first.") || !strings.Contains(display, "Done.") {
		t.Errorf("prose lost:\n%s", display)
	}
	_, none := extractProposal("Just prose, no block.")
	if none != nil {
		t.Error("expected nil proposal for block-free reply")
	}
}
