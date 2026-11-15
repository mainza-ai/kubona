package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMSettingsRevisionAndAirGapPatch(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	get := func() (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/llm/settings", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode settings: %v body=%s", err, w.Body.String())
		}
		return w.Code, body
	}
	call := func(method, token string, body any) (int, map[string]any) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/llm/settings", bytes.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var decoded map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &decoded)
		return w.Code, decoded
	}

	code, snap := get()
	if code != http.StatusOK {
		t.Fatalf("GET settings: %d", code)
	}
	if _, ok := snap["revision"].(float64); !ok {
		t.Fatalf("GET settings must include numeric revision, got %#v", snap["revision"])
	}
	providersBefore, _ := json.Marshal(snap["providers"])
	rolesBefore, _ := json.Marshal(snap["role_assignments"])
	modelBefore, _ := snap["active_model"].(string)

	if code, body := call(http.MethodPatch, "", map[string]any{"air_gapped": true}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous air-gap PATCH must 401, got %d %v", code, body)
	}
	if code, body := call(http.MethodPut, "", snap); code != http.StatusUnauthorized {
		t.Fatalf("anonymous settings PUT must 401, got %d %v", code, body)
	}

	if code, body := call(http.MethodPut, "op-token", map[string]any{"temperature": 0.2}); code != http.StatusPreconditionRequired {
		t.Fatalf("PUT without a revision must 428, got %d %v", code, body)
	} else if body["code"] != "precondition_required" {
		t.Fatalf("missing revision code = %v, want precondition_required", body["code"])
	}

	if code, body := call(http.MethodPatch, "op-token", map[string]any{"air_gapped": true, "active_model": "attacker"}); code != http.StatusBadRequest {
		t.Fatalf("PATCH with an extra field must 400, got %d %v", code, body)
	}

	// Release, then engage again. The second write is the state a stale
	// editor must not be able to undo.
	code, released := call(http.MethodPatch, "op-token", map[string]any{"air_gapped": false})
	if code != http.StatusOK || released["changed"] != true {
		t.Fatalf("PATCH release air-gap: %d changed=%v %v", code, released["changed"], released)
	}
	code, engaged := call(http.MethodPatch, "op-token", map[string]any{"air_gapped": true})
	if code != http.StatusOK || engaged["changed"] != true {
		t.Fatalf("PATCH engage air-gap: %d changed=%v %v", code, engaged["changed"], engaged)
	}
	code, afterPatch := get()
	if code != http.StatusOK || afterPatch["air_gapped"] != true {
		t.Fatalf("air-gap after PATCH = %v (status %d)", afterPatch["air_gapped"], code)
	}
	providersAfter, _ := json.Marshal(afterPatch["providers"])
	rolesAfter, _ := json.Marshal(afterPatch["role_assignments"])
	if string(providersAfter) != string(providersBefore) || string(rolesAfter) != string(rolesBefore) || afterPatch["active_model"] != modelBefore {
		t.Fatal("air-gap PATCH changed a field other than air_gapped")
	}
	revision := afterPatch["revision"].(float64)
	if revision != snap["revision"].(float64)+2 {
		t.Fatalf("two air-gap changes revision = %v, want %v", revision, snap["revision"].(float64)+2)
	}

	code, noop := call(http.MethodPatch, "op-token", map[string]any{"air_gapped": true})
	if code != http.StatusOK || noop["changed"] != false {
		t.Fatalf("repeat air-gap PATCH = %d changed %v, want 200 false", code, noop["changed"])
	}
	_, afterNoop := get()
	if afterNoop["revision"].(float64) != revision {
		t.Fatalf("no-op air-gap PATCH revision = %v, want %v", afterNoop["revision"], revision)
	}

	stale := afterNoop
	stale["air_gapped"] = false
	stale["temperature"] = 0.05
	stale["expected_revision"] = snap["revision"]
	code, conflict := call(http.MethodPut, "op-token", stale)
	if code != http.StatusConflict || conflict["code"] != "revision_conflict" {
		t.Fatalf("stale PUT = %d %v, want 409 revision_conflict", code, conflict)
	}
	_, afterStale := get()
	if afterStale["air_gapped"] != true || afterStale["revision"].(float64) != revision || afterStale["temperature"] == 0.05 {
		t.Fatalf("stale PUT released air-gap or applied the edit: air_gapped=%v revision=%v temperature=%v", afterStale["air_gapped"], afterStale["revision"], afterStale["temperature"])
	}

	fresh := afterStale
	fresh["temperature"] = 0.4
	delete(fresh, "expected_revision")
	code, saved := call(http.MethodPost, "op-token", fresh)
	if code != http.StatusOK {
		t.Fatalf("POST echoing revision must succeed, got %d %v", code, saved)
	}
	_, afterSave := get()
	if afterSave["air_gapped"] != true || afterSave["temperature"].(float64) != 0.4 {
		t.Fatalf("echoed-revision save lost air-gap or temperature: air_gapped=%v temperature=%v", afterSave["air_gapped"], afterSave["temperature"])
	}
	if afterSave["revision"].(float64) != revision+1 {
		t.Fatalf("successful save revision = %v, want %v", afterSave["revision"], revision+1)
	}
}
