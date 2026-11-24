package verify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExtractNumbersTargetsMeasurements(t *testing.T) {
	text := "The energy was -1.1373 Ha at 0.7414 with 4096 shots, 12.5% error, in 2024; see 6-31G and 2 layers."
	got := ExtractNumbers(text)
	if len(got) != 4 {
		t.Fatalf("extracted = %d: %+v", len(got), got)
	}
	values := map[float64]string{}
	for _, n := range got {
		values[n.Value] = n.Unit
	}
	for _, want := range []float64{-1.1373, 0.7414, 4096, 12.5} {
		if _, ok := values[want]; !ok {
			t.Fatalf("missing %v in %+v", want, values)
		}
	}
	if _, ok := values[2024]; ok {
		t.Fatal("a year was treated as a measurement claim")
	}
}

func TestExtractNumbersKeepsSignsInTables(t *testing.T) {
	got := ExtractNumbers("Row & -1.132769 & -1.137270 & 4.5007 \\\\")
	if len(got) != 3 {
		t.Fatalf("extracted = %+v", got)
	}
	if got[0].Value != -1.132769 || got[1].Value != -1.13727 || got[2].Value != 4.5007 {
		t.Fatalf("signs lost: %+v", got)
	}
	// A negative value must match a negative record, not its magnitude.
	ev := Evidence{Documents: []Document{{ID: "job", JSON: `{"energy": -1.132769}`}}}
	if f := VerifyNumbers("Row & -1.132769 &", ev); f[0].Status != NumberMatched {
		t.Fatalf("negative value = %+v", f)
	}
	if f := VerifyNumbers("Row & 1.132769 &", ev); f[0].Status != NumberUnmatched {
		t.Fatalf("positive magnitude should not match a negative record: %+v", f)
	}
}

