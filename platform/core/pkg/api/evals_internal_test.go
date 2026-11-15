package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/evals"
)

func TestEvalStatusEndpointReportsEvaluatedAndStale(t *testing.T) {
	set, err := evals.Load("copilot")
	if err != nil {
		t.Fatal(err)
	}
	store := evals.Store{Dir: t.TempDir()}
	s := &Server{evalStore: store}

	read := func() evals.Status {
		rec := httptest.NewRecorder()
		s.handleEvalLLM(rec, httptest.NewRequest(http.MethodGet, "/api/evals/llm?role=copilot&provider=local&model=m", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		var status evals.Status
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status
	}

	status := read()
	if status.Status != evals.StatusUnevaluated || status.Total != len(set.Fixtures)-2 {
		t.Fatalf("initial status = %+v", status)
	}

	// Record a perfect synthetic evaluation, then expect evaluated.
	report := evals.Report{
		Role: "copilot", Provider: "local", Model: "m", FixturesSHA256: set.SHA256,
		EvaluatedAt: time.Now().UTC().Format(time.RFC3339), Total: set.ScoredCount(),
		Unscored: 2, Brier: 0, ECE: 0, PerLabel: map[string]evals.LabelScore{},
		Labels: set.Labels,
	}
	if err := store.Write(report); err != nil {
		t.Fatal(err)
	}
	status = read()
	if status.Status != evals.StatusEvaluated || status.Evaluated != set.ScoredCount() || status.Brier == nil {
		t.Fatalf("evaluated status = %+v", status)
	}

	// Missing identity refuses.
	rec := httptest.NewRecorder()
	s.handleEvalLLM(rec, httptest.NewRequest(http.MethodGet, "/api/evals/llm?role=copilot", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing params status = %d", rec.Code)
	}
	// Unknown role refuses with a readable error.
	rec = httptest.NewRecorder()
	s.handleEvalLLM(rec, httptest.NewRequest(http.MethodGet, "/api/evals/llm?role=unknown&provider=p&model=m", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown role status = %d", rec.Code)
	}
}
