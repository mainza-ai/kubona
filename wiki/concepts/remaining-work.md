---
type: concept
name: Remaining work queue
date: 2026-10-17
updated: 2026-10-17
tags: [plan, queue, production]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Remaining work queue

> Every item the program still owes, with its owner and unblock
> condition. Landed work is NOT here (see [[full-tree-roadmap]] Landed,
> [[rea-adoption-plan]], [[cloudroom-adoption-plan]]). Durable blueprint
> questions live in [[questions]] — this page tracks execution leftovers
> only. Screened-out items name their reason; nothing rots silently.

## A · Operator deeds (blocked on the human — no slice can deliver these)

- Revoke the live `gho_…` token in `~/.git-credentials` + drop the
  `credential.helper=store` helper (plaintext live credential).
- Q-27 history flip: rotate the store token first, then `filter-repo`,
  force-push, re-clone.
- Advance `main` to `develop` (standing convention; needs explicit order
  each time — never assumed).
- Commission the tenant reviewer key (`KUBONA_REVIEWER_PRIVKEY_HEX`) —
  the reviewer path runs ephemeral-only until then.
- QPU credentials + backends: `KUBONA_IBM_QISKIT_TOKEN`, backend choice,
  IBM limits, vendor accounts, GPU host.
- `KUBONA_CONTACT_EMAIL` (console mailto + polite-pool), HF-token rotation,
  EPO registration.
- First paid QPU run (human AP2 mandate per Q-21).
- DeepSeek-vs-oMLX transcript delta (keyed providers; harness ready).
- Device/emulator recalibration (keyed hardware; canary verifies
  live-vs-declared meanwhile per Q-14).
- Protogen venv re-pin (`grpcio-tools` 1.71.x) + regen Python pb2 stubs
  (`EvidenceEnvelope` message waits on it).
- Confirm or reverse the recorded R-series defaults: R1 fields
  (+mandate/tenant), R2 authority (tribunal quorum), R0 lanes
  (fast+boundary required), P1-4 timing.
- R6 gating policy: scores are recorded, never enforced — set the
  threshold (if any) before any gate reads them.
- Failed-mandate review queue is steady-state ops: open obligations
  without settlement are the queue; refund primitive + close rules exist.

## B · Queued programs (unblocked, beyond a single slice)

- UI/Docker production program (report 2026-10-17: U0 → D1 → U1 → U2 → U3 → D2).
  > ✅ U0 LANDED: console tests + lint in CI (19 lint errors fixed:
  > 6 render-phase key-change resets — no stale-session bleed — + 13
  > typed test casts); quantum-runner == pins measured from the shipped
  > image + rebuilt + import-verified; openbao digest-pinned + dev-only
  > header.
  > ✅ D1-skeleton LANDED: Dockerfiles for kubonad (CGO/sqlite,
  > nonroot, healthcheck) / console (standalone, CI-identical toolchain,
  > nonroot) / bridge (pinned extras, grpc healthcheck); `.dockerignore`
  > ×3; `compose.prod.yml` (health-gated, resource-limited, vault +
  > pgvector deliberately absent); CI `docker-smoke` job wired into the
  > release gate. Live findings fixed: fresh state roots never became
  > ready (daemon now ensures the models dir, regression-tested);
  > plaintext-compose needs the explicit opt-in the server demands.
  > Next: U1 (health banner, tenant writes, loop override, metasearch UI).
  > ✅ U1 LANDED 2026-10-17: backend status strip (down vs degraded vs
  > healthy, 15s poll — ends "empty vs down" ambiguity); loop
  > pause/resume/kill roster in the recorder (status-gated buttons,
  > confirm on kill, reasons verbatim, proven live pause→resume→kill);
  > metasearch approve/reject/apply panel in the cockpit (+ the missing
  > reject client; signer `console-operator`; proven live list-shape +
  > gated refusal); LabsView tenant writes (ceiling/suspend/refund +
  > admin bootstrap) with byte-exact admin envelopes (Go-generated
  > golden hashes, 4 vectors; full live cycle on compose: bootstrap →
  > ceiling → wrong-key 403 → suspend → frozen-write 403 → resume →
  > no-bearer 401). Next: U2 (wire-or-decide per dead backend).
