package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
)

func TestExperimentExportIncludesCampaignJobsAndPayloadDigest(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_experiment_export.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("export drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.BuildCampaignProposal(context.Background(), sess.ID, "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	if err := l.SaveCampaignStateWithTrigger(sess.ID, stateBytes, "session-create"); err != nil {
		t.Fatal(err)
	}
	result := `{"success":true,"kind":"spin_chain","result_version":"experiment-result-v1","model":"tfim","n_sites":4,"energy":-3.2,"energy_exact":-3.21,"uncertainty":0.004,"gap":0.8,"claimable":false,"measured":true}`
	if _, err := srv.jobRunner.AdoptExternalJob(sess.ID, "job-export-1", "spin_chain", []byte(result)); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sess.ID+"/experiment-export", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK      bool `json:"ok"`
		Payload struct {
			ExportVersion string `json:"export_version"`
			Campaign      any    `json:"campaign"`
			Jobs          []struct {
				JobID        string `json:"job_id"`
				ResultSHA256 string `json:"result_sha256"`
			} `json:"jobs"`
		} `json:"payload"`
		Envelope struct {
			PayloadSHA256 string `json:"payload_sha256"`
			Signed        bool   `json:"signed"`
		} `json:"envelope"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Payload.ExportVersion != "experiment-export-v1" || len(out.Payload.Jobs) != 1 {
		t.Fatalf("export payload wrong: %+v", out)
	}
	if out.Payload.Jobs[0].JobID != "job-export-1" || out.Payload.Jobs[0].ResultSHA256 == "" {
		t.Fatalf("export job wrong: %+v", out.Payload.Jobs[0])
	}
	if out.Envelope.PayloadSHA256 == "" {
		t.Fatal("payload digest missing")
	}
	if out.Payload.Campaign == nil {
		t.Fatal("campaign must be included")
	}
}
