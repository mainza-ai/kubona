package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func TestTaskCreateListAndClose(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	l, err := ledger.OpenLedger(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)
	srv := NewServer(l, sm, jobs.NewRunner(l, nil, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	sess, err := sm.CreateSession("tasks", 10, "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	h := srv.Handler()

	get := func() []map[string]any {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks?session_id="+sess.ID, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("list status %d: %s", w.Code, w.Body.String())
		}
		var out struct {
			Tasks []map[string]any `json:"tasks"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("list decode: %v", err)
		}
		return out.Tasks
	}
	if got := get(); len(got) != 0 {
		t.Fatalf("new session tasks = %d, want 0", len(got))
	}

	raw, _ := json.Marshal(map[string]any{
		"session_id": sess.ID, "title": "Inspect the anchor",
		"source_domain": "measurement", "source_id": "m1",
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(raw)))
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	if id == "" || created["status"] != "open" {
		t.Fatalf("unexpected task: %+v", created)
	}
	if got := get(); len(got) != 1 || got[0]["title"] != "Inspect the anchor" {
		t.Fatalf("list after create: %+v", got)
	}

	closeRaw, _ := json.Marshal(map[string]any{"session_id": sess.ID, "status": "done"})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tasks/"+id, bytes.NewReader(closeRaw)))
	if w.Code != http.StatusOK {
		t.Fatalf("close status %d: %s", w.Code, w.Body.String())
	}
	if got := get(); len(got) != 1 || got[0]["status"] != "done" {
		t.Fatalf("list after close: %+v", got)
	}
}

func TestTaskCreateIsIdempotentWithRequestID(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	l, err := ledger.OpenLedger(t.TempDir() + "/tasks_idem.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)
	srv := NewServer(l, sm, jobs.NewRunner(l, nil, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	sess, err := sm.CreateSession("tasks idem", 10, "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	h := srv.Handler()
	body := func(title string) []byte {
		raw, _ := json.Marshal(map[string]any{
			"session_id": sess.ID, "title": title, "source_domain": "measurement", "source_id": "m1", "request_id": "task-create-1",
		})
		return raw
	}
	post := func(title string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(body(title))))
		return w
	}
	first := post("Inspect the anchor")
	if first.Code != http.StatusOK {
		t.Fatalf("first create: %d %s", first.Code, first.Body.String())
	}
	replay := post("Inspect the anchor")
	if replay.Code != http.StatusOK || replay.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("replay must return the stored receipt: %d %q", replay.Code, replay.Header().Get("X-Idempotent-Replay"))
	}
	conflict := post("Different title")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("same request id with different content must 409, got %d %s", conflict.Code, conflict.Body.String())
	}
}
