package api_test

// C1 API pins: a launched job is a recorded run part, the projection upgrades
// its state from the job row, and parts are bound to the loop's session.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func partsGet(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func partsPost(t *testing.T, h http.Handler, path string, body any) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer op-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestLoopPartsProjectLaunchedJob(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "run parts")
	pid, digest := mintTestProposal(t, srv, sid, b1QASM, "local_simulator", 1.0)

	code, body := b1Launch(t, h, map[string]any{
		"session_id": sid, "proposal_id": pid, "digest": digest, "request_id": "parts-launch",
	})
	if code != http.StatusCreated {
		t.Fatalf("launch: %d %s", code, body)
	}
	var launched struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
		Loop struct {
			ID string `json:"id"`
		} `json:"loop"`
	}
	if err := json.Unmarshal([]byte(body), &launched); err != nil {
		t.Fatal(err)
	}
	if launched.Loop.ID == "" || launched.Job.ID == "" {
		t.Fatalf("launch must return job + loop: %s", body)
	}

	code, body = partsGet(t, h, "/api/loops/"+launched.Loop.ID+"/parts")
	if code != http.StatusOK {
		t.Fatalf("parts: %d %s", code, body)
	}
	var out struct {
		LoopID string `json:"loop_id"`
		Count  int    `json:"count"`
		Parts  []struct {
			Helper    string `json:"helper"`
			Title     string `json:"title"`
			Goal      string `json:"goal"`
			State     string `json:"state"`
			JobID     string `json:"job_id"`
			JobStatus string `json:"job_status"`
		} `json:"parts"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.LoopID != launched.Loop.ID || out.Count != 1 || len(out.Parts) != 1 {
		t.Fatalf("one quantum_job part expected: %s", body)
	}
	part := out.Parts[0]
	if part.Helper != "quantum_job" || part.JobID != launched.Job.ID {
		t.Fatalf("part must link the dispatched job: %+v", part)
	}
	if part.Goal != "H2" {
		t.Fatalf("part goal must come from the proposal target, got %q", part.Goal)
	}
	// No workers in tests: the job is queued, so the part is waiting — never
	// rendered as still running after the fact.
	if part.State != "waiting" || part.JobStatus != "queued" {
		t.Fatalf("state must come from the job row: %+v", part)
	}

	// Unknown loop ids refuse without an oracle.
	if code, _ := partsGet(t, h, "/api/loops/loop-missing/parts"); code != http.StatusNotFound {
		t.Fatalf("unknown loop must 404, got %d", code)
	}
}

func TestManualLoopRecordsRegisteredPart(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "manual run")

	code, body := partsPost(t, h, "/api/loops", map[string]any{"session_id": sid, "max_steps": 4})
	if code != http.StatusCreated {
		t.Fatalf("register loop: %d %s", code, body)
	}
	var loop struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &loop); err != nil {
		t.Fatal(err)
	}
	code, body = partsGet(t, h, "/api/loops/"+loop.ID+"/parts")
	if code != http.StatusOK {
		t.Fatalf("parts: %d %s", code, body)
	}
	if !strings.Contains(body, `"title":"Run registered"`) || !strings.Contains(body, `"state":"planned"`) {
		t.Fatalf("registration part missing: %s", body)
	}
}
