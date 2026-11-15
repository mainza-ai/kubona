package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func TestDeleteSessionBlockedReturnsTyped409(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_SINGLE_LAB", "1")
	l, err := ledger.OpenLedger(t.TempDir() + "/delete.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	srv := NewServer(l, sm, jr, sup, watchdog.NewTaskWatchdog(l, sup))
	sess, err := sm.CreateSession("delete blocked", 10, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.SaveLoopState(ledger.LoopPersist{
		ID: "loop-live", SessionID: sess.ID, HarnessID: "ansatz-search",
		Status: "running", MaxSteps: 5, LastActivity: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.handleSessionByID(rec, httptest.NewRequest(http.MethodDelete, "/api/sessions/"+sess.ID, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "delete_blocked" {
		t.Fatalf("code = %q", body.Code)
	}
	raw, _ := json.Marshal(body.Details["blockers"])
	if !strings.Contains(string(raw), "live loops") {
		t.Fatalf("blockers = %s", raw)
	}
}
