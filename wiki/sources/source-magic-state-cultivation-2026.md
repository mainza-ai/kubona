---
type: source
tags: [source, fault-tolerance, magic-states, willow, metrology]
date: 2026-10-18
updated: 2026-10-18
status: source
origin: "Science (Oct 2026) via Google Quantum AI briefing: Rosenfeld/Gidney magic-state cultivation on Willow (transcript lines 295-360); unreproduced hardware claim — tracked vendor result, not adopted mechanism"
---

# Source: Magic-State Cultivation on Willow (Google 2026)

**Live record:** Science publication week of 15 Oct 2026; Rosenfeld (research scientist) + Gidney (software engineer), Google Quantum AI, Willow processor.

## What was demonstrated

- Cultivation = noisy-state inject → coordinated-measure → discard-flawed → grow-pure + adapt to larger code. Origin 2004 idea → 2024 Gidney/Shutty/Jones refinement.
- `~40×` error cut; record logical T-fidelity `99.99%` (state-of-art metrology); self-measuring benchmark after fixing drift/temperature benchmark bug; `d3-color → d5-surface` graft survives transfer.
- Still sub-scale; useful error-corrected machine remains outstanding.

## Why it matters to Kubona

Up to `~95%` of a machine can go to magic-state production; this attacks the dominant cost the `T`-proxy in [[evaluator-guided-circuit-codesign]] assumes. Capability schema must carry `factory-throughput, cultivation-fidelity, code-distances`; costing switches `T → depth/volume` when cultivation data present.

## Status

`tracked` vendor result. No advantage claim; record backend + anchor + receipts per [[quantum-advantage-realism]].
