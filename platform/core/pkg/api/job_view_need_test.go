package api

// C2 projection pin: the HTTP job view carries the persisted typed need, and
// an uncatalogued kind is treated as absent so the console never receives a
// remedy it cannot render.

import (
	"encoding/json"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
)

func TestJobViewProjectsTypedNeed(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"error": "RunExperiment RPC failed: rpc error: code = Unavailable",
		"need": map[string]any{
			"kind":   jobs.NeedBackendOffline,
			"target": "ibm_fez",
			"reason": "bridge unreachable",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := jobViewFrom(&jobs.Job{ID: "j1", JobType: "circuit_run", ResultJSON: raw})
	if view == nil || view.Need == nil {
		t.Fatalf("need must project: %+v", view)
	}
	if view.Need.Kind != jobs.NeedBackendOffline || view.Need.Target != "ibm_fez" {
		t.Fatalf("unexpected need: %+v", view.Need)
	}
	// Marshal/unmarshal like the wire does: the field must be named `need`.
	wire, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["need"]; !ok {
		t.Fatalf("wire view must expose `need`: %s", wire)
	}
}

func TestJobViewOmitsUncataloguedNeed(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"error": "something failed",
		"need":  map[string]any{"kind": "feel_better", "reason": "made up"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := jobViewFrom(&jobs.Job{ID: "j2", JobType: "circuit_run", ResultJSON: raw})
	if view.Need != nil {
		t.Fatalf("uncatalogued kind must be omitted, got %+v", view.Need)
	}

	// Malformed result JSON must not panic or invent a need.
	view = jobViewFrom(&jobs.Job{ID: "j3", JobType: "circuit_run", ResultJSON: json.RawMessage("{not json")})
	if view.Need != nil {
		t.Fatalf("malformed result must omit the need, got %+v", view.Need)
	}
}
