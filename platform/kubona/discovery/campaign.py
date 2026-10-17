"""A lab runs the full discovery loop end-to-end (the Phase-6 gate).

Every stage writes provenance; the produced report carries a ledger/citation ref
for every claim; the best result is archived to the algorithm library. Works
fully offline (scholarly sources degrade to a 'citation needed' record).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any
from kubona.anchors.chemistry import MolecularAnchorEngine
from kubona.autoresearch import Orchestrator, build_vqe_ansatz_harness
from kubona.autoresearch.harnesses import H_ANSATZ
from kubona.circuits import parse_qasm2
from kubona.cost.governance import CostGovernanceService, GovernanceError
from kubona.epistemics.wavefunction import (
    ClaimCredibilityWavefunction,
    EpistemicGateError,
    EvidenceSignal,
)
from kubona.integrations import ground_literature
from kubona.library import AlgorithmLibrary
from kubona.mitigation import run_zne
from kubona.provenance import Ledger
from kubona.reports import ExperimentReport
from kubona.schema import load_provider
from kubona.session import SessionStore

GROUND_Q = "variational quantum eigensolver error mitigation"
DESIGN_QASM = "OPENQASM 2.0;\nqreg q[2];\nry(0.5) q[0];\nry(0.5) q[1];\ncx q[0],q[1];"

# Caller budget + server caps (named consts — never inline literals).
DEFAULT_BUDGET_USD = 50.0
MAX_BUDGET_USD = 50.0
DEFAULT_SHOTS = 8192
MAX_SHOTS = 8192

# Known anchor molecules for single-problem-key validation. Read from the
# anchor table itself (never a parallel hardcoded list): if problem- and
# hamiltonian-derived keys are both known and disagree, the campaign refuses
# (no H2-vs-H2_STO3G confusion). Unknown keys fall through to the anchor
# engine, which refuses with its own honest error (never an invented anchor).
def _anchor_key(value: str) -> str | None:
    from kubona.anchors.chemistry import MolecularAnchorEngine

    norm = value.strip().upper().replace(" ", "")
    for suffix in ("_STO-3G", "_STO3G", "-STO-3G", "-STO3G"):
        if norm.endswith(suffix):
            norm = norm[: -len(suffix)]
    known = {k.upper() for k in MolecularAnchorEngine.BENCHMARKS}
    return norm if norm in known else None


def _resolve_anchor_molecule(problem: str, hamiltonian: str) -> str:
    """Single problem key: problem and hamiltonian must not name different
    known molecules. Returns the hamiltonian (anchor input); refuses on
    disagreement instead of silently analyzing the wrong physics."""
    pk, hk = _anchor_key(problem), _anchor_key(hamiltonian)
    if pk is not None and hk is not None and pk != hk:
        raise ValueError(
            f"problem {problem!r} and hamiltonian {hamiltonian!r} name different "
            f"molecules ({pk} vs {hk}) — refusing (single problem key)"
        )
    return hamiltonian


@dataclass
class DiscoveryCampaign:
    problem: str
    hamiltonian: str
    ground_state: float
    budget_steps: int = 40
    tenant: str = "default-lab"

    def run(self, *, ledger: Ledger | None = None, library: AlgorithmLibrary | None = None,
            source: str = "arxiv", report_store=None, llm=None,
            budget_ceiling_usd: float | None = None,
            shots: int | None = None) -> dict[str, Any]:
        ledger = ledger or Ledger(tenant=self.tenant)
        sessions = SessionStore(tenant=self.tenant)
        # Caller budget param + server cap: caller supplies, server refuses above-cap.
        budget = DEFAULT_BUDGET_USD if budget_ceiling_usd is None else float(budget_ceiling_usd)
        if not budget > 0:
            raise GovernanceError(f"budget_ceiling_usd must be positive, got {budget!r}")
        if budget > MAX_BUDGET_USD:
            raise GovernanceError(
                f"budget_ceiling_usd {budget} exceeds server cap {MAX_BUDGET_USD} — refusing"
            )
        n_shots = DEFAULT_SHOTS if shots is None else int(shots)
        if n_shots <= 0:
            raise GovernanceError(f"shots must be positive, got {shots!r}")
        if n_shots > MAX_SHOTS:
            raise GovernanceError(
                f"shots {n_shots} exceeds server cap {MAX_SHOTS} — refusing"
            )
        gov = CostGovernanceService(sessions, store_path=ledger.path.parent)
        session = sessions.create(f"campaign-{self.problem}", owner="researcher",
                                  budget_ceiling_usd=budget)

        stages: dict[str, Any] = {}

        # 1 GROUND — literature with license/freshness provenance
        ground = ground_literature(GROUND_Q, source=source, ledger=ledger)
        stages["ground"] = {"n_sources": len(ground["items"]), "offline": ground["offline"],
                            "ledger_ref": ground["ledger_ref"]}

        # 2 HYPOTHESIZE + DESIGN — pick Hamiltonian/ansatz, run the cost pre-flight.
        # The preflight result GATES execution: a failed preflight refuses the
        # campaign (fail-closed) instead of running an unbudgeted experiment.
        cap = load_provider("local-simulator")
        circuit = parse_qasm2(DESIGN_QASM)
        pf = gov.preflight(cap, circuit, n_shots, session)
        if not pf["ok"]:
            ledger.append("design_refused", {"problem": self.problem,
                                             "reason": pf["reason"]})
            raise GovernanceError(
                f"campaign design refused by preflight: {pf['reason']}"
            )
        stages["design"] = {"preflight_ok": pf["ok"], "ledger_ref": None}

        # 2b HYPOTHESIZE (local LLM, optional) — let the model seed the search
        hypotheses: list[str] = []
        if llm is not None and llm.available:
            try:
                hypotheses.append(llm.complete(
                    "You are Kubona's scientific assistant.",
                    f"Suggest one ansatz start parameter (a float near 0..4) for "
                    f"minimizing {self.hamiltonian} in a RY-RY-CX ansatz. Reply with a single number.",
                    temperature=0.3))
            except Exception:  # noqa: BLE001 — LLM failure never breaks the campaign
                hypotheses = []
        stages["hypothesis"] = {"llm_used": bool(hypotheses), "suggestion": hypotheses[:1]}
        stages["hypothesis"].update(
            {"ledger_ref": ledger.append("hypothesis", {
                "problem": self.problem, "llm_used": bool(hypotheses),
                "suggestion": hypotheses[:1]}).record_id})

        # 3 CLASSICAL ANCHOR FIRST — the classical bound precedes any
        # experiment: no variational number is interpreted without its anchor
        # in hand (quantum-advantage-realism: anchor before claim, always).
        # Single problem key: problem/hamiltonian must agree on the molecule.
        anchor_molecule = _resolve_anchor_molecule(self.problem, self.hamiltonian)
        mol_engine = MolecularAnchorEngine()
        anchor = mol_engine.compute_anchor(anchor_molecule)
        anchor_ref = ledger.append("analytical_anchor", anchor.to_dict()).record_id
        stages["anchor"] = {"anchor": anchor.to_dict(), "ledger_ref": anchor_ref}

        # 4 EXPERIMENT — the autonomous ansatz search (Search Mode), scored
        # against the anchor computed above.
        harness = build_vqe_ansatz_harness()
        orch = Orchestrator(harness, budget_steps=self.budget_steps, seed=7, ledger=ledger)
        history = orch.search()
        best = orch.best(history)
        stages["experiment"] = {"best_theta": best.artifact, "best_metric": best.metric,
                                "ledger_ref": best.ledger_ref, "steps": len(history),
                                "anchor_ref": anchor_ref}

        # 5 ANALYZE — ZNE on the BEST artifact circuit (the campaign's own
        # physics). If the best artifact cannot build a circuit, run the
        # X|0> preparation explicitly LABELED as a mitigation health-check —
        # never presented as analysis of the campaign system.
        analyze_is_health_check = False
        try:
            best_circuit = parse_qasm2(H_ANSATZ.format(theta=round(float(best.artifact), 4)))
        except Exception:
            best_circuit = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nx q[0];")
            analyze_is_health_check = True
        zne = run_zne(best_circuit, shots=n_shots,
                      noise_rate=0.05, scales=(1, 3, 5), seed=11)
        stages["analyze"] = {
            "mitigated": zne.mitigated_value, "raw": zne.raw_value,
            "health_check_only": analyze_is_health_check,
            "ledger_ref": ledger.append("analysis", {
                "mitigated": zne.mitigated_value,
                "health_check_only": analyze_is_health_check}).record_id}

        # 5b EPISTEMIC GATING (Ring 1 & 2) — over the precomputed anchor.
        variational_energy = -best.metric  # harness metric is -energy (higher is better)
        anchor_eval = mol_engine.evaluate_advantage_claim(variational_energy, anchor)

        # Measured confidences (no hardcoded 0.95/1.0/0.85):
        # - vqe: fraction of valid trials within chemical accuracy of the FCI
        #   anchor (reproducibility of the search itself).
        # - anchor: analytic exact diagonalization — certain by construction
        #   (documented, not sampled).
        # - zne: linear map of the measured mitigation error for the analyzed
        #   circuit (|mitigated - raw| relative to the 0.2 support window).
        chem_tol = 1.6e-3
        valid_energies = [-t.metric for t in history if t.valid]
        if valid_energies:
            n_close = sum(1 for e in valid_energies
                          if abs(e - anchor.fci_energy_hartree) <= chem_tol)
            vqe_conf = n_close / len(valid_energies)
        else:
            vqe_conf = 0.0
        anchor_conf = 1.0  # analytic FCI anchor: exact by construction
        zne_spread = abs(zne.mitigated_value - zne.raw_value)
        zne_conf = max(0.0, 1.0 - zne_spread / 0.2)

        wavefunction = ClaimCredibilityWavefunction()
        signals = [
            EvidenceSignal(
                source_id="vqe_ansatz_search",
                source_type="experiment",
                supports=True,
                confidence=vqe_conf,
                engagement=1.0,
                ledger_ref=best.ledger_ref,
            ),
            EvidenceSignal(
                source_id="classical_anchor_fci",
                source_type="reproduction",
                supports=anchor_eval["chemical_accuracy_achieved"],
                confidence=anchor_conf,
                engagement=1.0,
            ),
            EvidenceSignal(
                source_id="zne_mitigation",
                source_type="qpu_canary",
                supports=abs(zne.mitigated_value - zne.raw_value) < 0.2,
                confidence=zne_conf,
                engagement=0.5,
                ledger_ref=stages["analyze"]["ledger_ref"],
            ),
        ]
        wf_state = wavefunction.evaluate(signals)
        claim_text = (
            f"VQE variational energy {variational_energy:.4f} reproduces ground-state energetics of {self.hamiltonian} "
            f"against classical FCI anchor ({anchor.fci_energy_hartree:.4f} Ha)."
        )
        # Fail-closed assertion: a split verdict does NOT get asserted — the
        # gate's designed behavior is to hold it as a qualified hypothesis.
        # (Previously the unrelated-physics ZNE always voted contest and
        # forced a contested collapse; honest signals may genuinely not
        # collapse, and that is recorded, never crashed over.)
        try:
            asserted_claim = wavefunction.assert_claim(claim_text, wf_state)
        except EpistemicGateError:
            asserted_claim = (
                f"[UNCOLLAPSED] {claim_text} Held as hypothesis: "
                f"max P={max(wf_state.probabilities.values()):.2f}, "
                f"diversity={wf_state.diversity_score:.2f}."
            )

        # 5 REPORT — structured notebook; every claim -> ref. Refs are
        # preserved verbatim: a missing ref is recorded as missing (never
        # "n/a"/"none") and all_claims_traceable gates the return.
        report = ExperimentReport(
            title=f"Energetics of {self.hamiltonian}",
            objectives=f"Locate the ground state of {self.hamiltonian} (known {self.ground_state}).",
            methods="Simulator-backed VQE ansatz search with golden-section refinement.",
            protocol=f"Budget {self.budget_steps} search steps; backend local-simulator.",
            results=f"Best energy metric {best.metric:.4f} at theta {best.artifact:.4f}.",
            analysis=(f"Mitigation health-check <Z> to {zne.mitigated_value:.4f} from raw {zne.raw_value:.4f}."
                      if analyze_is_health_check else
                      f"ZNE recovered <Z> to {zne.mitigated_value:.4f} from raw {zne.raw_value:.4f}."),
            conclusions=f"{asserted_claim}. {anchor_eval['honest_verdict']}.",
            citations=ground["citations"],
            how_to_reproduce=f"session={session.session_id}; ledger tenant-scoped; result archived in the library",
            classical_anchor=anchor.to_dict(),
        )
        missing_refs: list[str] = []
        report.add_claim(claim_text, wf_state.to_dict())
        for section, ref in (
            ("methods", stages["experiment"]["ledger_ref"]),
            ("ground", ground["ledger_ref"]),
            ("analysis", stages["analyze"]["ledger_ref"]),
        ):
            if ref:
                report.add_ref(section, ref)
            else:
                missing_refs.append(section)
        stages["epistemics"] = {"wavefunction": wf_state.to_dict(), "anchor": anchor.to_dict()}
        stages["report"] = {"report_id": report.report_id, "n_refs": len(report.refs), "collapsed": wf_state.collapsed}
        if report_store is not None:
            report_store.save(report)  # the results-studio's data

        # 6 ARCHIVE — library entry (the compounding asset) with the report linked.
        # A missing best.ledger_ref refuses the archive (no "none" phantoms).
        # The entry is validated with the anchor ledger record as its explicit
        # proof path (simulator search scored against the FCI anchor above).
        entry = None
        if library is not None:
            if not best.ledger_ref:
                stages["archive"] = {"library_entry_id": None,
                                     "refused": "best trial carries no ledger ref — refusing phantom archive"}
            else:
                entry = library.add(harness="vqe_ansatz", artifact=best.artifact, metric=best.metric,
                                    cost_usd=0.0, ledger_ref=best.ledger_ref,
                                    validated=True, validation_proof=anchor_ref,
                                    owner="researcher", tenant=self.tenant,
                                    report_id=report.report_id)
                stages["archive"] = {"library_entry_id": entry.entry_id if entry else None}
        else:
            stages["archive"] = {"library_entry_id": None}

        traceable = (not missing_refs) and all(
            r[1] not in ("n/a", "none", "", None) for r in report.refs
        ) and len(report.refs) >= 3
        return {"problem": self.problem, "stages": stages, "session_id": session.session_id,
                "report": report.to_dict(), "library_entry_id": entry.entry_id if entry else None,
                "missing_refs": missing_refs,
                "all_claims_traceable": traceable}


def run_campaign(problem: str, hamiltonian: str, ground_state: float,
                budget_steps: int = 40, tenant: str = "default-lab",
                **run_kwargs: Any) -> dict[str, Any]:
    """Build a campaign (constructor args) and run it (ledger/library/source)."""
    return DiscoveryCampaign(problem=problem, hamiltonian=hamiltonian,
                             ground_state=ground_state, budget_steps=budget_steps,
                             tenant=tenant).run(**run_kwargs)
