package api_test

// Route policy conformance: the table in route_policy.go must pin EVERY
// registered route (kills the class where auth_matrix pinned ~10/~116 while
// new spend/compute writers landed open). Adding a route without a row — or
// leaving a stale row — fails here. Default-deny: public-read rows serve no
// writes; every other write-serving row carries its gate or a written
// justification.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
)

func TestRoutePolicyCoversEveryRegisteredRoute(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	registered := srv.RegisteredRoutes()
	if len(registered) == 0 {
		t.Fatal("no routes recorded: registrations must go through s.route")
	}

	byPattern := map[string]api.RoutePolicy{}
	for _, p := range api.RoutePolicies {
		if p.Pattern == "" {
			t.Fatal("policy table holds an empty pattern")
		}
		if _, dup := byPattern[p.Pattern]; dup {
			t.Fatalf("policy table lists %q twice", p.Pattern)
		}
		byPattern[p.Pattern] = p
	}

	// 1. Registered-but-unlisted fails (the class kill).
	for _, pattern := range registered {
		if _, ok := byPattern[pattern]; !ok {
			t.Errorf("registered route %q has no policy-table row: classify it in route_policy.go", pattern)
		}
	}
	// 2. Stale rows fail (the table cannot rot apart from the mux).
	seen := map[string]bool{}
	for _, pattern := range registered {
		seen[pattern] = true
	}
	for _, p := range api.RoutePolicies {
		if !seen[p.Pattern] {
			t.Errorf("policy-table row %q matches no registered route: remove or re-register it", p.Pattern)
		}
	}

	// 3. Class vocabulary + default-deny posture.
	valid := map[api.RouteAuthClass]bool{
		api.RouteAuthBearer:        true,
		api.RouteAuthBearerWrites:  true,
		api.RouteAuthA2A:           true,
		api.RouteAuthPublicRead:    true,
		api.RouteAuthSessionScoped: true,
		api.RouteAuthSignature:     true,
		api.RouteAuthOpenWrite:     true,
	}
	for _, p := range api.RoutePolicies {
		if !valid[p.Class] {
			t.Errorf("row %q names unknown auth class %q", p.Pattern, p.Class)
		}
		if p.Methods == "" {
			t.Errorf("row %q names no methods", p.Pattern)
		}
		// Default-deny: a public-read row must serve no writes.
		if p.Class == api.RouteAuthPublicRead && p.ServesWrites() {
			t.Errorf("row %q is public-read but serves writes (%s): gate it or reclassify with justification", p.Pattern, p.Methods)
		}
		// Every non-bearer row documents its posture (the gate is the
		// justification for pure bearer rows only).
		if p.Class != api.RouteAuthBearer && strings.TrimSpace(p.Rationale) == "" {
			t.Errorf("row %q (%s) carries no written rationale", p.Pattern, p.Class)
		}
	}
}

// Registrations must flow through s.route so the policy test sees them.
// A direct s.mux.HandleFunc call compiles and serves but evades the table —
// this source guard fails it loudly.
func TestRoutesRegisteredThroughPolicyRecorder(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	serverSrc, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "server.go"))
	if err != nil {
		t.Fatalf("reading server.go: %v", err)
	}
	for _, line := range strings.Split(string(serverSrc), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// A real registration passes a string-literal pattern; the
		// recorder itself (s.route) forwards a variable and is exempt.
		if strings.Contains(line, ".mux.HandleFunc(\"") {
			t.Fatalf("direct mux registration evades the policy table: %q (use s.route)", trimmed)
		}
	}
}

// Behavioral pins for the Phase-0 gates: anonymous callers 401 before any
// parsing/acting on the newly-gated spend/compute writers, while the
// documented reads on mixed routes stay open.
func TestPhase0GatedRoutesRefuseAnonymous(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/mandates/verify"},
		{http.MethodPost, "/api/cost/settle"},
		{http.MethodPost, "/api/krylov/solve"},
		{http.MethodPost, "/api/mitigation/apply"},
		{http.MethodPost, "/api/agents/custom"},
		{http.MethodDelete, "/api/agents/custom/anything"},
		{http.MethodPut, "/api/llm/settings"},
		{http.MethodPost, "/api/quantum/settings"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s must 401 unauthenticated, got %d (%s)",
				tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	// Mixed-route reads stay public: settings snapshots and agent listing.
	for _, path := range []string{"/api/llm/settings", "/api/quantum/settings", "/api/agents/custom"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s must stay public, got %d (%s)", path, w.Code, w.Body.String())
		}
	}
}
