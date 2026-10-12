package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func memoryTestSession(t *testing.T, srv interface {
	Handler() http.Handler
}, title, tenantID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"title": title, "tenant_id": tenantID})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("session %q: %d %s", title, w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.ID
}

func retainPost(t *testing.T, h http.Handler, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/memory/retain", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.NewDecoder(w.Body).Decode(&out)
	return w.Code, out
}

func recallGet(t *testing.T, h http.Handler, query string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/memory/recall?"+query, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.NewDecoder(w.Body).Decode(&out)
	return w.Code, out
}

// Retain → recall round-trips through the same scoping the tool enforces:
// explicit importance 0 survives (absent defaults to 0.5), banks persist,
// and unknown sessions 404 instead of answering empty.
func TestMemoryHTTPRoundTrip(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	sid := memoryTestSession(t, srv, "memory http drill", "")
	code, created := retainPost(t, h, map[string]any{
		"session_id": sid, "content": "H2 anchor recomputed at R=0.7414", "bank": "world-facts",
	})
	if code != http.StatusCreated {
		t.Fatalf("retain: %d %v", code, created)
	}
	if created["importance"] != 0.5 || created["bank"] != "world-facts" || created["tenant"] != "default" {
		t.Errorf("retain receipt wrong: %v", created)
	}

	code, zero := retainPost(t, h, map[string]any{
		"session_id": sid, "content": "stale note", "importance": 0,
	})
	if code != http.StatusCreated || zero["importance"] != float64(0) {
		t.Errorf("explicit importance 0 must survive, got %d %v", code, zero)
	}

	code, out := recallGet(t, h, "session_id="+sid+"&q=H2+anchor&bank=world-facts")
	if code != http.StatusOK {
		t.Fatalf("recall: %d %v", code, out)
	}
	snips, _ := out["snippets"].([]any)
	if len(snips) != 1 {
		t.Fatalf("expected 1 snippet, got %v", out)
	}
	first, _ := snips[0].(map[string]any)
	if first["content"] != "H2 anchor recomputed at R=0.7414" || first["bank"] != "world-facts" {
		t.Errorf("snippet wrong: %v", first)
	}

	if code, _ := recallGet(t, h, "session_id=ghost&q=x"); code != http.StatusNotFound {
		t.Errorf("unknown session must 404, got %d", code)
	}
	if code, _ := recallGet(t, h, "q=x"); code != http.StatusBadRequest {
		t.Errorf("missing session_id must 400, got %d", code)
	}

	// Inventory mode: empty query lists newest-first with null similarity
	// (no match computed), not an empty semantic result.
	code, out = recallGet(t, h, "session_id="+sid+"&bank=all")
	if code != http.StatusOK {
		t.Fatalf("inventory: %d %v", code, out)
	}
	snips, _ = out["snippets"].([]any)
	if len(snips) != 2 {
		t.Fatalf("inventory must list both retained rows, got %v", out)
	}
	latest, _ := snips[0].(map[string]any)
	if latest["content"] != "stale note" || latest["similarity"] != nil {
		t.Errorf("inventory newest-first with null similarity: %v", latest)
	}
}

