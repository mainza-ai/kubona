---
name: Fidelity Accounting
type: concept
tags: [frontier, noise, mitigation, governance, hardware]
date: 2026-09-27
updated: 2026-10-10
status: adopted
sources: [source-frontier-repo-audit-2026, source-architecture-paper]
---

# Fidelity Accounting

**Definition.** A second budget rail next to the money rail. Before a job may spend QPU time,
the platform must **predict** the fidelity of the result it intends to buy, and **verify**
that prediction with a cheap probe. Jobs are then admitted, downgraded, or refused on
*physical expected value*, not on price. Nobody asks for a quantum-advantage claim; the
question is whether a given run can produce a publishable number at all, and today Kubona
cannot answer that before spending.

> Decided (Q-07, closed 2026-10-10): **kept distinct, not merged.** One line:
> cost-governance gates *money*, error-mitigation *repairs results*, fidelity-accounting
> *predicts whether a run can produce a publishable number and refuses it upfront*.
> Enforced in both admission paths (`FidelityGate.EvaluateAdmissionFloorOnly` in Go
> dispatch; `evaluate_admission` in the Python execution server): unknown devices refuse
> as unpredictable, below-floor refuses with the recommended action stated, remediation
> is never applied silently (it changes what was approved).

## Why it is a gate, not a report

[[cost-governance]] already refuses jobs that would overspend. But a job can be perfectly
affordable and physically pointless — a 400-layer ansatz on a device whose coherence supports
40. Money gates protect the wallet; only a fidelity gate protects the *literature* from a
torrent of uninterpretable results. Ring 3 of [[five-ring-research-loop]].

## Verified primitives

| Primitive | Where | Role |
|---|---|---|
| Named device profiles with an enforced physical constraint `t2 <= 2*t1` | `milimoquantum/backend/app/quantum/noise_profiles.py:10,17,24` | the prior; validation is code, not documentation |
| `build_noise_model` → real `qiskit_aer.NoiseModel` | `noise_profiles.py:80` | predicts expectation values under noise (`execute_with_noise`, `:116`) |
| Capability/availability probe | `milimoquantum/backend/app/quantum/hal.py:37` `check_availability`, `:57` `get_method_status` | device/backend truth before submit |
| Clifford fast path (`stim` when `is_clifford`) | `hal.py:8` | makes the probe cheap enough to be mandatory |
| Existing estimate→pre-flight→settlement gate | Kubona `kubona/mcp/servers/cost.py`, [[cost-governance]] | the shape the fidelity gate copies |
| Mitigation menu | [[error-mitigation]], [[mitiq]], ZNE in `kubona/` Phase 1 | the lever used when the floor is unmet |

## Mechanism

```
predict(job)      → F_pred from device noise profile + circuit structure
probe(job)        → ≤ 64-shot Clifford / stabilizer variant of the same circuit topology
                      ⇒ F_meas, and calibration age (freshness gate)
budget(F_pred)    → shots needed for target σ; shots × price ⇒ *physics cost*
admit             → F_pred ≥ floor AND |F_pred − F_meas| ≤ tol  →  run
downgrade         → else search mitigations: ZNE folding, [[circuit-cutting]] via
                    PennyLane qcut (bi-sever + QPD sampling), depth reduction,
                    re-compilation via [[agentic-qir-compilation]]
refuse            → floor unreachable at any affordable setting →
                    insufficient_evidence, with the reason recorded
```

Two properties matter. **Bounded error propagation:** each gate's error is accounted against
the declared target (see [[nisq-and-hybrid-orchestration]]), so `F_pred` is a sum of auditable
terms rather than a guess. **Calibration freshness:** `F_pred` is void when calibration data is
older than `device_refresh_interval`, mirroring `cloud_provider_api_configs` in
[[qflow-studio]] and the staleness windows in `gods-eye-view`'s `local-receivers`.

## What is genuinely new

1. **A fidelity ledger.** Every run stores `(F_pred, F_meas, tol, mitigation_applied,
   calibration_age)`. Over a quarter this becomes a **calibration curve for the platform's own
   physics predictions** — and when the curve drifts, we know the noise model is wrong before
   a physicist notices the results are.
2. **Refusal with receipts.** "Refused, floor unreachable at price P, with mitigation set M"
   is a *reusable negative result* feeding [[science-discovery]], not a rejected request.
3. **Mitigation selection becomes a searchable harness.** Which of {ZNE, PEC, cutting,
   symmetry verification, shallower ansatz} maximises `F` per credit on *this* device — that
   is an ordinary `Harness` over `Orchestrator` (`autoresearch/base.py:72`), scored on real
   logs. Nobody tunes this by hand per device today.
4. **Device transfer predictions.** `F_pred` per ansatz genome × device populates the transfer
   prior in [[autonomous-ansatz-optimization]]: cheap, falsifiable, and the actual open problem
   in NISQ practice.

## Honesty constraints

Predicted fidelity is a model output, never a correctness guarantee; it is reported beside
measured, always as an interval with the calibration timestamp. Where noise is not
process-Markovian or leakage-dominated, profile-based prediction under-reports error — the
gate must therefore be able to say *"unpredictable on this device"* and treat that as a
refusal reason. No quantum-advantage claims (`wiki/concepts/risks.md`), and no
fidelity number may appear in an artefact without its `quantum_provenance` block
([[claim-credibility-wavefunctions]] § provenance rule).

## Per-mechanism budgets (built 2026-10-10)

`FidelityAccountingEngine.fidelity_budget` attributes predicted infidelity by
leave-one-out ablation (decoherence / 1Q / 2Q / readout toggled ideal in
turn): absolute gains are the honest numbers, normalized shares only rank.
Documented non-additivity — the model is multiplicative, so shares need not
sum to total infidelity and must never be presented as a ledger. Ideal
devices report no budget and no dominant mechanism. Bell-on-Heron example:
readout dominates, then decoherence — the selector implications (fix
readout first) fall out of the numbers, not from tuning a headline.

## Gate test

Synthetic device with a known profile → predicted-vs-measured fidelity within tolerance on a
Clifford probe; a deliberately over-deep circuit is refused at the configured floor; a stale
calibration voids admission. All three as pytest, simulator-only.

See also, as consumers of this gate: [[autonomous-ansatz-optimization]],
[[agentic-qir-compilation]], [[quantum-cloud-pricing]], [[provider-capability-schema]],
[[error-mitigation]].
