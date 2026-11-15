package api_test

// B10 API pins: retention screens secrets, recall never echoes them, and
// retraction tombstones a row with an audited event.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func memoryPost(t *testing.T, h http.Handler, path string, body any) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer op-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func memoryGet(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestMemorySecretScreenAndRetraction(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "memory hygiene")
	const leaked = "sk-abcdefghijklmnopqrstuvwxyz123456"

	code, body := memoryPost(t, h, "/api/memory/retain", map[string]any{
		"session_id": sid,
		"content":    "Provider setup: use " + leaked + " for the endpoint",
		"bank":       "experiences",
	})
	if code != http.StatusCreated || !strings.Contains(body, `"redacted":true`) {
		t.Fatalf("retain must screen the secret: %d %s", code, body)
	}
	var retained struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &retained); err != nil || retained.ID == "" {
		t.Fatalf("retain decode: %v %s", err, body)
	}

	code, body = memoryGet(t, h, "/api/memory/recall?session_id="+sid+"&query=Provider")
	if code != http.StatusOK {
		t.Fatalf("recall: %d %s", code, body)
	}
	if strings.Contains(body, leaked) {
		t.Fatalf("recall echoed the secret: %s", body)
	}
	if !strings.Contains(body, "[redacted:openai_key]") {
		t.Fatalf("recall must show the redaction marker: %s", body)
	}

	// Unknown/foreign memory ids refuse without an oracle.
	if code, _ := memoryPost(t, h, "/api/memory/retract", map[string]any{
		"session_id": sid, "memory_id": "mem-missing", "reason": "test",
	}); code != http.StatusNotFound {
		t.Fatalf("unknown memory must 404, got %d", code)
	}
	code, body = memoryPost(t, h, "/api/memory/retract", map[string]any{
		"session_id": sid, "memory_id": retained.ID, "reason": "operator request",
	})
	if code != http.StatusOK || !strings.Contains(body, `"retracted":true`) {
		t.Fatalf("retract: %d %s", code, body)
	}
	code, body = memoryGet(t, h, "/api/memory/recall?session_id="+sid+"&query=Provider")
	if code != http.StatusOK {
		t.Fatalf("recall after retraction: %d %s", code, body)
	}
	if strings.Contains(body, "Provider setup") || strings.Contains(body, leaked) {
		t.Fatalf("retracted memory must not be recalled: %s", body)
	}
	// The retraction is auditable.
	events, err := srv.Ledger().GetRecentEvents(sid, 200)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.EventType == "memory_retracted" {
			found = true
		}
	}
	if !found {
		t.Fatal("memory_retracted event missing")
	}
}
