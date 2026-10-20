---
type: concept
name: Circuit Co-Design UX Plan
date: 2026-10-18
updated: 2026-10-20
tags: [ux, console, codesign, frontier, observatory]
sources: [source-llm-circuit-codesign-2026, source-gap-analysis-plan]
status: adopted
---

# Circuit Co-Design UX Plan (backend → frontend, inclusive)

## Backend (landed slice 1)

- Python `platform/kubona/codesign/` (evaluator ground truth): `kickmix.py` (trajectory sim Eqs 10-16, `ε` Eqs 20-21), `tq.py` (shared `κ∈[1,3]`, golden `5×10=50`), `frontier.py` (MAP-Elites min-TQ/cell), `harness.py` (four-part contract, forged-artifact invalid), `passes.py` (MBU saving, routing detector — report only, rewrite stays in search loop).
- Python `console_api.py` (Python-owned, pending Go migration): `POST /api/codesign/evaluate` + `POST /api/codesign/frontier` (`and2/cz-phase/copy1`, `≤64` batch, fail-closed, read-only, no submission endpoints).
- Go `platform/core/pkg/codesign/` (serving mirror, not validity): `tq.go` + `frontier.go` (same golden pinned both sides), `pkg/api/codesign.go` `GET /api/codesign/tq` + `POST /api/codesign/frontier` (read-only, no spend, no jobs). Live-verified 2026-10-18 (`50` golden, HHL `1348×115=155020`).
- LLM: Strata local engine for any proposal calls (OpenAI-compat, default `http://127.0.0.1:47831/v1` overridable via `STRATA_BASE_URL`, model via `STRATA_MODEL`); NVIDIA parked (configured, explicitly selectable, not the default). No LLM calls in slice 1 tests (deterministic).

## Frontend (landed slice 1)

- `CodesignCockpit.tsx` (read-only): TQ scorer + MAP-Elites batch (≤64) + claimed-gains disclaimer. Talks only to Go `:8080`. **No job-submission endpoints** (hard rule #1 in [[ui-ux-vision]]).
- Wired as 4th `proofs` tab (`manuscripts/tribunal/gallery/circuits`) — no new dock icon this slice (minimizes SovereignDock/page/CommandPalette blast radius). Full dock surface deferred to slice 2 behind operator sign-off.
- Test `CodesignCockpit.test.tsx` asserts read-only (no `submit job` text).

## Full refactor (slices 2-5, queued)

1. **Slice 2 — Evaluator runs view:** per-trial ledger refs, `ε`-bound display with Wilson/Hoeffding note, failure-budget union-bound table (Table-S4 style), `κ` slider (`1→3`) + depth/volume toggles when factory data present.
2. **Slice 3 — Families view:** ≤5 ideation tracks, elite-sharing events, widen-nudge button (human nudge, never auto-prompt-injection), lineage graph (parent→child, non-monotonic allowed).
3. **Slice 4 — Packs:** `numerics → inversion (jstore slider 0/5/9) → DQI (BM/Lanczos/Ψ/stripes) → chemistry (frames, fallback intact) → gauge (4³ only)`, each `kubona-pack/v1` with anchor + evidence reqs.
4. **Slice 5 — Go migration:** move `console_api` codesign routes into Go via bridge RPC (Python stays evaluator); delete Python duplicates to honor single-surface rule. Needs operator sign-off (contract change).

## Threat model (reads public, writes gated)

Scoring reads stay public behind limiters (same as fleet/epistemics); no spend, no QPU, no secret touches. Forged artifacts invalidate, never score. Oversize batches refuse whole. Console never mints mandates or submits jobs from this surface.

See [[evaluator-guided-circuit-codesign]], [[failure-budget-and-frontier]], [[agent-ux-control-plane]], [[ui-ux-vision]].
