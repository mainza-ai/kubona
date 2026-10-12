---
type: concept
name: Deep Line-by-Line Implementation Audit 2026-10-12
date: 2026-10-12
updated: 2026-10-12
tags: [audit, implementation, gaps, verification, production]
sources: [source-gap-analysis-plan, source-architecture-paper, source-frontier-repo-audit-2026, source-agent-protocol-landscape-2026, source-openai-math-corpus, source-cloudflare-economy]
status: tracked
---

# Deep Line-by-Line Audit (2026-10-12) — code read, not sampled

Follow-up to [[full-wiki-audit-2026-10-12]] (113/113 pages read). This
pass read the implementation itself: `raw/` foundation docs in full,
dev-docs planning corpus in full (`implementation_plan.md`,
`master_execution_prompt.md`, `MASTER_AUDIT_REPORT.md`,
`kubona_wiki_vs_code_comprehensive_audit_report.md`, gap/blueprint
revs), then subsystem-by-subsystem code verification with live probes
on the parked stack (Go `:8080`, bridge `:50051`, oMLX `:8787` back
online, 11 models incl. OrcaRouter-27B; daemon stays on cloud deepseek
per speed directive — local-first restorable via settings).

Notation: ✅ completed · 🟡 partial · ❌ absent · 🔧 poorly
implemented (needs revision). Every item has file:line or a live probe.

## A. Completed (verified working)

- **Governance:** L0–L3 ladder + envelopes + cumulative reserve/release
  (tenant + session) + UCAN 1.0 via real `go-ucan v1.2.0` + revocation
  propagation + admission on every entry + parity both rails + bearer +
  limiters + flock + lease-HMAC/PG-DSN env + pre-commit guard.
- **Epistemics:** sources gating, withheld collapse, anchor repair,
  claim records/citations, contradiction check, grammar + tiers,
  wavefunction, hash-embedding curvature (`epistemics/curvature.py`),
  continuum + Fisher + NegativeCertificate, 20 anchor families tested,
  comparator gate + sandbox precedence.
- **Compute:** 14/14 live PySCF matrix (re-measured), Aer dispatch,
  earned consensus (live cloud proof: anchor-citing turns + DSML calls,
  TRUE legitimate; 1s-timeout fallback→false holds), classical PES
  sweep, real PennyLane `qcut` cutting, ZNE + refit, 5 Go harnesses
  (ansatz, QAOA, mitigation, compilation-pass, correctness) + Pareto +
  git branches + stagnation→proposal trigger + ledger events.
- **Evals/corpora:** Katas, HumanEval 136 + named exclusions, codeexec
  live-proven, truncation/schedule/critics, advantage checks, canary +
  nightly green, single `release_gate()`, budget/sim-first/WBP
  behavior evals.
- **Protocols:** MCP 2026-07-28 + 5 stdio servers + MRTR approvals,
  A2A internal + revoke + per-tenant sessions, x402 dormant-correct,
  Bao client + per-call sourcing, stdlib `crypto/mlkem` + noble
  kyber bilateral.
- **Console honesty (post-fix):** Illustrative badges, CONTESTED badge,
  COMPLETED-status table, abort wired, Action Gate (local sign +
  server verify, fail-closed), labs/literature/memory/chat batches,
  export/search/copy/attach/polls, BibTeX copy + DOI-never-here,
  Zenodo staged-never-published, manuscript ACCEPT≥9.0/hygiene-10 +
  NaCl sign-off in `manuscript.py`.
- **Rust:** qir-core parse + textual IR + `--json` + tests; ifc-core
  lattice + tests; CI builds. `native.py` Rust-binary-first with
  honest engine labels.
- **Ops:** pre-commit hook, math-only `.gitmodules`, LICENSE, anchored
  `.gitignore`, private repo, nightly green, quantum-runner images,
  hermetic venv, contents:read CI.

## B. Partial (works with caveats)

- 🟡 **Krylov non-H2 path** (`bridge/krylov.py:88-108`): analytical
  `fci + 0.05/(k+1)` schedule + hardcoded `meyer_wallach: 0.85`;
  replan folds MW>0 and marks trajectory `measured`. H2 path is a
  real numerical solve (expm + Hc=ESc + 1e-7 + real MW). Fix: `solver`
  flag in response; fold MW + `measured` only for numerical.
- 🟡 **Metasearch chain** (`pkg/metasearch`, `server.go:2565-2645`):
  propose/review/list wired + stagnation trigger + ledger events —
  but approve is a self-asserted string (defaults
  `human-quantum-reviewer`), routes have no bearer, proposals are
  in-memory only, and Apply flips status + resumes loops without
  writing anything to disk. Python MetaScope/MetaLoop tested but
  uncalled in production. Fix: bearer + real signer binding on
  approve/apply; persist proposals; make Apply write or say preview.
