package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

func TestResearchGraphSummaryAndResumeCursor(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 12})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}

	type graphResp struct {
		Summary string `json:"summary"`
		Report  struct {
			BudgetBytes int      `json:"budget_bytes"`
			Bytes       int      `json:"bytes"`
			Included    []string `json:"included"`
			Omitted     []string `json:"omitted"`
			Truncated   bool     `json:"truncated"`
		} `json:"summary_report"`
		Resume struct {
			SinceSeq int64 `json:"since_seq"`
			NextSeq  int64 `json:"next_seq"`
			HasMore  bool  `json:"has_more"`
			Events   []struct {
				ID        int64  `json:"id"`
				EventType string `json:"event_type"`
			} `json:"events"`
		} `json:"resume"`
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/research-graph", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("graph = %d: %s", w.Code, w.Body.String())
	}
	var full graphResp
	json.Unmarshal(w.Body.Bytes(), &full)
	if full.Summary == "" || len(full.Summary) > 4096 {
		t.Fatalf("default summary = %d bytes", len(full.Summary))
	}
	if full.Report.Bytes != len(full.Summary) {
		t.Fatalf("report bytes = %d, summary = %d", full.Report.Bytes, len(full.Summary))
	}
	found := false
	for _, s := range full.Report.Included {
		if s == "objective" {
			found = true
		}
	}
	if !found {
		t.Fatalf("objective not included: %+v", full.Report)
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/research-graph?budget_bytes=600", "", nil)
	var tight graphResp
	json.Unmarshal(w.Body.Bytes(), &tight)
	if w.Code != http.StatusOK || len(tight.Summary) > 600 {
		t.Fatalf("tight summary = %d bytes (%d)", len(tight.Summary), w.Code)
	}
	if tight.Report.BudgetBytes != 600 || !tight.Report.Truncated {
		t.Fatalf("tight report = %+v", tight.Report)
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/research-graph?budget_bytes=nope", "", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad budget = %d", w.Code)
	}

	// Resume: two pages must not overlap and the cursor must advance.
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/research-graph?since_seq=0&event_limit=1", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("resume = %d: %s", w.Code, w.Body.String())
	}
	var page1 graphResp
	json.Unmarshal(w.Body.Bytes(), &page1)
	if len(page1.Resume.Events) != 1 || page1.Resume.NextSeq <= 0 {
		t.Fatalf("page1 = %+v", page1.Resume)
	}
	if !page1.Resume.HasMore {
		t.Fatalf("expected more events than the page: %+v", page1.Resume)
	}
	w = doJSON(t, srv, http.MethodGet,
		"/api/sessions/"+sid+"/research-graph?since_seq="+strconv.FormatInt(page1.Resume.NextSeq, 10)+"&event_limit=1", "", nil)
	var page2 graphResp
	json.Unmarshal(w.Body.Bytes(), &page2)
	if w.Code != http.StatusOK {
		t.Fatalf("page2 = %d: %s", w.Code, w.Body.String())
	}
	for _, e := range page2.Resume.Events {
		if e.ID <= page1.Resume.NextSeq {
			t.Fatalf("cursor replay returned an old event: %+v", e)
		}
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/research-graph?since_seq=-1", "", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative cursor = %d", w.Code)
	}
}
