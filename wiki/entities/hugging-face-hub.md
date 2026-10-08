---
type: entity
tags: [models, hub, gguf, mlx, local-inference, huggingface]
date: 2026-10-07
updated: 2026-10-07
status: adopted
sources:
  - source-architecture-paper
  - source-frontier-repo-audit-2026
---

# Hugging Face Model Hub

The community-standard open-weights repository and API ecosystem (`https://huggingface.co`) hosting machine learning models, datasets, and quantized checkpoints (GGUF, Apple MLX, Safetensors).

## Role in Kubona

Kubona integrates with the Hugging Face Hub through an **in-platform search, inspection, and direct streaming downloader** (`platform/kubona/mesh/hf_hub.py` and `platform/console/src/components/surfaces/HuggingFaceModelHub.tsx`), allowing researchers to discover and acquire local model checkpoints without breaking the sovereign air-gap workflow:

1. **In-Platform Query & Filtering:** Researchers search for open reasoning models (e.g. `Qwen2.5-Coder`, `DeepSeek-R1-Distill`, `Llama-3.3`, `Phi-4`, `Qwen2.5-VL`) directly within the console surface, filtering by library tags (`gguf`, `mlx`, `safetensors`) and pipeline tags (`text-generation`, `image-text-to-text`).
2. **Quantization Inspection:** Fetches repository file trees (`siblings`) to present available quantization levels (`Q4_K_M`, `Q5_K_M`, `Q8_0`, `MLX-4bit`, `FP16`) alongside file sizes and RAM footprint estimates.
3. **Direct Streaming Download with SHA-256 Integrity:** Streams model weights directly from `https://huggingface.co/{repo_id}/resolve/main/{filename}` into the platform's local models directory (`~/.kubona/models/`), calculating SHA-256 integrity hashes on the fly and reporting transfer rates (MB/s).
4. **Local Runtime Registration:** Once downloaded, weights are instantly made available to local execution runtimes (Ollama, LM Studio, oMLX, llama.cpp), fulfilling the sovereign air-gap policy (0 outbound bytes required for inference).

## Cross-References

- [[multimodal-mission-control-and-custom-agents]] — the mission control architecture utilizing downloaded local models.
- [[unsloth]] — local model run and fast adaptation.
- [[secrets-and-workload-isolation]] — local air-gap security and workload sandboxing.
- [[agent-ux-control-plane]] — user interaction and flight control surfaces.