- 🟡 **Go mesh route** (`pkg/agents/custom.go:173-217`, served at
  `/api/mesh/route`): keyword model pick (`femoco`→Claude) with no
  mandate/budget check while claiming "AP2 mandate backing"; stale
  model names; `AirGapEnforced:true` on the non-air-gap default.
  The unserved Python router enforces correctly — port its gate.
- 🟡 **Console mandate tenant:** ActionGate + widget use
  `session?.id` / hardcoded `tenant-default`; `session.tenant_id`
  exists in types. Mandates verify cryptographically then die at
  admission on unknown tenant. Fix: use `session.tenant_id`.
- 🟡 **ADMET/discovery exec sites** (`server.go:2311,2380`): no
  timeout, CWD-relative `platform/bridge` fallback, bare-python3
  fallback; Aspirin/candidate defaults. Fix: timeouts, path
  resolution, default labeling.
- 🟡 **Canvas layout "persistence"** (`server.go:2092-2116`): in-memory
  map, lost on restart — log claims SQLite table. Fix: persist or
  correct the claim.
- 🟡 **Loops endpoint global-only** (`server.go:1884-1888`); header
  counts global loops. Fix: session filter + scoped display.
- 🟡 **Python console_api (37 routes, :8000 closed):** full shadow of
  Go (~20 dupes) + 17 with no Go equivalent (labs, models, reports,
  agents, settings, workflows, science/*, approvals, provenance).
  Nothing serves it; nothing references :8000. Decide: serve, merge,
  or retire (Q-17 table implications).
- 🟡 **Go custom-agent registry:** in-memory, no auth on
  create/delete, no escalation check visible. Fix: persist + gate +
  creator-capability check.
- 🟡 **Triage engines** (Go + Python agree at 50): tested, zero live
  callers — advisory only; wiki page says ≤36 (stale). Wire into
  pre-QPU path or label advisory; fix threshold + `EstimatedCredits`
  (computed, consumed nowhere).
- 🟡 **Shadows module** exists, served only via unserved
  science/shadows; widget illustrative. Fine if labeled; wire or note.
- 🟡 **Cloud LLM spend:** token budgets only, no dollar cap — tribunal
  loops on cloud models burn the operator account uncapped. Fix:
  per-provider cost model + session dollar ceiling for LLM spend.
- 🟡 **Qiskit provider:** SamplerV2 only; no EstimatorV2, no ISA
  transpile — VQE-on-hardware path incomplete.
- 🟡 **QIR reality:** text IR only (bitcode open), no MLIR authorship
  yet, no openqasm3 Rust frontend, no WASM/PyO3 — pipeline works via
  Go Analyze + portfolio + Rust text-IR. Honest; keep scoped.
- 🟡 **Lean:** AST-first + container→host precedence correct; image
  unbuilt (operator); Deltakit absent (Stim-direct fine).
- 🟡 **oMLX up, daemon on cloud** (speed directive); local-first
  restorable. Llama-3.2 direct completion proven LOCAL-OK.

## C. Absent (missing functionality)

- ❌ **Manuscript verification gating (P0-class):** ResultsStudio
  manuscript/LaTeX/Lean tabs unconditionally claim convergence,
  chemical-accuracy satisfaction, "machine-checked Lean 4 with 0
  sorryAx" + canned `sorryAx_free_check` proof — from campaign
  numbers with no prover run. Only nulls render UNVERIFIED; wrong
  numbers render as verified. `manuscript.py` enforcement exists but
  is uncalled. Fix: gate prose on claim-record verdict / folded
  measured delta, or label DRAFT-projected; wire studio to
  `manuscript.py` review.
- ❌ Slash `/clear /budget /anchor`, mode selector, HUD stop button,
  home feed, arg-normalization inspector, presence/cursors, Mol*,
  Yjs, WebAuthn, counterfactual forking, 3D terrain, ledger-link
  clicks in studio, annotation persistence (local-only), unified
  search index, provenance browser surface, bidirectional code↔diagram.
- ❌ Quantum multi-geometry scans, loop load/chaos tests, A2A external
  transport, per-tenant sharding, syndrome pipelines, IdP, membership,
  horizontal scale, photonic/ocean credentials, PEC/CDR, Cirq/Q#/
  Serverless/Pilot, Braket (+Program Sets/off-peak), rQOPS estimator,
  CQM annealing, GQE transformer, RL/recipes, Bemba/Typst/Isabelle
  (dev-docs claims — none exist anywhere).
- ❌ Staging gates with no evidence: nightly never caught a
  regression (P3 needs it), correctness-in-staging unproven,
  hardware-validated library entry absent, vendor grading unrun.
- ❌ CI secret scan (wiki demands it; only local pre-commit exists).
- ❌ TLS-bind config test (promised in "done looks like").
- ❌ Suspended-status mutator, PG/HNSW wiring, `useTelemetry`
  consumption, `@xyflow/react` verdict — see Q-33/34 + H13.

## D. Poorly implemented (revision list)

1. **Mesh route gate missing** (B above) — port Python gate.
2. **Meta approve/apply auth** (B above) — bearer + signer binding.
3. **Manuscript prose gating** (C above) — the largest remaining
   fiction surface.
4. **Mandate tenant attribution** (B above) — one-line + widget prop.
5. **Exec-site hardening** (B above) — timeouts + paths + labels.
6. **Canvas persistence claim** (B above).
7. **Krylov attribution flag** (B above).
8. **Triage wiring/threshold/credits** (B above).
9. **Custom-agent registry** (B above).
10. **Cloud-LLM dollar cap** (B above).
11. **Python shadow API decision** (B above) — serve/merge/retire.
12. **Prior-audit hygiene:** MASTER_AUDIT (ML-DSA error, invented
    surfaces/pages, stale paths) and Antigravity (6 dead paths rated
    🟢, incl. the deleted `pkg/mcp/gateway.go`) must not be inherited
    — this page supersedes both for build decisions. Forensic page
    already carries restructure corrections.
13. **Wiki fixes H1–H15** (previous audit) + anchor-family count
    (wiki names 8, code implements 21 tested — update page with
    corpus-fidelity caveat; 287 extraordinary → label corpus-claimed).
14. **Doc drift:** `kubona.md`/risks/design-evolution Dart remnants;
    forensic `10001:10001` vs code `1000:1000`; MockSource
    `offline=False` on fixture; `?? 0.12` gate-card estimate label.

## E. Updated implementation plan (supersedes G/H/I with this detail)

- **Phase J (P0 fiction + auth, ordered):** J1 manuscript gating +
  studio→`manuscript.py` wiring · J2 mesh-route gate port · J3
  meta approve/apply bearer + signer binding · J4 mandate tenant
  attribution · J5 krylov solver flag + fold gating.
  - [x] **J1 landed 2026-10-12:** (see previous entry — render tests
    caught 2 further ungated badges, fixed same slice).
  - [x] **J2 landed 2026-10-12:** `RouteMultimodal` cloud branch now
    requires `HasEd25519Mandate || SpendBudgetUSD > 0.05` (Python
    gate faithfully ported); "mandate backing" named only when a
    mandate exists; `AirGapEnforced` only on the true air-gap
    branch. Unit matrix (unfunded→local, funded→cloud,
    mandated→cloud+named, air-gap→enforced, default→unenforced)
    green; pre-existing HTTP mesh test (budget 5.0) still passes.
  - [x] **J1 landed 2026-10-12:** manuscript/LaTeX/Lean prose gated on
    `epistemics.status === COLLAPSED_VERIFIED` (claim) and recorded
    proof receipt (Lean — never set today, so honestly absent);
    unverified renders PROJECTED DRAFT + "Projected Research Draft"
    + "Variational Exploration Active" + no-prover-run text. The
    render tests caught 2 more ungated sites in the same component
    (header "Verified Research Finding" + "Chemical Accuracy Met"
    badges, "Zero sorryAx (ρ=1.000)" pill) — fixed in the same
    slice. 76/76 console green, build clean.
  Acceptance: each proven by a would-be-caught test (wrong-number
  manuscript refuses/marks; keyword route without mandate refuses;
  self-asserted approve 401s; session-tenant mandates admit past
  verify; analytical MW never folds).
- **Phase K (robustness + shadow API):** K1 exec-site timeouts/paths/
  labels · K2 canvas persist-or-correct · K3 loops session scope ·
  K4 custom-registry persist/gate/escalation · K5 triage wire-or-label
  + threshold sync + credits wired-or-removed · K6 cloud-LLM dollar
  cap · K7 Python shadow decision (serve/merge/retire) + MCP stdio
  role clarified · K8 CI secret scan + TLS-bind test · K9
  `@xyflow/react` verdict + `.db.lock` helper coverage.
  Acceptance: tests + live proof per slice, CI green.
- **Phase L (operator queue, in order):** oMLX-as-default switch
  (or documented cloud-default) · Lean image + warm · reviewer/API/
  contact env · EPO registration · vendor grading · review
  commission · paid QPU run · Q-27 flip. Acceptance: receipts.
- **Non-goals (stay VISION):** §C lists above except J/K items;
  KEMTLS; sharding; syndromes; IdP; membership; scale; Bemba/Typst
  (dev-docs-only claims, no code anywhere).
