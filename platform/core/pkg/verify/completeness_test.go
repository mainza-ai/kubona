package verify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCheckPublicationCompletenessFailClosedAndPassing(t *testing.T) {
	campBytes, _ := json.Marshal(map[string]any{
		"experiments": []map[string]any{
			{
				"job_id":         "job_hw_1",
				"kind":           "vqe_hardware",
				"hardware":       true,
				"seed":           7,
				"circuit_sha256": "abc123def456",
			},
		},
	})

	// Missing QASM and datasheet -> fail closed (Complete == false)
	bad := CheckPublicationCompleteness(string(campBytes), "{}", 3, 0, map[string]string{
		"main.pdf": "deadbeef",
	})
	if bad.Complete {
		t.Fatalf("expected incomplete report when QASM and datasheet are absent, got %+v", bad)
	}
	if len(bad.Blocking) < 2 {
		t.Fatalf("expected >=2 blocking findings (QASM + datasheet), got %+v", bad.Blocking)
	}

	// Deleting only the QASM file still fails closed with named missing job
	noQasm := CheckPublicationCompleteness(string(campBytes), "{}", 12, 12, map[string]string{
		"main.pdf":                    "deadbeef",
		"supplement/datasheet.json":   "111",
		"supplement/jobs.csv":         "222",
		"supplement/environment.json": "333",
	})
	if noQasm.Complete || !strings.Contains(strings.Join(noQasm.Blocking, " "), "job_hw_1") {
		t.Fatalf("expected blocking finding naming job_hw_1, got %+v", noQasm)
	}

	// Complete bundle passes
	ok := CheckPublicationCompleteness(string(campBytes), `{"_scholarly":{"affiliation":"Kubona","corresponding_author":"A <a@b.org>","competing_interests":"None","funding":"OSS","license":"CC-BY-4.0"}}`, 12, 12, map[string]string{
		"main.pdf":                      "deadbeef",
		"supplement/qasm/job_hw_1.qasm": "abc123def456",
		"supplement/datasheet.json":     "111",
		"supplement/jobs.csv":           "222",
		"supplement/environment.json":   "333",
	})
	if !ok.Complete || ok.HardwareCircuitsBundled != 1 || ok.SupplementFiles != 4 {
		t.Fatalf("expected complete report, got %+v", ok)
	}
	if len(ok.Warnings) != 0 {
		t.Fatalf("expected 0 warnings with full scholarly metadata, got %+v", ok.Warnings)
	}

	// R6: CheckScholarlyMetadata flags missing and placeholder fields
	missEmpty := CheckScholarlyMetadata("{}")
	if len(missEmpty) != 5 {
		t.Fatalf("expected 5 missing fields for empty evidence, got %+v", missEmpty)
	}
	missPlaceholder := CheckScholarlyMetadata(`{"_scholarly":{"affiliation":"TBD","corresponding_author":"A","competing_interests":"None","funding":"OSS","license":"CC-BY-4.0"}}`)
	if len(missPlaceholder) != 1 || missPlaceholder[0] != "affiliation" {
		t.Fatalf("expected placeholder affiliation flagged, got %+v", missPlaceholder)
	}
}

func TestCompletenessSkipsFailedHardwareAndGatesEveryPESPoint(t *testing.T) {
	campBytes, _ := json.Marshal(map[string]any{
		"experiments": []map[string]any{
			{
				"job_id":   "job_failed",
				"kind":     "hardware_experiment",
				"hardware": false,
				"status":   "failed",
			},
			{
				"job_id":      "job_pes",
				"kind":        "hardware_pes",
				"hardware":    true,
				"seed":        7,
				"qasm_points": 2,
			},
		},
	})
	base := map[string]string{
		"supplement/datasheet.json":   "111",
		"supplement/jobs.csv":         "222",
		"supplement/environment.json": "333",
	}
	partial := map[string]string{
		"supplement/qasm/job_pes_p0.qasm": "aaa",
	}
	for k, v := range base {
		partial[k] = v
	}
	rep := CheckPublicationCompleteness(string(campBytes), "{}", 0, 0, partial)
	if rep.Complete || rep.HardwareJobs != 1 {
		t.Fatalf("expected one settled hardware job and an incomplete report, got %+v", rep)
	}
	if !strings.Contains(strings.Join(rep.Blocking, " "), "job_pes") {
		t.Fatalf("expected the missing second PES point to block, got %+v", rep.Blocking)
	}

	full := map[string]string{
		"supplement/qasm/job_pes_p0.qasm": "aaa",
		"supplement/qasm/job_pes_p1.qasm": "bbb",
	}
	for k, v := range base {
		full[k] = v
	}
	done := CheckPublicationCompleteness(string(campBytes), `{"_scholarly":{"x":"y"}}`, 1, 12, full)
	if !done.Complete || done.HardwareCircuitsBundled != 1 {
		t.Fatalf("expected complete report once both point circuits are bundled, got %+v", done)
	}
	for _, c := range done.Checks {
		if c.ID == "references_count" && !c.Passed {
			t.Fatalf("printed bibliography count of 12 should satisfy references_count, got %+v", c)
		}
	}
}