func TestVerifyNumbersRoundsWithinWrittenPrecision(t *testing.T) {
	ev := Evidence{Documents: []Document{
		{ID: "job-1", JSON: `{"energy": -1.1372701746609035, "shots": 4096}`},
	}}
	got := VerifyNumbers("Measured -1.1373 Ha with 4096 shots.", ev)
	if len(got) != 2 {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Status != NumberMatched || got[0].MatchedPath != "energy" {
		t.Fatalf("rounded value not matched: %+v", got[0])
	}
	if got[1].Status != NumberMatched || got[1].MatchedValue == nil || *got[1].MatchedValue != 4096 {
		t.Fatalf("shots not matched: %+v", got[1])
	}

	fab := VerifyNumbers("Measured -1.9000 Ha.", ev)
	if len(fab) != 1 || fab[0].Status != NumberUnmatched {
		t.Fatalf("fabricated value = %+v", fab)
	}
}

func TestVerifyNumbersUnverifiableWithoutEvidence(t *testing.T) {
	got := VerifyNumbers("Measured -1.1373 Ha.", Evidence{})
	if len(got) != 1 || got[0].Status != NumberUnverifiable {
		t.Fatalf("findings = %+v", got)
	}
	// A document with no numeric leaves is still no evidence.
	got = VerifyNumbers("Measured -1.1373 Ha.", Evidence{Documents: []Document{{ID: "x", JSON: `{"ok": true}`}}})
	if got[0].Status != NumberUnverifiable {
		t.Fatalf("non-numeric evidence = %+v", got[0])
	}
}

type stubResolver struct {
	works []ResolvedWork
	err   error
}

func (s stubResolver) Resolve(context.Context, string) ([]ResolvedWork, error) {
	return s.works, s.err
}

func TestVerifyCitationsClassifiesEveryCase(t *testing.T) {
	resolver := stubResolver{works: []ResolvedWork{
		{ID: "https://doi.org/10.1/abc", Title: "Hardware-efficient ansatz for H2 ground states"},
	}}
	cases := []struct {
		c    Citation
		want string
	}{
		{Citation{Key: "doi", Title: "Hardware-efficient ansatz for H2", DOI: "10.1/ABC"}, CitationVerifiedDOI},
		{Citation{Key: "wrong", Title: "Hardware-efficient ansatz for H2", DOI: "10.1/other"}, CitationDOIMismatch},
		{Citation{Key: "title", Title: "Hardware-efficient ansatz for H2"}, CitationVerifiedTitle},
		{Citation{Key: "none"}, CitationMissingIdentify},
	}
	for _, tc := range cases {
		got := VerifyCitations(context.Background(), []Citation{tc.c}, resolver)
		if len(got) != 1 || got[0].Status != tc.want {
			t.Fatalf("%s: %+v, want %s", tc.c.Key, got, tc.want)
		}
	}

	down := VerifyCitations(context.Background(), []Citation{
		{Key: "x", Title: "Hardware-efficient ansatz", DOI: "10.1/abc"},
	}, stubResolver{err: errors.New("offline")})
	if len(down) != 1 || down[0].Status != CitationUnverified || down[0].Error == "" {
		t.Fatalf("resolver failure = %+v", down)
	}
	if got := VerifyCitations(context.Background(), []Citation{{Key: "x", Title: "T"}}, nil); got[0].Status != CitationUnverified {
		t.Fatalf("nil resolver = %+v", got)
	}
}

func TestNormalizeDOI(t *testing.T) {
	for _, in := range []string{"https://doi.org/10.1/ABC", "http://dx.doi.org/10.1/abc", "doi:10.1/abc", " 10.1/abc "} {
		if got := NormalizeDOI(in); got != "10.1/abc" {
			t.Fatalf("NormalizeDOI(%q) = %q", in, got)
		}
	}
}

func TestBuildReportStatusAndBlockingDetails(t *testing.T) {
	nums := []NumericFinding{
		{Text: "1.0", Value: 1, Status: NumberMatched},
		{Text: "9.9", Value: 9.9, Status: NumberUnmatched},
		{Text: "2.0", Value: 2, Status: NumberUnverifiable},
	}
	cites := []CitationFinding{
		{Key: "dead", Status: CitationDOIMismatch},
		{Key: "ok", Status: CitationVerifiedDOI},
	}
	rep := BuildReport("hash", nums, cites, time.Now())
	if rep.Summary.Status != "blocked" || rep.Summary.Blocking != 2 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	details := rep.BlockingDetails()
	if len(details) != 2 {
		t.Fatalf("details = %+v", details)
	}

	ok := BuildReport("hash", nums[:1], cites[1:], time.Now())
	if ok.Summary.Status != "verified" || ok.Summary.Blocking != 0 {
		t.Fatalf("verified report = %+v", ok.Summary)
	}
	un := BuildReport("hash", nums[2:], nil, time.Now())
	if un.Summary.Status != "unverified" || un.Summary.Unverified != 1 {
		t.Fatalf("unverified report = %+v", un.Summary)
	}
}

func TestShippedCalibrationAgreesWithItsLabels(t *testing.T) {
	rep, err := Calibrate()
	if err != nil {
		t.Fatalf("calibrate: %v", err)
	}
	if rep.Cases < 10 {
		t.Fatalf("calibration set is too small: %d", rep.Cases)
	}
	if rep.Accuracy != 1.0 {
		t.Fatalf("accuracy = %.3f (%d/%d): %+v", rep.Accuracy, rep.Agreed, rep.Cases, rep.FailureList)
	}
	if !strings.Contains(rep.Disclosure, "not an independent human annotation") {
		t.Fatalf("disclosure missing: %q", rep.Disclosure)
	}
	if !strings.Contains(rep.Labeler, "maintainer") {
		t.Fatalf("labeler missing: %q", rep.Labeler)
	}
}

func TestVerifyNumbersRejectsTrajectoryThetaCoincidenceAndSupportsDerivedDeltas(t *testing.T) {
	// D-02: trajectory[0].theta[0] = 0.00100568 (radians) must NOT certify
	// a fabricated 0.0010 mHa energy delta claim.
	// D-05: honest derived |energy - fci_exact_energy| (1.202 mHa) MUST verify.
	ev := Evidence{
		Documents: []Document{{
			ID: "job-exp1",
			JSON: `{
				"energy": -1.138508,
				"fci_exact_energy": -1.137306,
				"trajectory": [{"step": 58, "theta": [0.00100568, 0.123456]}]
			}`,
		}},
	}
	findings := VerifyNumbers("Measured -1.138508 Ha (discrepancy 1.202 mHa; bogus 0.0010 mHa).", ev)
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d: %+v", len(findings), findings)
	}
	if findings[0].Status != NumberMatched {
		t.Fatalf("measured energy must match, got %+v", findings[0])
	}
	if findings[1].Status != NumberMatched {
		t.Fatalf("derived delta 1.202 mHa must match, got %+v", findings[1])
	}
	if findings[2].Status != NumberUnmatched {
		t.Fatalf("D-02 regression: 0.0010 mHa must NOT match trajectory theta radians, got %+v", findings[2])
	}
}

