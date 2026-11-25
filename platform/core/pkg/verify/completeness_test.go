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
	bad := CheckPublicationCompleteness(string(campBytes), "{}", 3, map[string]string{
		"main.pdf": "deadbeef",
	})
	if bad.Complete {
		t.Fatalf("expected incomplete report when QASM and datasheet are absent, got %+v", bad)
	}
	if len(bad.Blocking) < 2 {
		t.Fatalf("expected >=2 blocking findings (QASM + datasheet), got %+v", bad.Blocking)
	}

	// Deleting only the QASM file still fails closed with named missing job
	noQasm := CheckPublicationCompleteness(string(campBytes), "{}", 12, map[string]string{
		"main.pdf":                    "deadbeef",
		"supplement/datasheet.json":   "111",
		"supplement/jobs.csv":         "222",
		"supplement/environment.json": "333",
	})
	if noQasm.Complete || !strings.Contains(strings.Join(noQasm.Blocking, " "), "job_hw_1") {
		t.Fatalf("expected blocking finding naming job_hw_1, got %+v", noQasm)
	}

	// Complete bundle passes
	ok := CheckPublicationCompleteness(string(campBytes), `{"_scholarly":{"affiliation":"Kubona"}}`, 12, map[string]string{
		"main.pdf":                      "deadbeef",
		"supplement/qasm/job_hw_1.qasm": "abc123def456",
		"supplement/datasheet.json":     "111",
		"supplement/jobs.csv":           "222",
		"supplement/environment.json":   "333",
	})
	if !ok.Complete || ok.HardwareCircuitsBundled != 1 || ok.SupplementFiles != 4 {
		t.Fatalf("expected complete report, got %+v", ok)
	}
}
