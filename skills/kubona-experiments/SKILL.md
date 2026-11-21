---
name: kubona-experiments
description: Run quantum circuits, PySCF classical anchors, VQE/PES/QAOA/spin-chain experiments, ZNE mitigation, QUBO annealing, and discovery pipelines on Kubona with scientific rigor.
---

# kubona-experiments: Anchors, Circuits & Discovery

Version 2026-11-20. Use this skill whenever computing energies, compiling
circuits, dispatching simulator or QPU jobs, or running structured experiments.

## Mandatory Scientific Protocol
1. **Compute the classical anchor first:** Call `get_classical_anchor` (or
   `POST /api/chemistry/anchor`) with the exact molecule, bond distance (Å),
   and basis set (`sto-3g`, etc.). Record both **RHF** and **FCI** energies in
   Hartree (Ha). If the bridge returns `bridge_unavailable`, stop and state
   that the claim is unverified — never fabricate energies.
2. **Default to free local simulators:** Use `qiskit-aer-simulator` or
   `local-simulator` first (`$0.00`). Call `list_provider_registry` to confirm
   which providers are live vs fail-closed capability stubs.
3. **Compare in mHa under Rayleigh-Ritz:** Report `E_quantum - E_FCI` in mHa.
   An energy slightly below FCI from finite-shot variance is shot noise, never
   quantum advantage.

## Path A: Structured Experiments & Discovery (Preferred)
Use `science` MCP tools (or Go HTTP API):
- **Single structured experiment:** Call `run_experiment` (`POST /api/experiments`)
  with `session_id` and `kind` in:
  `vqe`, `pes`, `qaoa`, `spin_chain`, `geometry_optimization`, `vqe_repeats`,
  `excited_state`, `reaction`, `thermodynamic_limit`, `conjecture`,
  `lattice_dynamics`.
- **Multi-step discovery:**
  1. Call `plan_discovery` (`POST /api/discovery/plan`) — spend-free.
  2. Review the returned plan and check `get_session_autonomy`.
  3. Call `run_discovery` (`POST /api/discovery/run`). If `prior_negative_exists`
     or `risk_approval_required` (`409`), escalate to the operator.

## Path B: Raw OpenQASM 2.0 Circuit Pipeline
1. **Validate & canonicalize:** Call `transpile_circuit` (or `compile_circuit`
   for pass portfolio + equivalence evidence, or `lower_circuit` for QIR IR).
   Malformed QASM refuses — never guessed.
2. **Pre-flight cost estimate:** Call `estimate_cost` with the canonical QASM,
   provider, and shot count. Always note `heuristic: true` (estimates inform,
   session ceilings bind).
3. **Submit:** Call `submit_job` (or `run_zne_job` for unitary-folding +
   Richardson ZNE mitigation, or `submit_qubo_job` for D-Wave/local QUBO).
4. **Poll & settle:** Poll `get_job_status` until terminal, then call
   `get_result`. Use `cancel_job` to abort a queued/running job.

## Paid Hardware Dispatch (IBM Quantum / Quantinuum)
- Paid backends trigger the **MRTR approval gate** in `KubonaGateway`
  (`platform/kubona/mcp/gateway.py:185`): `submit_job` returns an approval
  packet (`request_state`) without spending. Present it to the operator and
  re-submit only with their `approval_response` / signed Ed25519 mandate.
- For structured hardware runs over HTTP, use the two-step prepare → dispatch
  endpoints: `/api/experiments/vqe/hardware/prepare` → `/api/experiments/vqe/hardware`
  (and matching `pes`, `qaoa`, `spin-chain` pairs).

## Traps
- Do not claim chemical accuracy (`1.6 mHa`) without citing the exact FCI
  anchor, shot count, noise model, and backend name.
- `anchor_missing` (`400`) refuses verified claims that lack measured evidence.
- `agentic_disabled` (`403`) or `agentic_locked` (`409`) blocks autonomous
  execution; direct the operator to enable Auto Agent or run manually in the
  Experiment & Discovery Lab (`Cmd+E`).
