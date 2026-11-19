package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// writeJSONCached emits a strong ETag over the exact JSON payload and honours
// If-None-Match with a bodyless 304. Use it for low-churn catalogs, not for
// session-scoped state where a stale value would be a correctness bug.
func writeJSONCached(w http.ResponseWriter, r *http.Request, status int, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=30")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, status, data)
}
