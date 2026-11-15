package api_test

// Browser-write edge. Threat: the loopback API accepts credential-less
// writes in local single-lab mode, and a page can POST text/plain without
// a CORS preflight. DNS rebinding can also point a public name at the
// loopback listener and read public routes. The guard refuses those
// shapes and leaves curl (no Origin, no Sec-Fetch-Site) and the same-site
// console (allowlisted Origin, application/json) alone.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
)

func policyProbePath(pattern string) string {
	if strings.HasSuffix(pattern, "/") {
		return pattern + "probe"
	}
	return pattern
}

func firstWriteMethod(methods string) string {
	for _, method := range strings.Split(methods, ",") {
		switch strings.TrimSpace(method) {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			return strings.TrimSpace(method)
		}
	}
	return ""
}

func guardRequest(method, path, host string, headers map[string]string, body string) *http.Request {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, reader)
	}
	req.Host = host
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req
}

func TestBrowserWriteGuardOverRouteTable(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	handler := srv.Handler()

	writes := 0
	for _, row := range api.RoutePolicies {
		method := firstWriteMethod(row.Methods)
		if method == "" {
			continue
		}
		writes++
		path := policyProbePath(row.Pattern)
		cases := []struct {
			name    string
			host    string
			headers map[string]string
			body    string
			want    string
		}{
			{
				name: "rebinding host",
				host: "evil.example",
				body: "{}",
				want: "host is not allowlisted",
			},
			{
				name: "foreign origin",
				host: "127.0.0.1:8080",
				headers: map[string]string{
					"Origin":       "http://evil.example",
					"Content-Type": "application/json",
				},
				body: "{}",
				want: "cross-site write refused: origin is not allowlisted",
			},
			{
				name: "cross-site fetch",
				host: "127.0.0.1:8080",
				headers: map[string]string{
					"Origin":         "http://localhost:3000",
					"Sec-Fetch-Site": "cross-site",
					"Content-Type":   "application/json",
				},
				body: "{}",
				want: "cross-site write refused",
			},
			{
				name: "simple content type",
				host: "127.0.0.1:8080",
				headers: map[string]string{
					"Origin":         "http://localhost:3000",
					"Sec-Fetch-Site": "same-site",
					"Content-Type":   "text/plain",
				},
				body: "not-json",
				want: "browser writes must send application/json",
			},
		}
		for _, tc := range cases {
			t.Run(row.Pattern+"/"+tc.name, func(t *testing.T) {
				req := guardRequest(method, path, tc.host, tc.headers, tc.body)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				var body struct {
					Message string `json:"message"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				if rec.Code != http.StatusForbidden || body.Message != tc.want {
					t.Fatalf("%s %s: got %d %s, want 403 message %q",
						method, path, rec.Code, rec.Body.String(), tc.want)
				}
			})
		}
	}
	if writes < 10 {
		t.Fatalf("route table exposed %d write rows; the guard was not pinned", writes)
	}

	t.Run("public read refuses a rebinding host and keeps a cross-site read", func(t *testing.T) {
		rebinding := guardRequest(http.MethodGet, "/api/health", "evil.example", nil, "")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, rebinding)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "host is not allowlisted") {
			t.Fatalf("rebinding read: got %d %s", rec.Code, rec.Body.String())
		}
		crossRead := guardRequest(http.MethodGet, "/api/health", "127.0.0.1:8080", map[string]string{
			"Origin":         "http://evil.example",
			"Sec-Fetch-Site": "cross-site",
		}, "")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, crossRead)
		if rec.Code != http.StatusOK {
			t.Fatalf("cross-site GET must stay a read, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("curl and same-site json are not browser-write refusals", func(t *testing.T) {
		curl := guardRequest(http.MethodPost, "/api/discovery/plan", "127.0.0.1:8080", map[string]string{
			"Content-Type": "text/plain",
		}, "not-json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, curl)
		if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "cross-site write refused") {
			t.Fatalf("curl was treated as a browser write: %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "browser writes must send application/json") {
			t.Fatalf("curl was held to the browser content-type rule: %s", rec.Body.String())
		}
		console := guardRequest(http.MethodPost, "/api/discovery/plan", "localhost:8080", map[string]string{
			"Origin":         "http://localhost:3000",
			"Sec-Fetch-Site": "same-site",
			"Content-Type":   "application/json",
		}, `{"goal":"guard-probe"}`)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, console)
		if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "cross-site write refused") {
			t.Fatalf("same-site console write was refused: %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "host is not allowlisted") || strings.Contains(rec.Body.String(), "browser writes must send") {
			t.Fatalf("same-site console write hit the guard: %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("configured name is allowed and an unlisted name is not", func(t *testing.T) {
		t.Setenv("KUBONA_ALLOWED_HOSTS", "lab.local")
		allowed := guardRequest(http.MethodGet, "/api/health", "lab.local:8080", nil, "")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, allowed)
		if rec.Code != http.StatusOK {
			t.Fatalf("configured host: got %d %s", rec.Code, rec.Body.String())
		}
		t.Setenv("KUBONA_ALLOWED_HOSTS", "")
		refused := guardRequest(http.MethodGet, "/api/health", "lab.local:8080", nil, "")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, refused)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unconfigured host: got %d %s", rec.Code, rec.Body.String())
		}
	})
}
