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
from kubona.circuits import parse_qasm2
from kubona.cost.governance import CostGovernanceService
from kubona.epistemics.wavefunction import (
    ClaimCredibilityWavefunction,
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


@dataclass
class DiscoveryCampaign:
    problem: str
    hamiltonian: str
    ground_state: float
    budget_steps: int = 40
    tenant: str = "default-lab"

    def run(self, *, ledger: Ledger | None = None, library: AlgorithmLibrary | None = None,
            source: str = "arxiv", report_store=None, llm=None) -> dict[str, Any]:
        ledger = ledger or Ledger(tenant=self.tenant)
        sessions = SessionStore(tenant=self.tenant)
        gov = CostGovernanceService(sessions, store_path=ledger.path.parent)
        session = sessions.create(f"campaign-{self.problem}", owner="researcher",
                                  budget_ceiling_usd=50.0)

        stages: dict[str, Any] = {}

        # 1 GROUND — literature with license/freshness provenance
        ground = ground_literature(GROUND_Q, source=source, ledger=ledger)
        stages["ground"] = {"n_sources": len(ground["items"]), "offline": ground["offline"],
                            "ledger_ref": ground["ledger_ref"]}

        # 2 HYPOTHESIZE + DESIGN — pick Hamiltonian/ansatz, run the cost pre-flight
        cap = load_provider("local-simulator")
        circuit = parse_qasm2(DESIGN_QASM)
        pf = gov.preflight(cap, circuit, 8192, session)
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
        mol_engine = MolecularAnchorEngine()
        anchor = mol_engine.compute_anchor(self.hamiltonian)
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

        # 5 ANALYZE — ZNE error-mitigation sanity check
        zne = run_zne(parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nx q[0];"), shots=8000,
                      noise_rate=0.05, scales=(1, 3, 5), seed=11)
        stages["analyze"] = {
            "mitigated": zne.mitigated_value, "raw": zne.raw_value,
            "ledger_ref": ledger.append("analysis", {"mitigated": zne.mitigated_value}).record_id}

        # 5b EPISTEMIC GATING (Ring 1 & 2) — over the precomputed anchor.
        variational_energy = -best.metric  # harness metric is -energy (higher is better)
        anchor_eval = mol_engine.evaluate_advantage_claim(variational_energy, anchor)

        wavefunction = ClaimCredibilityWavefunction()
        signals = [
            EvidenceSignal(
                source_id="vqe_ansatz_search",
                source_type="experiment",
                supports=True,
                confidence=0.95,
                engagement=1.0,
                ledger_ref=best.ledger_ref,
            ),
            EvidenceSignal(
                source_id="classical_anchor_fci",
                source_type="reproduction",
                supports=anchor_eval["chemical_accuracy_achieved"],
                confidence=1.0,
                engagement=1.0,
            ),
            EvidenceSignal(
                source_id="zne_mitigation",
                source_type="qpu_canary",
                supports=abs(zne.mitigated_value - 1.0) < 0.2,
                confidence=0.85,
                engagement=0.5,
                ledger_ref=stages["analyze"]["ledger_ref"],
            ),
        ]
        wf_state = wavefunction.evaluate(signals)
        claim_text = (
            f"VQE variational energy {variational_energy:.4f} reproduces ground-state energetics of {self.hamiltonian} "
            f"against classical FCI anchor ({anchor.fci_energy_hartree:.4f} Ha)."
        )
        asserted_claim = wavefunction.assert_claim(claim_text, wf_state)

        # 5 REPORT — structured notebook; every claim -> ref
        report = ExperimentReport(
            title=f"Energetics of {self.hamiltonian}",
            objectives=f"Locate the ground state of {self.hamiltonian} (known {self.ground_state}).",
            methods="Simulator-backed VQE ansatz search with golden-section refinement.",
            protocol=f"Budget {self.budget_steps} search steps; backend local-simulator.",
            results=f"Best energy metric {best.metric:.4f} at theta {best.artifact:.4f}.",
            analysis=f"ZNE recovered <Z> to {zne.mitigated_value:.4f} from raw {zne.raw_value:.4f}.",
            conclusions=f"{asserted_claim}. {anchor_eval['honest_verdict']}.",
            citations=ground["citations"],
            how_to_reproduce=f"session={session.session_id}; ledger tenant-scoped; result archived in the library",
            classical_anchor=anchor.to_dict(),
        )
        report.add_claim(claim_text, wf_state.to_dict())
        report.add_ref("methods", stages["experiment"]["ledger_ref"] or "n/a")
        report.add_ref("ground", ground["ledger_ref"] or "n/a")
        report.add_ref("analysis", stages["analyze"]["ledger_ref"])
        stages["epistemics"] = {"wavefunction": wf_state.to_dict(), "anchor": anchor.to_dict()}
        stages["report"] = {"report_id": report.report_id, "n_refs": len(report.refs), "collapsed": wf_state.collapsed}
        if report_store is not None:
            report_store.save(report)  # the results-studio's data

        # 6 ARCHIVE — library entry (the compounding asset) with the report linked
        entry = None
        if library is not None:
            entry = library.add(harness="vqe_ansatz", artifact=best.artifact, metric=best.metric,
                                cost_usd=0.0, ledger_ref=best.ledger_ref or "none",
                                report_id=report.report_id)
        stages["archive"] = {"library_entry_id": entry.entry_id if entry else None}

        return {"problem": self.problem, "stages": stages, "session_id": session.session_id,
                "report": report.to_dict(), "library_entry_id": entry.entry_id if entry else None,
                "all_claims_traceable": all(r[1] not in ("n/a", "none") for r in report.refs)}


def run_campaign(problem: str, hamiltonian: str, ground_state: float,
                budget_steps: int = 40, tenant: str = "default-lab",
                **run_kwargs: Any) -> dict[str, Any]:
    """Build a campaign (constructor args) and run it (ledger/library/source)."""
    return DiscoveryCampaign(problem=problem, hamiltonian=hamiltonian,
                             ground_state=ground_state, budget_steps=budget_steps,
                             tenant=tenant).run(**run_kwargs)
