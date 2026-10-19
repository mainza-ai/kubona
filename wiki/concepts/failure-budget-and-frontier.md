---
type: concept
name: Failure Budget and TQ Frontier
date: 2026-10-18
updated: 2026-10-18
tags: [failure-budget, tq-cost, frontier, packs, magic-states]
sources: [source-llm-circuit-codesign-2026, source-magic-state-cultivation-2026]
status: adopted
---

# Failure Budget and TQ Frontier

## Failure budget (localized rare cost)

Find worst-case-driving rare inputs by distribution sampling (cf. 400M BM sims, 200M walks); reorganize so extra cost is separable (stack entries, iterations, lanes, windows); omit + fail only there within allowed `ε`. Every truncation carries its Table-S4-style union-bound row in the ledger: per-use rate × uses. `400M/200M/43M`-scale sims are the method, not an anecdote.

## TQ frontier

MAP-Elites `(T,Q)` grid; `jstore`-style sliders produce the frontier (`0:198hist/1065Q → 5:60Q → 9:24Q` for divsteps; `1616→1432` schedule-cut for Euclid). Console shows the frontier plot read-only (no job-submission endpoints). `κ` default stays `1` for paper-comparability; `κ∈[1,3]` adopted when qubit-scarce realism demands it; depth/volume variants when factory/cultivation data present (source-magic-state-cultivation-2026).

## Packs (dependency order)

`numerics-pack` → `inversion-pack` → `dqi-pack` → `chemistry-pack (fallback to row-wise intact)` → `gauge-pack (4³ only)`. Each `kubona-pack/v1` (semver, anchor-required, evidence reqs, Ed25519, allowlist, revocation); classical anchor always; claim-grammar (`12 verbs, ρ tier`) enforced.

See [[evaluator-guided-circuit-codesign]], [[reversible-co-design-patterns]], [[quantum-domain-packs]], [[quantum-advantage-realism]], [[fidelity-accounting]].
