---
type: concept
name: Provider Capability Schema
date: 2026-08-16
updated: 2026-10-10
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

## Attestation: probing back (Q-14, closed 2026-10-10)

Yes — where measurement is possible, and honestly unknown where it is not
(`platform/kubona/canary/canary.py`, `schema/provider.py`, plugin
`calibration_snapshot()` contract):

- **Qubit counts**: plugin `health()["measured_qubits"]` (live property or hard-coded
  bound) vs declared `physical_qubits`; mismatch fails attestation.
- **Calibration**: `declared_calibration` (T1/T2, gate/readout errors; unsound
  declarations refuse to load) vs measured snapshot (local-sim zeros by construction;
  IBM live backend properties when credentialed), compared within 25% tolerance.
  Mismatch **fails the canary** (`ok=False`), not just the attestation.
- **Unknown stays unknown**: unmeasurable plugins and undeclared fields yield
  `None` — never a pass, never a fail. Pricing and queue depth remain
  declared-until-metered (only qubit capacity and calibration are attested).