- P1-4 subsystem consolidation execution — design: [[agent-subsystem-consolidation]].
  > ✅ STEP 1 LANDED 2026-10-17: conformance freezes on all three
  > registry homes (Go + 2× Python) + boot-crash hardening
  > (negative-proven). Step 2 scoped: wire-shape freezes both sides,
  > canonical mapping left for the operator (three Go vocabularies,
  > not one). Steps 3–5 await operator scheduling + identity-home
  > decision. Page status stays `candidate` — correctly.
- Lean skill slices L1–L5 (plan: [[leanrea-adoption-plan]] §3, conflicts:
  [[leanrea-conflicts-and-remediation]] — S-sized, ordered L1 → L2-baseline
  → L3 → L2-rerun → L4 → L5 (LC3: baseline before change), none touch the
  trust boundary; distribution gated on the public flip (LC1).
  > ✅ L1 + L2-baseline LANDED 2026-10-17 (Phase L-A exit met):
  > `skills/kubona/SKILL.md` + pins, `bench/` harness + first numbers.
  > ✅ L3 + L2-rerun + L4 LANDED 2026-10-17 (Phase L-B exit met):
  > scribe tags (convention, R3 gate unchanged), bench delta zero on
  > record, doctor deepening + `mandate-check` (LC5 wording contract).
  > ✅ L5 LANDED 2026-10-17 (Phase L-C exit met): MCP catalog +
  > 29 keep-verdicts (P2-13 closed) + unscoped-tool guard + `not_found`
  > code. Lean program COMPLETE — all phases exited green.
  L1 `skills/kubona/SKILL.md` (router + traps + first-moves, ≤12 KB
  pinned); L2 `bench/` harness with answer keys + ctx-budget gate;
  L3 scribe seen/inferred/unknown tags (convention, R3 gate unchanged);
  L4 doctor deepening + `mandate-check` first-move; L5 per-tool
  justification review + MCP catalog (closes P2-13).
- Bundle export endpoint — needs run-archival design (post-run turns
  cannot be reconstructed from events+chat; collective-level flow exists).
- Sidecar rotation — needs chain-aware reads (rotation breaks the
  tail-chained ledger assumption).
- S8c full hub — needs skills/packs/automations backends (reduced: no
  theater built).
- Steer — needs engine surgery (documented N/A: synchronous model has
  no background turn to steer into).
- ~~Assess principal threading on the remaining MCP servers
  (cost/execution/provenance write paths)~~ — ASSESSED 2026-10-17, no
  change (by design, threat argument): control already threads
  `requester` with label scoping (`test_control_tenants.py`); provenance
  reads thread `requester` via `readable()` with ambient resources
  public-only; provenance writes take caller-stated `label` (default =
  file's own tenant, files sharded per tenant, label bound into the
  content hash so forgery breaks validation); cost/execution run a
  possession model (unguessable `uuid4` session/job ids) + gateway scope
  gates for WHO — consistent with the `step_loop` precedent (possession
  of loop_id, documented). Known limit, stated: the gateway never binds
  `principal.sub` → `requester`, so label isolation is honest-caller +
  file-sharding + scopes, not cryptographic binding — the documented
  local-deployment model. Re-open if MCP serves remote cross-lab
  clients.
- Deferred batch: IdP (Phase 4), human lab membership (Phase 4),
  pgvector activation, sharding, full LLVM lowering, KEMTLS re-eval,
  CKT, Cirq/Q#/Serverless/Braket/rQOPS/CQM providers, GQE transformer,
  Bemba/Typst surfaces, quantum multi-geometry, loop load/chaos tests,
  cgroup kill, peercred sidecar (Q5 Linux hardening), workspace-files v2.
- Lean image — blocked on toolchain (no `lake`/`elan`, image
  unresolvable); comparator stays AST-first.

## C · Conditional (wire when a consumer exists)

- Python↔Go mandate crossing — bytes agree three-way (W0.9 vector);
  both rails mint and verify their own by design. Wire only if a flow
  needs cross-rail mandates (none does today).

## D · Standing questions

- [[questions]] sections A–E (7 open-question pages) + Q-27 — the
  insight work-list, separate from this execution queue.

Related: [[questions]] · [[roadmap]] · [[risks]] · [[full-tree-roadmap]] ·
[[rea-adoption-plan]] · [[cloudroom-adoption-plan]] ·
[[agent-subsystem-consolidation]]
