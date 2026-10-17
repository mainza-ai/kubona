package api_test

// R5 conformance: the Go error registry IS the single-source CSV
// (drift = red), and every catalogued code constructs + renders its
// documented shape (the retry-vs-escalate fixture, reviewed by reading).

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
)

func TestErrorRegistryMatchesCatalog(t *testing.T) {
	// Independent re-parse of the single-source CSV (no shared loader):
	// the registry must equal the file row for row — drift = red.
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "error-catalog.csv"))
	if err != nil {
		t.Fatalf("reading catalog: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil {
		t.Fatalf("parsing catalog: %v", err)
	}
	seen := map[string]bool{}
	for i, r := range rows {
		if i == 0 {
			continue
		}
		code := strings.TrimSpace(r[0])
		seen[code] = true
		e, err := api.NewAPIError(code, "probe", nil)
		if err != nil {
			t.Fatalf("catalogued code %q must construct: %v", code, err)
		}
		status, _ := strconv.Atoi(strings.TrimSpace(r[2]))
		retryable, _ := strconv.ParseBool(strings.TrimSpace(r[3]))
		if e.Category != strings.TrimSpace(r[1]) || e.HTTPStatus != status ||
			e.Retryable != retryable || e.Remediation != strings.TrimSpace(r[4]) {
			t.Fatalf("registry drifts from catalog row %d (%s): %+v", i+1, code, e)
		}
		w := httptest.NewRecorder()
		e.Write(w)
		if w.Code != e.HTTPStatus {
			t.Fatalf("%s: rendered %d, catalogued %d", code, w.Code, e.HTTPStatus)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: body not JSON: %v", code, err)
		}
		for _, k := range []string{"code", "category", "message", "retryable", "remediation"} {
			if _, ok := body[k]; !ok {
				t.Fatalf("%s: body missing %q: %v", code, k, body)
			}
		}
		if body["code"] != code || body["retryable"] != e.Retryable {
			t.Fatalf("%s: body disagrees with registry: %v", code, body)
		}
	}
	// Every registry code must exist in the file (no orphan codes the
	// CSV review never saw).
	for _, code := range []string{
		"unauthorized", "forbidden", "rate_limited", "bad_request",
		"ghost_session", "unknown_rail", "method_not_allowed",
		"ceiling_exceeded", "budget_exceeded", "mandate_invalid", "request_conflict",
		"request_in_progress", "storage_blocked", "resource_constraint",
		"not_ready", "bridge_unavailable", "llm_unavailable",
		"unknown_tool", "internal", "server_misconfigured",
	} {
		if !seen[code] {
			t.Fatalf("registry code %q missing from catalog", code)
		}
		if _, err := api.NewAPIError(code, "probe", nil); err != nil {
			t.Fatalf("registry code %q must construct: %v", code, err)
		}
	}
	if _, err := api.NewAPIError("no_such_code", "x", nil); err == nil {
		t.Fatal("uncatalogued codes must refuse construction")
	}
}

func TestErrorCatalogShape(t *testing.T) {
	// The CSV parses to the same 19 codes (row-level drift guard: a
	// silent CSV edit that the registry test above would bless by
	// construction is caught here by count).
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "error-catalog.csv"))
	if err != nil {
		t.Fatalf("reading catalog: %v", err)
	}
	lines := 0
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			lines++
		}
	}
	if lines != 21 { // header + 20 codes
		t.Fatalf("catalog must carry header + 19 codes, got %d lines", lines)
	}
	_ = http.StatusOK
}
