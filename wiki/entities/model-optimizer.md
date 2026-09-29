---
type: entity
name: Model-Optimizer (NVIDIA)
date: 2026-09-27
updated: 2026-09-27
tags: [apache-2.0, recipes, plugin-contract, submodule-candidate]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Model-Optimizer — recipes as the plugin contract

**https://github.com/NVIDIA/Model-Optimizer** · Apache-2.0 · HEAD verified 2026-09-26.
Integration class: **Submodule (candidate)** ([[integration-topology-and-licence-discipline]]).

The most mature "pack" discipline found anywhere in the audit, and it is mostly **data**:

- **Code side**: `modelopt/torch/{quantization,prune,sparsity,distill,nas/autonas,peft,speculative,kernels,export}`
  — verified directory names.
- **Recipe side**: `modelopt_recipes/`, organised as method × model-type
  (`configs/{auto_quantize,numerics,ptq}`, `general/{auto_quantize,distillation,ptq,qad,speculative_decoding}`,
  then `huggingface/`, `model_type/`, `timm/`, `models/`), each level carrying its own README.
- **Each recipe pins its schema in a comment** — `# modelopt-schema: modelopt.recipe.config.ModelOptDFlashRecipe`
  — and maps its sections onto the training script's argument dataclasses; fields are overridden by
  an **OmegaConf dotlist** on the CLI, so a recipe is a default, not a lock.
- **Variants fail loudly.** `general/speculative_decoding/lilicorr_conv.yaml` states which upstream
  variant it requires, warns it "raises at model build with a message naming the two keys to
  remove", imports the shared convolution rather than copying it "so that the two variants cannot
  drift apart arithmetically", and concedes that "**recipes do not compose**".

Kubona copies the discipline, not the code: pack manifests pin a schema, declare composition rules,
and refuse to load with a message that names the offending field
([[quantum-domain-packs]], [[agent-resource-economy]]). The `classical_anchors` and
`evidence_requirements` fields Kubona adds are the research-platform half Model-Optimizer does not
need.

> ⚑ Earlier drafts of this wiki cited `modelopt_recipes/configurators/kits/registry.py` and an
> in-repo `experimental/ControlNetToolkit`; neither exists in this tree. Corrected 2026-09-27
> ([[source-frontier-repo-audit-2026]]).