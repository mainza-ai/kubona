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

Training data is systematically generated through the autonomous Karpathy loop engine and validated against strict classical physics anchors:
- **Champion Circuits:** 890 verified circuits achieving $< 1.6\text{ mHa}$ chemical accuracy against PySCF Full Configuration Interaction (FCI) calculations.
- **Hard Negatives:** 530 circuit mutations flagged and rejected by the Epistemic Tribunal due to vanishing cost gradients ($\operatorname{Var}[\partial_{\theta} \langle H \rangle] < 10^{-6}$), excessive circuit depth, or noise susceptibility.

## Checkpoint Manifest & Verification

The adapter artifacts are tracked in [`manifest.json`](./manifest.json) with SHA-256 integrity checksums to ensure reproducible and tamper-evident deployments across cluster nodes.
