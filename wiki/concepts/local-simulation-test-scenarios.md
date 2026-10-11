---
type: concept
name: Local-Simulation Copilot Test Scenarios
date: 2026-10-11
updated: 2026-10-11
tags: [testing, copilot, simulation, acceptance, console]
sources: [source-gap-analysis-plan]
status: adopted
---

# Local-Simulation Copilot Test Scenarios

**Adopted operator procedure, 2026-10-11.** Ten copy-paste scenarios that
exercise the co-pilot and console end-to-end on **local classical simulation
only** ($0.00, no QPU). Validated live against Go `kubonad` + oMLX Qwen
(28 s chat round-trip with proposal JSON emitted, full H₂ campaign synthesis,
session lifecycle create/delete verified) before filing.

## Standing rules

- **Local simulators only** (`qiskit-aer-simulator`, statevector, NumPy,
  PySCF anchors, Clifford canary on sims). If the Action Gate appears,
  **deny it or leave it pending** — physical pulses are out of scope.
- Stack: `kubonad -port 8080` (Go core — the API the console was built
  against) + `pnpm dev --port 3000` with `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN`
  set. The Python bridge is the *wrong* backend for UI testing (SSE events,
  `session_id` shape) — see [[ui-ux-vision]] §13 log trail.
- Setup per run: avatar menu → New → `Local Sim Campaign`, target `H2`, cap `$150`.
- Report per scenario: **Pass** (observed) / **Fail** (console errors) /
  **Confusing** (felt wrong), plus latency (chat ~30 s/turn local; >3 min is a bug).

## Scenarios

1. **Baseline chat + proposal.** Paste: *Map the H2 ground state potential
   energy surface across bond distances from 0.5 to 2.0 Angstrom.* Verify:
   reasoning + launchable proposal (`Local Simulator` tier, PySCF STO-3G
   anchor `-1.13727 Ha`); arena retargets, probes refresh.
2. **Classical anchor first.** *Verify the PySCF STO-3G FCI ground truth for
   H2 at 0.7414 Angstrom before we run anything variational.* Verify: anchor
   cited; HUD Δ ribbon updates; no variational claim without the number
   ([[quantum-advantage-realism]]).
3. **Clifford canary on local sim.** *Simulate a Clifford RB canary on the
   local statevector simulator and report the fidelity.* Verify: fidelity on
   the Critic probe; proceed only if ≥ ~0.99.
4. **Barren plateau check.** *Assess barren plateau risk for a UCCSD ansatz on
   H2 with 2 qubits: report gradient variance and verdict.* Verify: Theorist
   hover ∇ variance; Critic CLEARED/WATCHING consistent with it.
5. **Full tribunal.** *Convene a tribunal on the LiH ground state electronic
   structure* (or the tribunal button). Verify: 4 role cards, consensus badge,
   synthesis (~4 local-LLM turns, minutes).
6. **Launch on local sim.** Launch any proposal answer. Verify: `qiskit-aer`
   tier, `$0.00`, loop in the Flight Recorder (`⌘2`).
7. **On-demand widgets.** Under a synthesis, hit ✦ **Inspect circuit** /
   **Shadow estimates** / **Active space**. Verify: single widget, live numbers,
   × returns silence. **Spend card** → read-only + *Review at Action Gate*
   (do not authorize).
8. **Flight recorder scrub.** *Show me the trajectory of this campaign: what
   did we try, what failed, what forked?* Verify: steps, branches, forks.
9. **Literature grounding.** *Find prior art on variational H2 ground state
   methods and cite it.* Verify: records with sources/years, live-vs-cached
   flagged, ledger-backed ([[science-data-integrations]]).
10. **Manuscript + closeout.** *Draft the manuscript for this campaign with
    methods, anchor, and uncertainty — then report total spend vs cap.*
    Verify: proofs → Manuscripts draft with anchor + Δ; spend ≈ `$0.00`.

Related: [[evaluation-and-benchmarks]] · [[ui-ux-vision]] ·
[[agent-ux-control-plane]] · [[quantum-advantage-realism]] ·
[[science-data-integrations]] · [[market-intelligence-capability]]
