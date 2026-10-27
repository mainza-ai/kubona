package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPanicRecoveryReturnsTyped500(t *testing.T) {
	h := requestIDAndRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("drill")
	}))
	req := httptest.NewRequest(http.MethodGet, "/drill", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic must return 500, got %d", w.Code)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("panic response must carry a request id")
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["code"] != "internal" {
		t.Fatalf("panic must return a typed internal error: %s", w.Body.String())
	}
}
