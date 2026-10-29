package api

// Route policy table: the single path→auth-class registry for every HTTP
// route. Threat (2026-10-26 audit follow-up): auth_matrix_test pinned
// ~10 of ~116 routes, so new spend/compute writers could land bearer-less
// without failing any test. This table enumerates ALL registered patterns;
// route_policy_test.go fails on any registered-but-unlisted pattern (and on
// any stale row), so adding a route without classifying it is red.
//
// Posture: default-deny for POST/PUT/DELETE/PATCH. A write-serving route is
// either gated at registration (bearer / bearer-writes / a2a) or carries a
// written Rationale justifying why it stays open (session-scoped in-handler
// capability gates, signature auth, or stateless spend-free compute). A
// public-read row must serve no writes at all — the test enforces that.
//
// Auth classes:
//   - bearer: requireBearer at registration (operator token or lab session).
//   - bearer-writes: requireBearerForWrites at registration — writes gated,
//     reads public (mixed settings/listing routes; the gate lives at
//     registration, never as an inner handler switch).
//   - a2a: requireA2AAuth at registration (operator token via Bearer or
//     X-Kubona-A2A-Token; x402 payment headers pass through).
//   - public-read: anonymous-safe reads only; serving a write fails the test.
//   - session-scoped: open edge, caller-vs-session/tenant binding inside
//     (D-09: authorizeKnownSession / authorizeSessionRead / checkCallerAuth).
//     Spend writers in this class additionally gate on autonomy + admission
//     + idempotency; the Rationale names the in-handler controls.
//   - signature: public edge, a cryptographic signature IS the auth
//     (outer bearer impossible — the caller is not ours).
//   - open-write: POST/PUT/DELETE left open with a written Rationale
//     (stateless, spend-free compute). Anything spend-adjacent must NOT be
//     here — it gets bearer or session-scoped instead.

import "strings"

// RouteAuthClass is the edge-auth class of one registered route.
type RouteAuthClass string

// Shared auth-class consts — one vocabulary for the table and its test.
// Never copy these literals elsewhere.
const (
	RouteAuthBearer        RouteAuthClass = "bearer"
	RouteAuthBearerWrites  RouteAuthClass = "bearer-writes"
	RouteAuthA2A           RouteAuthClass = "a2a"
	RouteAuthPublicRead    RouteAuthClass = "public-read"
	RouteAuthSessionScoped RouteAuthClass = "session-scoped"
	RouteAuthSignature     RouteAuthClass = "signature"
	RouteAuthOpenWrite     RouteAuthClass = "open-write"
)

// RoutePolicy is one row of the policy table.
type RoutePolicy struct {
	// Pattern is the exact string passed to s.route in registerRoutes.
	Pattern string
	// Methods served (comma form, e.g. "GET", "GET,POST", "POST").
	Methods string
	// Class is the auth posture (one of the RouteAuth* consts).
	Class RouteAuthClass
	// Rationale documents why the posture is correct. Required for every
	// row except pure bearer/a2a gates (the gate is the justification).
	Rationale string
}

// ServesWrites reports whether the row's Methods include a state-changing
// verb. Single source for the default-deny test — never reimplement.
func (p RoutePolicy) ServesWrites() bool {
	for _, m := range strings.Split(p.Methods, ",") {
		switch strings.TrimSpace(m) {
		case "POST", "PUT", "DELETE", "PATCH":
			return true
		}
	}
	return false
}