func TestVerifyNumbersScrubsDOIsAndIndexesBeyond5000Leaves(t *testing.T) {
	// W4-04: DOI strings (10.1038/s41586-023-06096-3) must not be flagged as
	// unmatched measurements, and large 60-step lattice arrays must not crowd out
	// spin_chain_hardware or excited_state derived deltas.
	bigTimes := make([]string, 60)
	for i := range bigTimes {
		bigTimes[i] = `{"t":0.1,"loschmidt_echo":0.5,"zz_correlator_mean":0.4,"m0":0.1,"m1":0.2,"m2":0.3,"m3":0.4,"m4":0.5,"m5":0.6,"m6":0.7,"m7":0.8}`
	}
	latticeDoc := `{"kind":"lattice_dynamics","times":[` + strings.Join(bigTimes, ",") + `]}`
	docs := []Document{}
	for i := 0; i < 10; i++ {
		docs = append(docs, Document{ID: "lattice-" + string(rune('0'+i)), JSON: latticeDoc})
	}
	docs = append(docs,
		Document{ID: "spin-hw", JSON: `{"kind":"spin_chain_hardware","energy":-3.611328,"energy_exact":-5.226252}`},
		Document{ID: "exc-1", JSON: `{"kind":"excited_state","ground_energy":-7.836872,"fci_exact_energy":-7.882402}`},
	)
	prose := `Prior work \href{https://doi.org/10.1038/s41586-023-06096-3}{doi:10.1038/s41586-023-06096-3} ` +
		`and 10.1038/nature23879. Spin chain measured -3.611328 Ha (1614.924 mHa). ` +
		`Excited state ground delta 45.530 mHa.`
	findings := VerifyNumbers(prose, Evidence{Documents: docs})
	if len(findings) != 3 {
		t.Fatalf("expected exactly 3 measurement findings (DOIs scrubbed), got %d: %+v", len(findings), findings)
	}
	for _, f := range findings {
		if f.Status != NumberMatched {
			t.Fatalf("expected all 3 findings matched, got %+v", f)
		}
	}
	// W5-01: derived mean(zz_correlator_mean) and min(loschmidt_echo) over
	// lattice_dynamics times[] must verify cleanly.
	latOnly := Evidence{Documents: []Document{{
		ID:   "lat-60",
		JSON: `{"kind":"lattice_dynamics","times":[{"t":0.0,"loschmidt_echo":1.0,"zz_correlator_mean":0.8},{"t":1.0,"loschmidt_echo":0.125,"zz_correlator_mean":0.136}]}`,
	}}}
	latFindings := VerifyNumbers("Quench reached min L=0.125 and mean ZZ=0.468.", latOnly)
	if len(latFindings) != 2 || latFindings[0].Status != NumberMatched || latFindings[1].Status != NumberMatched {
		t.Fatalf("W5-01 expected derived lattice min/mean matched, got %+v", latFindings)
	}
}
