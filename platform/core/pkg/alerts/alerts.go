package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// Notify posts one operated alert to KUBONA_ALERT_WEBHOOK_URL. It is
// best-effort and never blocks control flow: a missing URL is a no-op, and
// delivery failures are intentionally not converted into spend/state errors.
// The payload is deliberately small and contains no prompts or credentials.
func Notify(ctx context.Context, event, severity, detail string) {
	url := strings.TrimSpace(os.Getenv("KUBONA_ALERT_WEBHOOK_URL"))
	if url == "" {
		return
	}
	body, _ := json.Marshal(map[string]string{
		"text":     "Kubona alert [" + severity + "] " + event + ": " + detail,
		"event":    event,
		"severity": severity,
	})
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}
