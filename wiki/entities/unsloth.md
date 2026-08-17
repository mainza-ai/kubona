---
type: entity
name: Unsloth
date: 2026-08-16
tags: [ml, fine-tuning, tool, open-source]
sources: [source-gap-analysis-plan]
status: tracked
---

# Unsloth

Open-source (Apache-2.0, Python) local LLM & diffusion **run + train** platform — desktop
app and studio — known for **~2× faster fine-tuning with ~70% less VRAM** (LoRA, QLoRA,
full fine-tune, RL), GGUF export/deploy, OpenAI-compatible serving, MCP/agent tooling, and
"data recipes" that build datasets from PDFs/CSVs/DOCX. Runs on CPU, NVIDIA, AMD, Intel,
macOS (Apple Silicon) and multi-GPU — relevant because the Milimo/Mainza HAL is Apple
Silicon/CUDA ([[mainza-milimo-foundation]]).

## Why Kubona should care

1. **The GQE flywheel (§13.3).** The meta-loop periodically retrains the GQE proposal
   model from the algorithm library ([[meta-search-and-self-evolution]]). Unsloth is the
   natural training substrate: 2× speedup and low VRAM mean a lab can retrain on
   consumer/Apple-Silicon hardware instead of renting a GPU cluster.
2. **Personas & domain models.** Fine-tune small specialized models (per lab, per workflow
   class) from provenance/library data — privately, on local hardware (pharma/finance
   data never leaves).
3. **RL for agent behavior.** Unsloth supports RL fine-tuning — a fit for
   [[evaluation-and-benchmarks]]' agent-behavior evals (e.g. reward budget-compliance).
4. **Dataset recipes:** turn the [[provenance-and-reproducibility]] ledger + algorithm
   library into training sets.
5. **OpenAI-compatible API + MCP tooling:** served models plug into Kubona's agent layer
   without custom glue; keeps the "open-source components only" posture
   ([[kubona]]).

## Notes

Adopted as an operational integration (Phase 4+ for GQE training, Phase 6 for personas),
not a hard dependency. Verify the current feature state before committing — the product
surface (Desktop/Studio) moves fast.

Related: [[meta-search-and-self-evolution]] · [[karpathy-autoresearch]] ·
[[evaluation-and-benchmarks]] · [[mainza-milimo-foundation]] · [[science-discovery]]
