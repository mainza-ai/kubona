# Generative Quantum Engine (GQE) — LoRA Adapter Distribution

This directory specifies the distribution and runtime integration architecture for the **Generative Quantum Engine (GQE)** low-rank adaptation (LoRA) checkpoint pipeline in Kubona.

## Architecture & Base Model Alignment

The GQE model specializes general-purpose code reasoning LLMs into physics-grounded quantum circuit synthesizers capable of:
1. Generating variational ansatz circuits (HardwareEfficient, Ry-Rz, UCCSD, Clifford-perturbed) that maximize **Meyer-Wallach expressibility** while avoiding **barren plateau** manifolds.
2. Generating valid OpenQASM 2.0 / PennyLane circuits conforming to strict multi-qubit hardware coupling topologies (e.g., Heavy-Hex, Trapped-Ion All-to-All).
3. Mapping molecular electronic Hamiltonians (computed via PySCF / OpenFermion) into qubit operators with minimal Pauli terms.

### Target Base Models
- **Local Workstation / Air-Gapped Mode:** `Qwen/Qwen2.5-Coder-32B-Instruct` served locally via `oMLX` on Apple Silicon with 4-bit / 8-bit MLX quantization.
- **Enterprise / Server Mode:** Hugging Face PEFT / vLLM serving container with multi-GPU tensor parallelism.

## LoRA Hyperparameters

```json
{
  "r": 16,
  "lora_alpha": 32,
  "lora_dropout": 0.05,
  "target_modules": ["q_proj", "k_proj", "v_proj", "o_proj", "gate_proj", "up_proj", "down_proj"]
}
```

## Training Data Provenance & Epistemic Curation

**Status: specification only — no checkpoint is shipped in this tree.**
The counts below are targets for the training run, not measurements:
- **Target champion circuits:** 890 achieving $< 1.6\text{ mHa}$ against PySCF FCI.
- **Target hard negatives:** 530 mutations rejected for vanishing gradients ($\operatorname{Var}[\partial_{\theta} \langle H \rangle] < 10^{-6}$), depth, or noise.

`manifest.json` carries `status: spec-only`, `artifact_present: false`, and null size/hash. Add the artifact and a verified SHA-256 together before claiming a deployable adapter.

## Checkpoint Manifest & Verification

[`manifest.json`](./manifest.json) is a specification, not a distribution. There is no checksum to verify because there is no artifact; do not describe this as a tamper-evident deployment until both exist.
