package jobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateSweepParams(t *testing.T) {
	ok := `{"molecule":"H2","r_min":0.5,"r_max":1.5,"steps":5}`
	if _, err := ValidateSweepParams([]byte(ok)); err != nil {
		t.Errorf("valid grid refused: %v", err)
	}
	cases := []struct {
		name string
		json string
		want string
	}{
		{"missing molecule", `{"r_min":0.5,"r_max":1.5,"steps":5}`, "requires molecule"},
		{"one step is an anchor", `{"molecule":"H2","r_min":0.5,"r_max":1.5,"steps":1}`, "steps must be"},
		{"too many steps", `{"molecule":"H2","r_min":0.5,"r_max":1.5,"steps":13}`, "steps must be"},
		{"inverted range", `{"molecule":"H2","r_min":1.5,"r_max":0.5,"steps":5}`, "r_min < r_max"},
		{"range too wide", `{"molecule":"H2","r_min":0.1,"r_max":1.5,"steps":5}`, "r_min < r_max"},
		{"not json", `nope`, "invalid pes_sweep params"},
	}
	for _, tc := range cases {
		if _, err := ValidateSweepParams([]byte(tc.json)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}

// The worker runs the grid point by point: independent geometries, one
// stored result. fakeBridge anchors everything, so all points succeed.
func TestPesSweepWorkerSettles(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	j, err := r.SubmitJob("sess-1", "pes_sweep", map[string]any{
		"molecule": "H2", "r_min": 0.5, "r_max": 1.5, "steps": 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status.String() != "JOB_STATUS_COMPLETED" {
		t.Fatalf("want COMPLETED, got %v", settled.Status)
	}
	var res PesSweepResult
	if err := json.Unmarshal(settled.ResultJSON, &res); err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Molecule != "H2" || len(res.Points) != 3 {
		t.Errorf("result wrong: %+v", res)
	}
	if res.Points[0].R != 0.5 || res.Points[2].R != 1.5 || res.Points[1].R != 1.0 {
		t.Errorf("grid must span inclusive endpoints: %+v", res.Points)
	}
}

// Out-of-range scans refuse pre-admission with no residue, same rule as
// malformed QASM.
func TestPesSweepRefusesWithoutResidue(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	_, err := r.SubmitJob("sess-1", "pes_sweep", map[string]any{
		"molecule": "H2", "r_min": 0.5, "r_max": 1.5, "steps": 1,
	})
	if err == nil || !strings.Contains(err.Error(), "steps must be") {
		t.Fatalf("expected grid refusal, got %v", err)
	}
	listed, lerr := r.ListJobs("sess-1", 50)
	if lerr != nil {
		t.Fatal(lerr)
	}
	for _, j := range listed {
		if j.JobType == "pes_sweep" {
			t.Error("refused sweep must leave no row")
		}
	}
}

// No bridge means no computation: the sweep fails honestly instead of
// returning canned points.
func TestPesSweepWithoutBridgeFails(t *testing.T) {
	r, cleanup := setupRunner(t, nil)
	defer cleanup()

	j, err := r.SubmitJob("sess-1", "pes_sweep", map[string]any{
		"molecule": "H2", "r_min": 0.5, "r_max": 1.5, "steps": 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status.String() != "JOB_STATUS_FAILED" {
		t.Errorf("bridgeless sweep must fail, got %v", settled.Status)
	}
}
