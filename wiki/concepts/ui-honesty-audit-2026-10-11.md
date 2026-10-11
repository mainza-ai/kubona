---
type: concept
name: UI Honesty Audit 2026-10-11
date: 2026-10-11
updated: 2026-10-11
tags: [audit, console, honesty, theater, verification]
sources: [source-gap-analysis-plan]
status: tracked
---

# UI Honesty Audit (2026-10-11) — what the console fakes

**Method:** every surface + widget read in full; every button traced to backend
or local state; every rendered number classified live-vs-baked. Contract audit
(42/42 endpoint shapes MATCH) done separately — the wire protocol is sound;
the lying is all in presentation. Owner: fix list below, no new features until
violations clear.

## Rule applied

A number rendered without a live source is a violation. A button with an
authoritative label and no backend effect is theater (dead if it implies
safety). Showcases must be labeled illustrative.

## Widgets (all six violate except the mandate card's math)

- **MolecularActiveSpaceWidget** — static 8-orbital energy table + symmetries
  as a computed ladder; `CASSCF(10e,10o)` initial arbitrary; triage strings
  (`Multi-Reference QKSD Target`) as routing decisions. View tabs don't change
  the diagram.
- **QuantumCircuitQIREditorWidget** — `12:288 BB overhead`, `4.2x shots`,
  `Family 279`, cut positions, partition claims all literals; schematic ignores
  `qubits` prop; sample QIR shown as *this* compilation.
- **Lean4GoalTreeInspectorWidget** — entire tactic trace + `ρ=1.0` +
  `0 sorryAx` + axiom whitelist canned; no prover invocation anywhere.
- **ShadowTomographyRadarWidget** — observable estimates, std errors, `14.8x`
  reduction, `99.2% F_shadow` static; **Resample is `setTimeout` theater**;
  slider changes shot count without touching statistics; Converged badge
  unconditional.
- **EpistemicManifoldSurfaceWidget** — anchors with uniform `ρ=1.0`, regime
  label not conditioned on the curvature prop it receives.
- **CryptographicMandateCardWidget** — real Ed25519 math, but `Authorized`
  without verification, ephemeral key as human identity, `tenant-default`
  hardcoded. (Deliberately *not* reused on the canvas for this reason.)

## Surfaces

- **FlightRecorder** — `Advance Discovery Step` fakes an experiment
  (`sleep` + index). Viewer otherwise honest (explicit UNVERIFIED empties).
- **ResultsStudio** — manuscript gating honest; `Prompt Swarm to Re-verify`
  is local-only under a swarm label; 2 seeded annotations
  (`Dr. M. Chileshe`, fixed timestamps) as review history; `Zero sorryAx`
  pill shows without verified data.
- **EpistemicTribunalRoom** — deliberation, personas, quorum % fully
  simulated (`Chamber Live` badge over constants); only Merkle commit is real.
- **FleetRadar** — live (fleet + 1s radar poll); `100% Sovereign Air-Gap`
  toggle is a list filter wearing an enforcement label.
- **CognitiveMesh / HuggingFaceModelHub / LabsView / LLMSettings** — live,
  all buttons reach backend. (LLMSettings rebuilt same day: live providers,
  wired keys, editable roles.)
- **AutonomyCockpit** — highest risk: approve path live (sign + verify), but
  **Emergency Pause does not call `/api/supervisor/abort`** (dead safety
  control), spend slider + air-gap switch local-only, `Simulate Offline`
  banner-only, spend fallbacks (`$1.42`) render as measured.
- **EpistemicManifold** — live with data; empty-state fallback renders
  `Verified True` orb over a hardcoded H₂ claim.
- **SpatialCanvas** — Morse-potential toy curve labeled `VQE Energy` +
  `live PySCF bridge`; Hamiltonian coefficients + fallback QASM/ansatz as
  session data. (Superseded by the arena universe; legacy overlay.)
- **GenerativeUIStudio** — orphaned from the shell; claims agent-collective
  rendering over static mounts.

## Fix list (ordered)

1. **Safety first:** wire Emergency Pause to `/api/supervisor/abort` or
   remove the button. No dead safety controls, ever.
2. **Empty-state discipline:** every fallback renders `UNVERIFIED` /
   `illustrative` / `—` (EpistemicManifold orb, ResultsStudio pills,
   AutonomyCockpit spend, canvas probes already comply).
3. **Theater → real or removed:** FlightRecorder advance (drive a real loop
   step or relabel `Preview next`), Resample (recompute or remove),
   Simulate Offline (dispatch or remove), spend slider (persist or relabel),
   air-gap switch (save or relabel).
4. **Showcase labeling:** all six widgets get `illustrative` badges until fed
   by runs; static tables clearly marked.
5. **Tribunal room:** either drive real `/api/chat/tribunal` turns or relabel
   as rehearsal — never `Chamber Live` over constants.

Related: [[ui-ux-vision]] · [[evaluation-and-benchmarks]] ·
[[local-simulation-test-scenarios]] · [[market-intelligence-capability]]
