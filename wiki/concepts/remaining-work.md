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

- P1-4 subsystem consolidation execution — design: [[agent-subsystem-consolidation]].
- Lean skill slices L1–L5 (plan: [[leanrea-adoption-plan]] §3 — S-sized,
  ordered L1 → L2+L3 → L4 → L5, none touch the trust boundary):
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
- Assess principal threading on the remaining MCP servers
  (cost/execution/provenance write paths) — control plane done.
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
