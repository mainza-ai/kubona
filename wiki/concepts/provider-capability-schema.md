---
type: concept
name: Provider Capability Schema
date: 2026-08-16
updated: 2026-09-27
tags: [providers, schema, routing]
sources: [source-gap-analysis-plan]
status: adopted
---

# Provider Capability Schema

Replaces the fixed four-row pricing table in the architecture paper with a **data-driven
contract** describing each backend, so onboarding a new provider (IQM, a future entrant) is
a data change, not a code change (source-gap-analysis-plan).

## Shape (example from the source)

```json
{
  "provider": "quantinuum",
  "modality": "trapped_ion",
  "physical_qubits": 98,
  "logical_qubits_available": 48,
  "native_gate_set": ["rzz", "rz", "rx"],
  "queue_depth_estimate_s": 120,
  "pricing_model": "per_shot_plus_task",
  "jurisdiction": "US/UK",
  "export_control_flag": "review_required_non_allied",
  "sdk_bindings": ["pytket", "qiskit_via_qir"],
  "status": "GA"
}
```

## What it enables

- Agent routing decisions on real signals: qubit count, modality, gate set, logical-qubit
  availability, queue depth, pricing model.
- **Export-control / jurisdiction enforcement** at the routing layer (entity-level
  restrictions, not just tool-level permissions) — the answer to the compliance gap raised
  in the source plan (§2.9).
- Service that generalizes "agent readiness" audits the platform could sell to providers.