// RoutePolicies is the full path→auth-class table (one row per registered
// pattern, same order as registerRoutes). Adding a route = adding a row.
var RoutePolicies = []RoutePolicy{
	{Pattern: "/api/health", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "liveness probe, no state, no data."},
	{Pattern: "/metrics", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/ready", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "readiness probe, no state, no data."},
	{Pattern: "/api/capabilities", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "static capability advertisement, no state."},
	{Pattern: "/api/sessions", Methods: "GET,POST", Class: RouteAuthSessionScoped, Rationale: "List: D-09 caller scoping (anon single-lab only, lab callers own-lab, bearer all). Create: spoof-guarded (anon mints default-lab only with clamped budget; lab callers pin to own lab). No spend moves here."},
	{Pattern: "/api/sessions/refund", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/sessions/export", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Read-only manifest export; known sessions bind caller to session tenant (ghosts 404)."},
	{Pattern: "/api/sessions/import", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/sessions/context", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "Read-only context readout; known sessions bind caller to session tenant."},
	{Pattern: "/api/sessions/", Methods: "GET,POST,PUT,DELETE", Class: RouteAuthSessionScoped, Rationale: "Session CRUD, messages, claims, clear-chat: every known-session path binds caller to session tenant (ghosts keep legacy behavior). Destruction (clear-chat DELETE) takes the same gate as reads."},
	{Pattern: "/api/campaigns/", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "Flight-recorder reads bind caller to session tenant via authorizeKnownSession."},
	{Pattern: "/api/chat/messages", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Session content write: caller bound to session tenant (T-09c); LLM spend tracked per session, never reserved here."},
	{Pattern: "/api/chat/stream", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Streaming twin of chat POST: same caller binding, same spend tracking."},
	{Pattern: "/api/chat/regenerate", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Session content mutation: caller bound to session tenant before any row is touched."},
	{Pattern: "/api/chat/rewrite", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Session content mutation: caller bound to session tenant before any row is touched."},
	{Pattern: "/api/chat/feedback", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Session content mutation: caller bound to session tenant before any row is touched."},
	{Pattern: "/api/chat/tribunal", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Tribunal runs spend (LLM/sandbox): attributed session required (G7 ghosts 404), dispatch autonomy enforced, caller bound to session tenant, idempotent."},
	{Pattern: "/api/tribunal/resume", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Resumed runs spend on the session lab: same gates as fresh tribunal (session attribution, autonomy, caller binding, idempotency)."},
	{Pattern: "/api/chat/launch", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Spend dispatch (mints jobs): attributed session required, dispatch autonomy enforced, caller bound to session tenant, hardware admission (mandate/fidelity/routing) + idempotency inside. Open edge preserves the zero-login local flow; capability gates are the control."},
	{Pattern: "/api/jobs", Methods: "GET,POST", Class: RouteAuthSessionScoped, Rationale: "List: tenant-scoped reads (unscoped list refused, ghosts 404). Dispatch: attributed session, dispatch autonomy, caller binding, hardware admission + content-bound idempotency. Same local-flow posture as launch."},
	{Pattern: "/api/jobs/", Methods: "GET,POST", Class: RouteAuthSessionScoped, Rationale: "Read: job+scope session must share a tenant and the caller must clear the scope session. Cancel: known-job path binds caller to the job session before CancelJob."},
	{Pattern: "/api/loops", Methods: "GET,POST", Class: RouteAuthBearer},
	{Pattern: "/api/loops/", Methods: "GET,POST", Class: RouteAuthBearer},
	{Pattern: "/api/events", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "Session-keyed event reads bind caller to session tenant."},
	{Pattern: "/api/memory/recall", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/memory/retain", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/agents", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Static agent-role listing, no state, no spend."},
	{Pattern: "/api/fidelity", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Pure local fidelity-floor estimate (probe_run:false); no measurement, no spend."},
	{Pattern: "/api/epistemics", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "Session-keyed epistemic reads bind caller to session tenant."},
	{Pattern: "/api/fleet", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Measured liveness snapshot (TCP dials); read-only, no state change."},
	{Pattern: "/api/radar", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Local port/service probe snapshot; read-only, no state change."},
	{Pattern: "/api/ledger/verify", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/mandates/sign", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/mandates/verify", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/auth/challenge", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Auth bootstrap: no identity exists yet, so bearer is impossible. Mints a nonce only; human-paced limiter; no state beyond the challenge."},
	{Pattern: "/api/auth/login", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Auth bootstrap: challenge-response against the lab admin key. Human-paced limiter; every refusal ledger-sealed (guessing campaigns leave a trail)."},
	{Pattern: "/api/auth/logout", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Destroys only the caller's own session; always succeeds; CSRF-gated (cross-site use 403s without revoking)."},
	{Pattern: "/api/auth/whoami", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Auth introspection: returns authenticated:false anonymously; never discloses other labs."},
	{Pattern: "/api/auth/sessions", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "Lists only the caller's own lab sessions; session principal required (bearer carries no tenant to scope to, so it 401s)."},
	{Pattern: "/api/auth/revoke", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Revokes one of the caller's own sessions only; CSRF-gated; session principal required."},
	{Pattern: "/api/harnesses", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Static harness registry listing, no state, no spend."},
	{Pattern: "/api/results/notebooks", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "Notebook reads bind caller to session tenant when a session is named."},
	{Pattern: "/api/literature", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Bridged literature search with offline-cache fallback; fails soft (503) without a bridge; no spend, no writes."},
	{Pattern: "/api/autoresearch/harnesses", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Static autoresearch harness listing, no state, no spend."},
	{Pattern: "/api/autoresearch/start", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Loop spend: attributed session required (G7), dispatch autonomy enforced, caller bound to session tenant, worst-case spend reserved up front (I3), slow-rail idempotent."},
	{Pattern: "/api/autoresearch/loops", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Operational loop-state listing only (no transcripts); per-loop control lives under /api/loops/ (bearer) and /api/autoresearch/loops/ actions below."},
	{Pattern: "/api/autoresearch/loops/", Methods: "GET,POST", Class: RouteAuthOpenWrite, Rationale: "Loop step/pause/resume/stop/sleep draw against the reservation admitted at start (bounded by max_iterations + reservation); single-lab local flow. RESIDUAL: no caller-vs-loop-tenant binding — a cross-lab caller can drive another lab's loop within its pre-authorized budget. Bearer-gating this rail is the follow-up; the spend itself cannot exceed the admitted reservation."},
	{Pattern: "/api/metasearch/proposals", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Read-only strategy-proposal listing; mutations (approve/reject/apply) are bearer-gated."},
	{Pattern: "/api/metasearch/approve", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/metasearch/reject", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/metasearch/apply", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/llm/settings", Methods: "GET,PUT,POST", Class: RouteAuthBearerWrites, Rationale: "Reads are public snapshots; writes (provider/endpoint flips redirect prompts+secrets) gated at registration."},
	{Pattern: "/api/llm/models", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Provider model listing; no secrets, no state."},
	{Pattern: "/api/llm/test", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/quantum/settings", Methods: "GET,PUT,POST", Class: RouteAuthBearerWrites, Rationale: "Reads are public snapshots; writes (backend selection steers human runs at metered hardware) gated at registration. Secrets never accepted here."},
	{Pattern: "/api/quantum/backends", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Static backend catalog, no state."},
	{Pattern: "/api/quantum/jobs", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/quantum/jobs/", Methods: "GET,POST", Class: RouteAuthBearer},
	{Pattern: "/api/quantum/canary", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "TTL-cached liveness probe; fails soft to catalog values; read-only."},
	{Pattern: "/api/workflows/plan", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/workflows/run", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/vault/leases", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/vault/leases/revoke", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/a2a/v1", Methods: "POST", Class: RouteAuthA2A, Rationale: "Inter-agent delegation + x402 authorization; operator token via Bearer or X-Kubona-A2A-Token (Authorization header carries payment proof)."},
	{Pattern: "/api/a2a/v1/challenge", Methods: "GET", Class: RouteAuthA2A, Rationale: "x402 challenge mint; same operator rail as delegation."},
	{Pattern: "/api/events/stream", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "SSE over one session's events; caller bound to session tenant (errors surface as events once headers are sent)."},
	{Pattern: "/api/canvas/layout", Methods: "GET,POST,PUT", Class: RouteAuthSessionScoped, Rationale: "Per-session layout rows (256KB cap, SQLite-persisted); caller bound to session tenant for reads and writes alike."},
	{Pattern: "/api/observatory/lens", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/vault/predictions/seal", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/vault/predictions/evaluate", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/vault/predictions/latest", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/vault/predictions/assert", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/vault/predictions/calibration", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/vault/predictions/history", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/observatory/assets", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/observatory/lens/resolve", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/observatory/lens/history", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/observatory/webhooks", Methods: "POST", Class: RouteAuthSignature, Rationale: "Atlas delivery: outer bearer impossible (Atlas is not ours). The Ed25519 v1a signature IS the auth — verified before any state change; poll+resolve stays the correctness path."},
	{Pattern: "/api/chemistry/admet", Methods: "GET,POST", Class: RouteAuthOpenWrite, Rationale: "Stateless molecular scoring (bridge or local RDKit rail); no spend reservation, no jobs; demo defaults labeled, interpreter named. Ledger provenance event only."},
	{Pattern: "/api/discovery/pipeline", Methods: "GET,POST,PUT", Class: RouteAuthOpenWrite, Rationale: "Stateless candidate-pipeline scoring; same posture as ADMET (no spend, no jobs; ledger provenance event only)."},
	{Pattern: "/api/mandates/pqc-key", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Public-key publication (encapsulation key + fingerprint); the secret never leaves the server."},
	{Pattern: "/api/mandates/pqc-verify", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/qir/compile", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Pure local circuit analysis (no bridge, no QPU, no spend, no jobs); empty input refused; bounded input."},
	{Pattern: "/api/codesign/tq", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Read-only pre-scored entries; validity stays Python-owned."},
	{Pattern: "/api/codesign/frontier", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Read-only pre-scored entries; validity stays Python-owned."},
	{Pattern: "/api/codesign/evaluate", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Stateless evaluator proxy (bounded body, ctx timeout); Python owns validation; no spend, no jobs."},
	{Pattern: "/api/codesign/evaluate-batch", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Batch twin of evaluate: same stateless proxy posture."},
	{Pattern: "/api/codesign/families", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Stateless codesign op proxy: same bounded stateless posture as evaluate."},
	{Pattern: "/api/codesign/budget", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Stateless union-bound math proxy: same bounded stateless posture as evaluate."},
	{Pattern: "/api/codesign/verified", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Versioned result documents (numbers, not verdicts); read-only."},
	{Pattern: "/api/codesign/packs", Methods: "GET,POST", Class: RouteAuthOpenWrite, Rationale: "Pack registry served live; POST serves the same read as GET (raw-body passthrough, no mutation server-side); validity stays Python-owned."},
	{Pattern: "/api/krylov/solve", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/mitigation/apply", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/cost/settle", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/cost/summary", Methods: "GET", Class: RouteAuthPublicRead, Rationale: "Aggregate LLM cost estimates (heuristic, never metered billing); no per-session attribution recorded."},
	{Pattern: "/api/hf/search", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/hf/models/", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/hf/download", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/hf/downloads", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/hf/downloads/", Methods: "GET,POST", Class: RouteAuthBearer},
	{Pattern: "/api/agents/custom", Methods: "GET,POST", Class: RouteAuthBearerWrites, Rationale: "Listing stays public; registry create is an operator mutation gated at registration."},
	{Pattern: "/api/agents/custom/", Methods: "DELETE", Class: RouteAuthBearer},
	{Pattern: "/api/mesh/route", Methods: "POST", Class: RouteAuthOpenWrite, Rationale: "Stateless routing decision only (model choice + explanation): no jobs, no spend, no ledger writes, no secrets, no external fetch. Refuses (rather than invents) when unconfigured."},
	{Pattern: "/api/ledger/append", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/ledger/export", Methods: "POST", Class: RouteAuthSessionScoped, Rationale: "Ledger export binds caller to session tenant (unknown sessions refuse without disclosing existence)."},
	{Pattern: "/api/supervisor/abort", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants", Methods: "GET,POST", Class: RouteAuthSessionScoped, Rationale: "List: D-09 scoping (anon single-lab only; multi-lab anon 401s; lab callers see own row; bearer sees all). Create: open self-provisioning BY DESIGN (self-hosted labs; first SetAdminKey wins) behind a creation-spam limiter."},
	{Pattern: "/api/tenants/", Methods: "GET", Class: RouteAuthSessionScoped, Rationale: "By-ID read binds caller like the list: anon single-lab only, lab callers own row only (cross-lab 403), bearer all."},
	{Pattern: "/api/grants/register", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/grants/revoke", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenant-grants/issue", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenant-grants/revoke", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/obligations/open", Methods: "GET", Class: RouteAuthBearer},
	{Pattern: "/api/obligations/", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants/admin-key", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants/ceiling", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants/refund", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants/autonomy", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants/revoke", Methods: "POST", Class: RouteAuthBearer},
	{Pattern: "/api/tenants/status", Methods: "POST", Class: RouteAuthBearer},
}