// Tenant isolation holds over HTTP: tenant B's session recalls nothing of
// tenant A's memories, and bank=all still respects the boundary.
func TestMemoryHTTPenantIsolation(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	if _, err := srv.TenantManager().CreateTenant("lab-a", "Lab A"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.TenantManager().CreateTenant("lab-b", "Lab B"); err != nil {
		t.Fatal(err)
	}
	sidA := memoryTestSession(t, srv, "a session", "lab-a")
	sidB := memoryTestSession(t, srv, "b session", "lab-b")

	if code, _ := retainPost(t, h, map[string]any{"session_id": sidA, "content": "lab-a secret synthesis route"}); code != http.StatusCreated {
		t.Fatalf("retain in A: %d", code)
	}
	code, out := recallGet(t, h, "session_id="+sidB+"&q=secret+synthesis&bank=all")
	if code != http.StatusOK {
		t.Fatalf("recall in B: %d", code)
	}
	if snips, _ := out["snippets"].([]any); len(snips) != 0 {
		t.Errorf("cross-tenant leak over HTTP: %v", snips)
	}
	code, out = recallGet(t, h, "session_id="+sidA+"&q=secret+synthesis&bank=all")
	if code != http.StatusOK {
		t.Fatalf("recall in A: %d", code)
	}
	if snips, _ := out["snippets"].([]any); len(snips) != 1 {
		t.Errorf("own tenant must recall exactly 1, got %v", out)
	}
}

// Malformed retain/recall refuses with the reason named: bad bank on both
// routes, empty/oversize content, bad similarity floor.
func TestMemoryHTTPRefusals(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	sid := memoryTestSession(t, srv, "refusal drill", "")

	for name, payload := range map[string]map[string]any{
		"empty content": {"session_id": sid, "content": "   "},
		"bad bank":      {"session_id": sid, "content": "x", "bank": "vibes"},
		"oversize":      {"session_id": sid, "content": strings.Repeat("x", 4001)},
		"ghost session": {"session_id": "ghost", "content": "x"},
	} {
		if code, _ := retainPost(t, h, payload); code < 400 || code >= 500 {
			t.Errorf("retain %s: must 4xx, got %d", name, code)
		}
	}
	for name, q := range map[string]string{
		"bad bank":   "session_id=" + sid + "&bank=vibes",
		"bad minsim": "session_id=" + sid + "&min_similarity=2",
	} {
		if code, out := recallGet(t, h, q); code != http.StatusBadRequest {
			t.Errorf("recall %s: must 400, got %d %v", name, code, out)
		}
	}
	if code, _ := retainPost(t, h, map[string]any{
		"session_id": sid, "content": "clamped", "importance": 9,
	}); code != http.StatusCreated {
		t.Errorf("over-range importance must clamp, not refuse, got %d", code)
	}
}

// Inventory mode (empty q → ListRecent) crosses the same tenant boundary
// as semantic recall: tenant B's inventory must not list tenant A's rows,
// and listed rows carry similarity:null (no match computed, never a score).
func TestMemoryHTTPInventoryIsolation(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	if _, err := srv.TenantManager().CreateTenant("lab-ia", "Lab IA"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.TenantManager().CreateTenant("lab-ib", "Lab IB"); err != nil {
		t.Fatal(err)
	}
	sidA := memoryTestSession(t, srv, "ia session", "lab-ia")
	sidB := memoryTestSession(t, srv, "ib session", "lab-ib")

	if code, _ := retainPost(t, h, map[string]any{"session_id": sidA, "content": "lab-ia inventory secret"}); code != http.StatusCreated {
		t.Fatalf("retain in A: %d", code)
	}
	code, out := recallGet(t, h, "session_id="+sidB+"&bank=all")
	if code != http.StatusOK {
		t.Fatalf("inventory in B: %d", code)
	}
	if snips, _ := out["snippets"].([]any); len(snips) != 0 {
		t.Errorf("cross-tenant inventory leak: %v", snips)
	}
	code, out = recallGet(t, h, "session_id="+sidA+"&bank=all")
	if code != http.StatusOK {
		t.Fatalf("inventory in A: %d", code)
	}
	snips, _ := out["snippets"].([]any)
	if len(snips) != 1 {
		t.Fatalf("own inventory must list exactly 1, got %v", out)
	}
	row, _ := snips[0].(map[string]any)
	if sim, present := row["similarity"]; !present || sim != nil {
		t.Errorf("inventory similarity must be present-null (no match computed), got %v (present=%v)", sim, present)
	}
	if row["content"] != "lab-ia inventory secret" {
		t.Errorf("inventory content wrong: %v", row)
	}
}
