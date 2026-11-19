package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"deflate, gzip", true},
		{"gzip;q=0", false},
		{"gzip;q=0.5", true},
		{"br, *;q=0.1", true},
		{"*;q=0", false},
		{"identity", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		if tc.header != "" {
			r.Header.Set("Accept-Encoding", tc.header)
		}
		if got := acceptsGzip(r); got != tc.want {
			t.Fatalf("acceptsGzip(%q)=%v want %v", tc.header, got, tc.want)
		}
	}
}

func TestWriteJSONSetsContentLength(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"ok": "yes"})
	if got := rec.Header().Get("Content-Length"); got != "12" {
		t.Fatalf("Content-Length=%q want 12", got)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("small response must stay identity, got %q", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil || decoded["ok"] != "yes" {
		t.Fatalf("body=%q err=%v", rec.Body.String(), err)
	}
}

func TestWriteJSONCompressesLargePayload(t *testing.T) {
	big := strings.Repeat("campaign-session-row-", 400)
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, gzipOK: true}
	writeJSON(sw, http.StatusOK, map[string]string{"data": big})
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding=%q want gzip", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary=%q missing Accept-Encoding", got)
	}
	if got, want := rec.Header().Get("Content-Length"), rec.Body.Len(); got != strconvItoa(want) {
		t.Fatalf("Content-Length=%q want %d", got, want)
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded["data"] != big {
		t.Fatalf("decoded payload mismatch err=%v", err)
	}
}

func TestWriteJSONIdentityWhenGzipRefused(t *testing.T) {
	big := strings.Repeat("x", 4096)
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, gzipOK: false}
	writeJSON(sw, http.StatusOK, map[string]string{"data": big})
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("identity response must not set Content-Encoding, got %q", got)
	}
	if got, want := rec.Header().Get("Content-Length"), rec.Body.Len(); got != strconvItoa(want) {
		t.Fatalf("Content-Length=%q want %d", got, want)
	}
}

func TestPprofRequiresOperatorBearer(t *testing.T) {
	t.Setenv("KUBONA_API_TOKEN", "operator-secret")
	s := &Server{mux: http.NewServeMux(), httpMetrics: newHTTPMetrics()}
	s.registerPprof()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous pprof status=%d want 401/403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer operator-secret")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator pprof status=%d want 200", rec.Code)
	}
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func BenchmarkWriteJSON(b *testing.B) {
	sessions := make([]map[string]string, 50)
	for i := range sessions {
		sessions[i] = map[string]string{
			"id": strings.Repeat("a", 36), "title": strings.Repeat("campaign ", 6),
			"tenant_id": "lab-mainza", "status": "active",
		}
	}
	payload := map[string]any{"sessions": sessions}
	b.Run("identity", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rec := httptest.NewRecorder()
			sw := &statusWriter{ResponseWriter: rec, gzipOK: false}
			writeJSON(sw, http.StatusOK, payload)
		}
	})
	b.Run("gzip", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rec := httptest.NewRecorder()
			sw := &statusWriter{ResponseWriter: rec, gzipOK: true}
			writeJSON(sw, http.StatusOK, payload)
		}
	})
}

func TestWriteJSONCachedHonoursETag(t *testing.T) {
	payload := map[string]any{"catalog": strings.Repeat("x", 2048)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	writeJSONCached(rec, req, http.StatusOK, payload)
	etag := rec.Header().Get("ETag")
	if etag == "" || rec.Code != http.StatusOK {
		t.Fatalf("first response code=%d etag=%q", rec.Code, etag)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	writeJSONCached(rec2, req2, http.StatusOK, payload)
	if rec2.Code != http.StatusNotModified || rec2.Body.Len() != 0 {
		t.Fatalf("304 expected, got code=%d body=%d", rec2.Code, rec2.Body.Len())
	}
}
