package api_test

// R5/C5/I5 HTTP test: the capabilities endpoint serves the pinned
// catalog digest + runtime state in one shape the console degrades from.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/disk"
)

func TestCapabilitiesShape(t *testing.T) {
	srv, _, close := setupIdemServer(t)
	defer close()
	// Attach the decided guard: the endpoint serves LIVE values, so pin
	// the live configuration first (guardless servers report guarded:false).
	srv.SetDiskGuard(disk.NewGuard(disk.DefaultThresholds(), []string{t.TempDir()}, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("capabilities: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		CatalogDigest string `json:"catalog_digest"`
		Tools         []any  `json:"tools"`
		Disk          struct {
			Guarded        bool   `json:"guarded"`
			WarnBytes      uint64 `json:"warn_bytes"`
			BlockBytes     uint64 `json:"block_bytes"`
			ResumeBytes    uint64 `json:"resume_bytes"`
			Level          string `json:"level"`
			BytesAvailable uint64 `json:"bytes_available"`
		} `json:"disk"`
		Harnesses []string       `json:"harnesses"`
		Budget    map[string]any `json:"budget"`
		Flags     map[string]any `json:"flags"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Digest agrees with the checked-in reviewed file (fail-closed
	// discovery: the console pins this value).
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "tool-catalog.json"))
	if err != nil {
		t.Fatalf("reading checked-in catalog: %v", err)
	}
	var checked struct {
		Digest string `json:"digest"`
		Tools  []any  `json:"tools"`
	}
	if err := json.Unmarshal(raw, &checked); err != nil {
		t.Fatalf("parsing checked-in catalog: %v", err)
	}
	if out.CatalogDigest != checked.Digest || len(out.Tools) != len(checked.Tools) {
		t.Fatalf("endpoint serves unreviewed surface: %q vs %q", out.CatalogDigest, checked.Digest)
	}
	if !out.Disk.Guarded {
		t.Fatal("attached guard must report guarded:true (guardless would lie)")
	}
	if out.Disk.WarnBytes != 2*1024*1024*1024 || out.Disk.BlockBytes != 1024*1024*1024 {
		t.Fatalf("disk thresholds must be the decided values: %+v", out.Disk)
	}
	if _, ok := out.Budget["turn_context_runes"]; !ok {
		t.Fatalf("budget must expose the turn projection budget: %v", out.Budget)
	}
	for _, k := range []string{"reviewer_configured", "ephemeral_signer_allowed", "bridge_connected"} {
		if _, ok := out.Flags[k]; !ok {
			t.Fatalf("flags must carry %q: %v", k, out.Flags)
		}
	}
	if out.Harnesses == nil {
		t.Fatal("harness catalog must be present (possibly empty)")
	}
	// POST refuses typed (discovery is GET-only).
	req = httptest.NewRequest(http.MethodPost, "/api/capabilities", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST capabilities must 405 typed, got %d", w.Code)
	}
	var terr map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &terr); err != nil || terr["code"] != "method_not_allowed" {
		t.Fatalf("405 must carry the typed code: %s", w.Body.String())
	}
}
