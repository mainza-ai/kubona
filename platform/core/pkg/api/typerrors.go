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
	"unauthorized":                 {"auth", 401, false, "pass a valid bearer token (KUBONA_API_TOKEN)"},
	"forbidden":                    {"auth", 403, false, "use an operator-authorized credential for this action"},
	"rate_limited":                 {"auth", 429, true, "back off and retry after the limiter window"},
	"bad_request":                  {"validation", 400, false, "fix the request shape (see message details)"},
	"approval_required":            {"validation", 400, false, "approve the exact workflow with POST /api/workflows/approve then pass approval_token"},
	"anchor_missing":               {"validation", 400, false, "record measured job evidence or an explicit general_knowledge assertion before accepting the verified claim"},
	"ghost_session":                {"validation", 404, false, "create the session first; ids are never auto-vivified"},
	"not_found":                    {"validation", 404, false, "the named object does not exist: list first, check the id"},
	"unknown_rail":                 {"validation", 400, false, "use a documented rail name (see message)"},
	"method_not_allowed":           {"validation", 405, false, "use the documented method for this route"},
	"ceiling_exceeded":             {"budget", 403, false, "raise the tenant ceiling or shrink the request (fewer iterations)"},
	"budget_exceeded":              {"budget", 200, false, "lower max_budget_usd or raise the tenant ceiling (rides a 200 valid:false envelope, not an error status)"},
	"mandate_invalid":              {"state", 200, false, "mint a fresh mandate: bind the tenant admin key, exact parameters, and a live expiry (rides a 200 valid:false envelope)"},
	"request_conflict":             {"state", 409, false, "same id with different content will never succeed: use a new request_id"},
	"request_in_progress":          {"state", 409, true, "the claim is still running: poll the same request_id again"},
	"storage_blocked":              {"resource", 503, true, "free disk above the resume threshold, then retry (reads stay available)"},
	"resource_constraint":          {"resource", 503, false, "shrink inputs or page evidence via evidence_continue"},
	"not_ready":                    {"state", 503, true, "wait for readiness (disk recovery, bridge connect) then retry"},
	"bridge_unavailable":           {"upstream", 502, true, "retry when the Python bridge is reachable; sim-only paths stay available"},
	"llm_unavailable":              {"upstream", 503, true, "retry when a provider is configured; fallback paths stay open"},
	"unknown_tool":                 {"validation", 400, false, "call a tool from the pinned catalog (/api/capabilities)"},
	"request_too_large":            {"validation", 413, false, "reduce the request body or raise KUBONA_MAX_REQUEST_BODY_BYTES within the hard ceiling"},
	"revision_conflict":            {"state", 409, false, "the record changed since you read it: re-read it and re-apply your edit on the current revision"},
	"precondition_required":        {"state", 428, false, "send expected_revision from your last read of this record"},
	"unpriced_model":               {"validation", 400, false, "pick a priced model in Model Settings for metered loops (unknown models never bill)"},
	"credits_unavailable":          {"budget", 402, false, "add World Labs credits, then retry the same idempotency key (no duplicate spend)"},
	"spend_limit_exceeded":         {"budget", 402, false, "raise the Atlas account/project monthly limit or wait for reset, then retry the same idempotency key"},
	"task_not_entitled":            {"upstream", 404, false, "enable the Atlas task on this account/project or request beta access"},
	"internal":                     {"internal", 500, false, "report the incident id; do not blind-retry mutations (use idempotency keys)"},
	"server_misconfigured":         {"internal", 503, false, "set the missing server configuration (see message) and restart"},
	"proposal_stale":               {"state", 409, false, "the campaign or calibration moved since this plan was priced: re-read the campaign and authorize the fresh proposal"},
	"proposal_not_offered":         {"state", 409, false, "this proposal was already launched, dismissed, or failed: mint a new one from the current plan"},
	"proposal_digest_mismatch":     {"validation", 400, false, "the digest does not match the server-minted body: re-read the proposal before launching"},
	"proposal_unmintable":          {"validation", 400, false, "the plan cannot be priced or routed honestly (missing circuit, unknown backend, or unpriced metered hardware): revise the plan"},
	"loop_not_running":             {"state", 409, false, "steering applies only to a running run: resume it first (paused and terminal runs never consume instructions)"},
	"delete_blocked":               {"state", 409, false, "settle or abort the named work, then delete again; nothing was removed"},
	"campaign_missing":             {"state", 409, false, "run an experiment so the session has a campaign projection before creating a manuscript"},
	"campaign_invalid":             {"validation", 400, false, "the stored campaign projection is not decodable: re-create the campaign"},
	"source_changed":               {"state", 409, false, "the manuscript source changed since the draft was stored: create a new draft rather than rebuilding a stale one"},
	"content_hash_mismatch":        {"validation", 409, false, "sign the manuscript's current content hash (read it from the manuscript detail)"},
	"review_not_accepted":          {"state", 409, false, "deposit staging requires an ACCEPT review; address the review critiques and re-review"},
	"signoff_required":             {"state", 409, false, "record an operator signoff over the content hash before staging a deposit"},
	"signoff_expired":              {"state", 409, false, "re-sign the manuscript with a future expiry before staging a deposit"},
	"build_required":               {"state", 409, false, "build the manuscript first: deposit staging packages the built artifacts"},
	"build_unavailable":            {"upstream", 503, true, "install the TeX toolchain in the bridge image or retry when it is available; no PDF is produced without it"},
	"build_failed":                 {"upstream", 502, false, "read the build log in the response and fix the LaTeX source or figure; the build is deterministic and will not fix itself"},
	"reviewer_key_unconfigured":    {"internal", 503, false, "commission KUBONA_REVIEWER_PRIVKEY_HEX on the deployment host before signing manuscripts"},
	"annotations_open":             {"state", 409, false, "resolve or dismiss every open manuscript annotation before signing off"},
	"manuscript_immutable":         {"state", 409, false, "only a draft can be edited; create a new draft for changed content"},
	"artifact_ceiling_exceeded":    {"resource", 413, false, "delete old drafts or raise KUBONA_MANUSCRIPT_ARTIFACT_CEILING_BYTES; the session artifact ceiling was reached"},
	"comparator_unconfigured":      {"internal", 503, false, "set KUBONA_COMPARATOR_PUBKEY_HEX from the operator-run comparator key before attesting a kernel proof"},
	"publication_evidence_present": {"state", 409, false, "export the session's manuscripts with kubonad manuscript-export, then retry with ?acknowledge_publication_loss=1"},
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
func needDetailsFor(code string) map[string]any {
	switch code {
	case "ceiling_exceeded", "budget_exceeded", "spend_limit_exceeded":
		return map[string]any{"kind": "budget_exceeded", "reason": "the spend ceiling refused this work"}
	case "approval_required":
		return map[string]any{"kind": "approval", "reason": "a human decision is required before this can proceed"}
	case "anchor_missing":
		return map[string]any{"kind": "anchor_missing", "reason": "no measured or explicit general-knowledge anchor is recorded"}
	default:
		return nil
	}
}

func WriteAPIError(w http.ResponseWriter, code, message string, details map[string]any) {
	if need := needDetailsFor(code); need != nil {
		if details == nil {
			details = map[string]any{}
		}
		if _, exists := details["need"]; !exists {
			details["need"] = need
		}
	}
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
