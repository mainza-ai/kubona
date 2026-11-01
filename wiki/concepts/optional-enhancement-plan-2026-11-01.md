---
type: concept
name: Optional enhancement plan (2026-11-01)
date: 2026-11-01
updated: 2026-11-01
tags: [plan, api, accessibility, operations, production]
sources: [source-architecture-paper, source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Optional enhancement plan (2026-11-01)

> Scope: work that improves the **local-private production** platform after
> the current runtime, vendor-evidence, contract, and automated-accessibility
> bar is green. None of these items blocks the supported single-operator
> deployment. They are ordered by risk and by how cheaply they can be proven.

## Current baseline

- Local-private stack: fresh-volume preflight, health/ready, console, local
  simulator, SSE replay, backup/restore, bridge-restart persistence, and
  measured IBM settlement are verified.
- Runtime rate card defaults to the reviewed vendor-list card.
- IBM Open Plan/free is established through the authenticated Qiskit Runtime
  API; the historical local `$0.00` row stands.
- Contract inventory: 89 modeled / 40 client-only / 0 unmodeled, 85 schemas,
  84 OpenAPI paths.
- Axe WCAG 2 A/AA passes Arena, Labs, Settings, and Circuit Codesign.
- CI is the 20-job push gate plus Console E2E; run IDs live in `AGENTS.md`.

## E1 — API versioning (highest compatibility value)

> ⚑ State (2026-11-01): the `/api/v1/...` prefix alias and
> `Kubona-API-Version: 1` response header are landed; the compatibility
> `/api/...` paths and route policy are unchanged. Generated client/versioned
> OpenAPI gating remains open.

**Problem.** The console and agent clients share unversioned `/api/...`
paths. Generated OpenAPI is a snapshot, not a negotiated contract. A future
breaking response change has no explicit migration seam.

**Approach.**

1. Keep `/api/...` as the compatibility alias for the supported local
   release. Do not break existing integrations.
2. Add `/api/v1/...` as the explicit versioned surface, backed by the same
   handlers and the same route policy table.
3. Emit `Kubona-API-Version: 1` and `Deprecation`/`Sunset` headers only when
   a legacy path is scheduled to move. Never invent dates.
4. Generate OpenAPI per version (`/api/v1` canonical), keep the compatibility
   document separate, and gate both inventories in CI.
5. Add a contract test that every `/api/v1` path maps 1:1 to a registered
   compatibility route and that no handler is reachable only through the
   unversioned alias.
6. Only after a stable local release, decide whether `/api` redirects,
   aliases, or remains forever. That is a separate decision; no redirects in
   v1.

**Acceptance.** Existing console/E2E unchanged; `/api/v1` smoke passes for
every path in the generated inventory; breaking-change test fails closed when
a versioned response schema drifts.

## E2 — External alert routing beyond preflight/backup

**Problem.** `KUBONA_ALERT_WEBHOOK_URL` currently covers preflight and
scheduled-backup failures. Disk pressure, rate-card expiry, settlement
mismatch, and SSE/stream health are visible only in UI/logs. An operator who
is not watching the console can miss them.

**Approach.**

1. Define one local alert envelope: `kind`, `severity`, `code`, `summary`,
   `evidence` (no prompts/keys/session payloads), `at`, `dedupe_key`.
2. Add sources in priority order:
   - rate-card stale/near-expiry;
   - disk guard warning/blocked/resumed;
   - backup failure/stale (already present, migrate to the envelope);
   - settlement mismatch (vendor usage vs local actual);
   - SSE stream closed/unavailable (console-side).
3. Dispatch to a **single webhook sink** with bounded retries, jitter, and a
   per-`dedupe_key` cooldown. Never log the webhook URL or alert secrets.
4. Keep alerts opt-in and default-off; add a local dry-run that prints the
   envelope and exits without network egress.
5. Test the dispatcher with an `httptest` receiver and a failed-delivery path;
   no external service in CI.

**Acceptance.** A forced rate-card/disk/settlement condition produces one
correct envelope, a retry, and a cooldown duplicate suppression. With no
webhook configured, behavior is unchanged.

## E3 — Deeper assistive-technology audit

**Problem.** Axe covers automated WCAG A/AA on four core surfaces. It cannot
judge screen-reader flow, canvas equivalence, or whether dynamic announcements
are useful rather than noisy.

**Approach.**

1. Manual scripted passes with VoiceOver (macOS) and NVDA (Windows), recorded
   as pass/fail notes per surface: Arena, copilot, Labs, Settings, Codesign,
   Flight Recorder, tribunal.
2. Add a visible, keyboard-reachable text alternative for the spatial canvas:
   current selected point, bond distance, energy, anchor status, and camera/
   layout state. No visual-only meaning.
3. Review live regions: one announcement per completed event batch, not per
   token/frame; ensure toasts and errors are not duplicated by the banner.
4. Check tables/progressbars/tabs for names, roles, values, and focus order;
   add E2E assertions where they can be automated.
5. Add a reduced-motion/manual checklist result to CI-adjacent docs, not a
   fake automated pass.

**Acceptance.** The scripted manual pass is recorded; every control has an
accessible name; the canvas has a text-equivalent state; announcements do not
repeat on every poll.

## E4 — Deferred optional hardening

- Encrypted-at-rest ledger / KMS-backed secret injection (requires key
  custody design).
- Opt-in OTLP/Sentry exporters with scrubbing and sampling.
- API token rotation / dual-token overlap.
- Containerized stdio MCP broker only if the deployment shape changes.
- HA/multi-host operation is out of scope for local-private.

## Execution order

1. E1 version seam + OpenAPI split (largest compatibility risk).
2. E2 alert envelope + dispatcher (operational visibility).
3. E3 manual AT script and canvas text equivalent.
4. Re-evaluate E4 only when a real deployment requirement appears.

No item should be implemented by weakening an existing gate or by inventing
an external result. Each lands with tests, a runbook note, and a green gate.
