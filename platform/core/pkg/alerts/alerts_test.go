package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyPostsStructuredAlert(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("KUBONA_ALERT_WEBHOOK_URL", srv.URL)
	Notify(context.Background(), "settlement_skipped", "warning", "job-1: actual_usd_missing")
	if got["event"] != "settlement_skipped" || got["severity"] != "warning" || got["text"] == "" {
		t.Fatalf("alert payload wrong: %+v", got)
	}
}
