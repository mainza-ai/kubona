package api

// R5 — canonical typed errors: every agent-facing refusal carries
// {code, category, message, retryable, remediation, details?} so an agent
// can implement retry-vs-escalate from the catalog alone. Codes are a
// CLOSED set in platform/contracts/error-catalog.csv (the single source);
// this registry mirrors it and the conformance test asserts identity
// (drift = red, same pattern as the event taxonomy). Constructing an
// uncatalogued code refuses — fail-closed construction.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// APIError is one typed refusal.
type APIError struct {
	Code        string         `json:"code"`
	Category    string         `json:"category"`
	HTTPStatus  int            `json:"http_status"`
	Retryable   bool           `json:"retryable"`
	Message     string         `json:"message"`
	Remediation string         `json:"remediation"`
	Details     map[string]any `json:"details,omitempty"`
}

type errorSpec struct {
	category    string
	httpStatus  int
	retryable   bool
	remediation string
}

var errorRegistry = map[string]errorSpec{
	"unauthorized":         {"auth", 401, false, "pass a valid bearer token (KUBONA_API_TOKEN)"},
	"forbidden":            {"auth", 403, false, "use an operator-authorized credential for this action"},
	"rate_limited":         {"auth", 429, true, "back off and retry after the limiter window"},
	"bad_request":          {"validation", 400, false, "fix the request shape (see message details)"},
	"ghost_session":        {"validation", 404, false, "create the session first; ids are never auto-vivified"},
	"unknown_rail":         {"validation", 400, false, "use a documented rail name (see message)"},
	"method_not_allowed":   {"validation", 405, false, "use the documented method for this route"},
	"budget_exceeded":      {"budget", 200, false, "lower max_budget_usd or raise the tenant ceiling (rides a 200 valid:false envelope, not an error status)"},
	"mandate_invalid":      {"state", 200, false, "mint a fresh mandate: bind the tenant admin key, exact parameters, and a live expiry (rides a 200 valid:false envelope)"},
	"request_conflict":     {"state", 409, false, "same id with different content will never succeed: use a new request_id"},
	"request_in_progress":  {"state", 409, true, "the claim is still running: poll the same request_id again"},
	"storage_blocked":      {"resource", 503, true, "free disk above the resume threshold, then retry (reads stay available)"},
	"resource_constraint":  {"resource", 503, false, "shrink inputs or page evidence via evidence_continue"},
	"not_ready":            {"state", 503, true, "wait for readiness (disk recovery, bridge connect) then retry"},
	"bridge_unavailable":   {"upstream", 502, true, "retry when the Python bridge is reachable; sim-only paths stay available"},
	"llm_unavailable":      {"upstream", 503, true, "retry when a provider is configured; fallback paths stay open"},
	"unknown_tool":         {"validation", 400, false, "call a tool from the pinned catalog (/api/capabilities)"},
	"internal":             {"internal", 500, false, "report the incident id; do not blind-retry mutations (use idempotency keys)"},
	"server_misconfigured": {"internal", 503, false, "set the missing server configuration (see message) and restart"},
}

// NewAPIError builds a typed refusal. Unknown codes refuse construction:
// emitting an uncatalogued code would fork the retry-vs-escalate contract.
func NewAPIError(code, message string, details map[string]any) (*APIError, error) {
	spec, ok := errorRegistry[code]
	if !ok {
		return nil, fmt.Errorf("uncatalogued error code %q: register it in contracts/error-catalog.csv first", code)
	}
	return &APIError{
		Code: code, Category: spec.category, HTTPStatus: spec.httpStatus,
		Retryable: spec.retryable, Message: message, Remediation: spec.remediation,
		Details: details,
	}, nil
}

// Write renders the typed refusal at the catalogued status.
func (e *APIError) Write(w http.ResponseWriter) {
	raw, err := json.Marshal(e)
	if err != nil {
		http.Error(w, e.Message, e.HTTPStatus)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	_, _ = w.Write(raw)
}

// WriteAPIError is the handler one-liner: same status the plain-text
// error carried, now typed. Construction cannot fail for catalogued
// codes; a failure falls back to the legacy shape (never a 500 for a
// 400-class refusal).
func WriteAPIError(w http.ResponseWriter, code, message string, details map[string]any) {
	e, err := NewAPIError(code, message, details)
	if err != nil {
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	e.Write(w)
}

// errorCatalogPath locates the single-source CSV from the api package dir.
func errorCatalogPath() string {
	return filepath.Join("..", "..", "contracts", "error-catalog.csv")
}

// loadErrorCatalog parses the single-source CSV (code,category,
// http_status,retryable,remediation).
func loadErrorCatalog(path string) (map[string]errorSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string]errorSpec{}
	for i, r := range rows {
		if i == 0 {
			continue // header
		}
		if len(r) != 5 {
			return nil, fmt.Errorf("row %d: want 5 columns, got %d", i+1, len(r))
		}
		status, err := strconv.Atoi(strings.TrimSpace(r[2]))
		if err != nil {
			return nil, fmt.Errorf("row %d: bad status %q", i+1, r[2])
		}
		retryable, err := strconv.ParseBool(strings.TrimSpace(r[3]))
		if err != nil {
			return nil, fmt.Errorf("row %d: bad retryable %q", i+1, r[3])
		}
		out[strings.TrimSpace(r[0])] = errorSpec{
			category: strings.TrimSpace(r[1]), httpStatus: status,
			retryable: retryable, remediation: strings.TrimSpace(r[4]),
		}
	}
	return out, nil
}
