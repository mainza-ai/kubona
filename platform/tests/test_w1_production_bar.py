"""W1.18–W1.20 production-bar regression cover.

Every fix in the slice gets a refusal-path or labeling test here (bridge
modules load via the file pattern used by test_bridge_chemistry.py; the
pinned `pytest platform/tests/` command never touches platform/bridge/).
"""

import importlib.util
import json
import math
import sys
from pathlib import Path

import pytest
from pydantic import ValidationError

BRIDGE = Path(__file__).resolve().parent.parent / "bridge"


def _load(name, filename):
    if str(BRIDGE) not in sys.path:
        sys.path.insert(0, str(BRIDGE))
    spec = importlib.util.spec_from_file_location(name, BRIDGE / filename)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


@pytest.fixture()
def b_epistemics():
    return _load("w1_bridge_epistemics", "epistemics.py")


@pytest.fixture()
def b_ansatz():
    return _load("w1_bridge_ansatz", "ansatz_search.py")


# ------------------------------------------------- W1.18 bridge epistemics ---

def test_epistemics_consts_and_heuristic_label(b_epistemics):
    m = b_epistemics
    assert m.CHEMICAL_ACCURACY_HA == pytest.approx(1.6e-3)
    assert m.CREDIBILITY_METHOD == "credibility_heuristic_v1"
    for const in ("DIVERSITY_CAP_SOURCES", "AMPLITUDE_FLOOR",
                  "W_CORR_DIVERSITY", "W_CONT_CONTRADICTION"):
        assert isinstance(getattr(m, const), float)
    out = m.evaluate_claim_credibility(
        "c", "measured x", -1.136, -1.137, ["a"], 10, 0.0)
    assert out["credibility_method"] == "credibility_heuristic_v1"


def test_epistemics_advantage_requires_fci_exact(b_epistemics):
    m = b_epistemics
    kw = dict(claim_id="c", statement="quantum advantage", quantum_value=-1.138,
              classical_anchor_value=-1.137, source_platforms=["a", "b", "c", "d"],
              total_engagement=5000, contradiction_signal=0.0, claim_type="advantage")
    ok = m.evaluate_claim_credibility(**kw, anchor_method="FCI-exact")
    assert ok["quantum_advantage_verified"] is True
    for bad in ("CASCI(6,6)", "RHF", "unknown", ""):
        refused = m.evaluate_claim_credibility(**kw, anchor_method=bad)
        assert refused["quantum_advantage_verified"] is False
        assert "FCI-exact" in refused["verdict_reason"]


def test_epistemics_amplitudes_fused_not_decorative(b_epistemics):
    m = b_epistemics
    # Passing delta but a contested prior must NOT validate cleanly.
    out = m.evaluate_claim_credibility(
        "c", "measured x", -1.136, -1.137, ["only-one"], 5, 0.95)
    assert out["passes_chemical_accuracy"] is True
    assert "Validated" not in out["verdict_reason"]
    assert out["contested_amplitude"] > out["corroborated_amplitude"]


def test_epistemics_claim_type_allowlist(b_epistemics):
    m = b_epistemics
    with pytest.raises(ValueError, match="claim_type"):
        m.evaluate_claim_credibility("c", "measured x", 0.0, 0.0, [], 0, 0.0,
                                     claim_type="PROVES")


# ------------------------------------------------- W1.18 claim grammar ---

def _claim_kw(**over):
    from kubona.epistemics.claim_grammar import (
        ClaimCategory, EvidenceTier, HardwareProvenance, BackendTier)
    kw = dict(
        category=ClaimCategory.EMPIRICAL_SIMULATION,
        predicate="Measured H2 ground energy converged within chemical accuracy",
        classical_anchor_name="PySCF STO-3G FCI",
        classical_anchor_value=-1.137,
        quantum_measured_value=-1.136,
        delta_hartree=0.001,
        chemical_accuracy_achieved=True,
        uncertainty_sigma=0.0005,
        confidence_interval_95=(-1.137, -1.135),
        falsification_criteria=["independent FCI recomputation differs by >1.6 mHa"],
        hardware_provenance=HardwareProvenance(
            backend_name="qiskit_aer.AerSimulator",
            backend_tier=BackendTier.T1_LOCAL_EMU, shots=4000, seed=7),
        evidence_tier=EvidenceTier.T3_SAMPLED,
    )
    kw.update(over)
    return kw


def test_grammar_word_boundary_verbs():
    from kubona.epistemics.claim_grammar import ScientificClaim
    # "recomputed" must NOT pass via substring "computed".
    with pytest.raises(ValidationError, match="permitted reporting verb"):
        ScientificClaim(**_claim_kw(
            predicate="We recomputed the energy value today ok"))
    # Standalone verb passes.
    ScientificClaim(**_claim_kw(predicate="Measured the energy value today ok"))


def test_grammar_hype_blocklist_expanded():
    from kubona.epistemics.claim_grammar import ScientificClaim
    for hype in ("Measured and proves the energy value today",
                 "Measured breakthrough energy value today ok",
                 "Measured revolutionary energy value today ok"):
        with pytest.raises(ValidationError, match="Forbidden ungrounded term"):
            ScientificClaim(**_claim_kw(predicate=hype))


def test_grammar_delta_derived_not_supplied():
    from kubona.epistemics.claim_grammar import ScientificClaim
    with pytest.raises(ValidationError, match="derived"):
        ScientificClaim(**_claim_kw(delta_hartree=0.5))
    with pytest.raises(ValidationError, match="contradicts"):
        ScientificClaim(**_claim_kw(chemical_accuracy_achieved=False))


def test_grammar_shots_required_for_measured():
    from kubona.epistemics.claim_grammar import (
        ClaimCategory, ScientificClaim, HardwareProvenance, BackendTier)
    hw0 = HardwareProvenance(backend_name="b", backend_tier=BackendTier.T1_LOCAL_EMU,
                             shots=0)
    with pytest.raises(ValidationError, match="shots > 0"):
        ScientificClaim(**_claim_kw(category=ClaimCategory.QPU_MEASURED,
                                    hardware_provenance=hw0))
    # Analytic proofs need no shots.
    ScientificClaim(**_claim_kw(category=ClaimCategory.ANALYTIC_PROOF,
                                hardware_provenance=hw0))


def test_grammar_structured_justification():
    from kubona.epistemics.claim_grammar import (
        AdvantageJustification, CredibilityField, ScientificClaim)
    # A bare string is not a justification (method + anchor ref required).
    with pytest.raises(ValidationError):
        ScientificClaim(**_claim_kw(claims_advantage=True,
                                    advantage_justification="a long string justification"))
    # Structured justification without credibility still refuses.
    with pytest.raises(ValidationError, match="credibility field"):
        ScientificClaim(**_claim_kw(
            claims_advantage=True,
            advantage_justification=AdvantageJustification(
                method="VQE RY-RY-CX depth2", anchor_ref="ledger:anchor:h2")))
    ok = ScientificClaim(**_claim_kw(
        claims_advantage=True,
        advantage_justification=AdvantageJustification(
            method="VQE RY-RY-CX", anchor_ref="ledger:anchor:h2"),
        credibility=CredibilityField(rho=0.95, tier="L3",
                                     evidence_refs=["ledger:anchor:h2"])))
    assert "[ADVANTAGE GROUNDED]" in ok.formatted_statement()


def test_grammar_tier_enums_and_ci_validator():
    from kubona.epistemics.claim_grammar import (
        CHEMICAL_ACCURACY_HA, CredibilityField, ScientificClaim)
    assert CHEMICAL_ACCURACY_HA == pytest.approx(1.6e-3)
    with pytest.raises(ValidationError):
        CredibilityField(rho=0.9, tier="L9", evidence_refs=["x"])
    with pytest.raises(ValidationError, match="ordered"):
        ScientificClaim(**_claim_kw(confidence_interval_95=(-1.135, -1.137)))
    with pytest.raises(ValidationError, match="must contain"):
        ScientificClaim(**_claim_kw(confidence_interval_95=(-1.130, -1.129)))


# ----------------------------------------------------- W1.18 continuum ---

def test_continuum_single_threshold_table():
    from kubona.epistemics.continuum import EpistemicContinuum as EC, EpistemicLevel as L
    assert EC.classify_level(1.0) is L.LEVEL_4_FORMAL_DEDUCTION
    assert EC.classify_level(0.97) is L.LEVEL_3_ANALYTICAL_ANCHOR
    assert EC.classify_level(0.95) is L.LEVEL_3_ANALYTICAL_ANCHOR
    assert EC.classify_level(0.92) is L.LEVEL_2_CLASSICAL_BASELINE
    assert EC.classify_level(0.85) is L.LEVEL_1_QUANTUM_SUBSPACE
    assert EC.classify_level(0.5) is L.LEVEL_0_EXPLORATORY_SEARCH
    with pytest.raises(ValueError):
        EC.classify_level(float("nan"))


def test_continuum_certs_unique_and_typed():
    from kubona.epistemics.continuum import (
        ComputationalHypothesis, EpistemicContinuum as EC, RefutationType)
    hyp = ComputationalHypothesis(
        hypothesis_id="h1", lean4_statement=None, anchor_system=None,
        circuit_qir=None, spend_budget_usd=1.0, mandate_signature=None,
        target_entropy_reduction=0.1)
    c1 = EC.create_negative_certificate(hyp, "BARREN_PLATEAU", "ev")
    c2 = EC.create_negative_certificate(hyp, RefutationType.BARREN_PLATEAU, "ev")
    assert c1.certificate_id != c2.certificate_id  # never deterministic overwrite
    assert c1.hypothesis_id == "h1" and c1.hypothesis_hash == hyp.content_hash()
    assert c1.refutation_type is RefutationType.BARREN_PLATEAU
    with pytest.raises(ValueError, match="refutation_type"):
        EC.create_negative_certificate(hyp, "MADE_UP", "ev")
    assert "hash-then-sign" in hyp.content_hash.__doc__


def test_continuum_free_step_requires_max_steps():
    from kubona.epistemics.continuum import EpistemicContinuum as EC
    with pytest.raises(ValueError, match="max_steps"):
        EC.evaluate_fisher_stopping_rule(1.0, 0.0)  # free + unbounded refuses
    assert EC.evaluate_fisher_stopping_rule(1.0, 0.0, max_steps=10,
                                            steps_taken=3) is False
    assert EC.evaluate_fisher_stopping_rule(1.0, 0.0, max_steps=10,
                                            steps_taken=10) is True


# ----------------------------------------------- W1.19 ansatz (bridge) ---

def test_bridge_ansatz_consts_and_labels(b_ansatz):
    assert b_ansatz.MAX_QUBITS >= 24 and b_ansatz.MAX_DEPTH >= 50
    assert b_ansatz.EXPRESSIBILITY_METHOD == "expressibility_heuristic_v1"
    g = b_ansatz.AnsatzGenome(family="HardwareEfficient", num_qubits=2, depth=1,
                              entangler_topology="linear", initializer="zero")
    assert b_ansatz._expected_param_count("RealAmplitudes", 2, 2) == 4
    assert b_ansatz._expected_param_count("HardwareEfficient", 2, 2) == 8
    mw = b_ansatz.evaluate_meyer_wallach_expressibility(g)
    assert mw["measured"] is True and mw["method"] == "statevector"
    assert 0.0 <= mw["q"] <= 1.0


def test_bridge_ansatz_validation(b_ansatz):
    with pytest.raises(ValueError, match="num_qubits"):
        b_ansatz.AnsatzGenome(family="HardwareEfficient", num_qubits=0, depth=1,
                              entangler_topology="linear", initializer="zero")
    with pytest.raises(ValueError, match="cap"):
        b_ansatz.AnsatzGenome(family="HardwareEfficient", num_qubits=31, depth=1,
                              entangler_topology="linear", initializer="zero")


# ------------------------------------------------- W1.19 optimizer ---

def _gen(**over):
    from kubona.circuits.ansatz_optimizer import AnsatzGenome
    kw = dict(family="hardware_efficient", n_qubits=2, depth=2,
              entangler_topology="linear", initializer="random",
              symmetry="none")
    kw.update(over)
    return AnsatzGenome(**kw)


def test_optimizer_heuristic_prior_labeled_t2():
    from kubona.circuits.ansatz_optimizer import AutonomousAnsatzOptimizer
    opt = AutonomousAnsatzOptimizer()
    pre = opt.precheck_genome(_gen())
    assert pre.measured is False
    assert pre.heuristic_method == "expressibility_heuristic_v1"
    assert pre.evidence_tier == "T2_CONSISTENT"  # advisory, never T1_VERIFIED
    assert pre.passed is True


def test_optimizer_expressibility_fail_has_no_placeholders():
    from kubona.circuits.ansatz_optimizer import AutonomousAnsatzOptimizer
    opt = AutonomousAnsatzOptimizer(min_heuristic_prior=0.99)
    pre = opt.precheck_genome(_gen(depth=1))
    assert pre.passed is False
    assert pre.trainability_ok is None and pre.estimated_mps_bond_dim is None
    assert pre.rejection_reason


def test_optimizer_depth_gate_single_const():
    from kubona.circuits.ansatz_optimizer import (
        AutonomousAnsatzOptimizer, MAX_TRAINABLE_DEPTH)
    opt = AutonomousAnsatzOptimizer()
    pre = opt.precheck_genome(_gen(depth=MAX_TRAINABLE_DEPTH + 1))
    assert pre.passed is False and "trainability" in pre.rejection_reason


def test_optimizer_enums_refuse():
    with pytest.raises(ValueError, match="family"):
        _gen(family="magic")
    with pytest.raises(ValueError, match="topology"):
        _gen(entangler_topology="hypercube")


def test_optimizer_greenlight_requires_measured():
    from kubona.circuits.ansatz_optimizer import AutonomousAnsatzOptimizer
    opt = AutonomousAnsatzOptimizer()
    g = _gen()
    assert opt.can_greenlight(g)["can_greenlight"] is False  # heuristic alone
    assert opt.can_greenlight(g, measured_mw=0.1, measured=True)[
        "can_greenlight"] is False  # below cutoff
    gate = opt.can_greenlight(g, measured_mw=0.8, measured=True)
    assert gate["can_greenlight"] is True


# ------------------------------------------------- W1.19 campaign ---

def test_campaign_budget_and_shot_caps(tmp_path):
    from kubona.cost.governance import GovernanceError
    from kubona.discovery.campaign import DiscoveryCampaign
    from kubona.provenance import Ledger
    camp = DiscoveryCampaign(problem="H2", hamiltonian="H2", ground_state=-1.07,
                             budget_steps=2)
    led = Ledger(path=str(tmp_path / "l.jsonl"))
    with pytest.raises(GovernanceError, match="server cap"):
        camp.run(ledger=led, budget_ceiling_usd=1000.0, source="mock")
    with pytest.raises(GovernanceError, match="server cap"):
        camp.run(ledger=led, shots=1 << 20, source="mock")


def test_campaign_single_problem_key(tmp_path):
    from kubona.discovery.campaign import DiscoveryCampaign, _resolve_anchor_molecule
    assert _resolve_anchor_molecule("h2-energetics", "H2_STO3G") == "H2_STO3G"
    camp = DiscoveryCampaign(problem="H2", hamiltonian="Z0Z1 + 0.5 X0X1",
                             ground_state=-1.07, budget_steps=2)
    from kubona.provenance import Ledger
    with pytest.raises(ValueError, match="no tabulated anchor"):
        camp.run(ledger=Ledger(path=str(tmp_path / "l.jsonl")), source="mock")


def test_campaign_zne_on_best_and_measured_confidences(tmp_path):
    from kubona.discovery.campaign import DiscoveryCampaign
    from kubona.library import AlgorithmLibrary
    from kubona.provenance import Ledger
    camp = DiscoveryCampaign(problem="H2", hamiltonian="H2", ground_state=-1.07,
                             budget_steps=5)
    out = camp.run(ledger=Ledger(path=str(tmp_path / "l.jsonl")),
                   library=AlgorithmLibrary(path=str(tmp_path / "lib.json")),
                   source="mock")
    assert out["stages"]["analyze"]["health_check_only"] is False
    assert out["all_claims_traceable"] is True
    assert out["missing_refs"] == []


# ------------------------------------------------- W1.19 dialogue ---

def test_dialogue_molecule_parser():
    from kubona.chat.dialogue import parse_molecule
    assert parse_molecule("run VQE on h2 please") == "H2"
    assert parse_molecule("LiH dissociation curve") == "LiH"
    # "because" contains "be" — substring routing must NOT fire BeH2.
    with pytest.raises(ValueError, match="unknown molecule"):
        parse_molecule("because we need results fast")
    with pytest.raises(ValueError, match="unknown molecule"):
        parse_molecule("run caffeine please")
    with pytest.raises(ValueError, match="ambiguous"):
        parse_molecule("compare h2 and lih")


def test_dialogue_cost_computed_not_hardcoded():
    from kubona.chat.dialogue import ResearchDialogueEngine
    eng = ResearchDialogueEngine(model="test-model")
    assert eng.estimate_cost_usd("T1_LOCAL_EMU") == pytest.approx(0.0)
    assert eng.estimate_cost_usd("T3_CLOUD_QPU") > 0.0
    with pytest.raises(ValueError, match="tier"):
        eng.estimate_cost_usd("T9_NOPE")


def test_dialogue_offline_surfaced_never_canned(monkeypatch):
    from kubona.chat.dialogue import ResearchDialogueEngine
    monkeypatch.delenv("KUBONA_MLX_MODEL", raising=False)
    eng = ResearchDialogueEngine()  # no model configured
    res = eng.query_local_mlx([{"role": "user", "content": "hi"}])
    assert res["ok"] is False and res["offline"] is True
    assert "verified" not in res["text"].lower()

    eng2 = ResearchDialogueEngine(model="test-model")
    import urllib.request
    monkeypatch.setattr(urllib.request, "urlopen",
                        lambda *a, **k: (_ for _ in ()).throw(
                            ConnectionRefusedError("no mlx server")))
    res2 = eng2.query_local_mlx([{"role": "user", "content": "hi"}])
    assert res2["ok"] is False and res2["offline"] is True
    assert "verified" not in res2["text"].lower()

    turn = eng2.process_turn("run h2 please")
    assert turn.proposal_card is not None  # card carries computed numbers
    assert "unavailable" in turn.content or "offline" in turn.content
    refused = eng2.process_turn("run caffeine please")
    assert refused.proposal_card is None and "Refusing" in refused.content


# ------------------------------------------------- W1.19 library ---

def test_library_validated_requires_proof(tmp_path):
    from kubona.library import AlgorithmLibrary, LibraryError
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    e = lib.add(harness="h", artifact=1.0, metric=1.0, cost_usd=0.0,
                ledger_ref="ref-1")
    assert e.validated is False  # default
    with pytest.raises(LibraryError, match="validation_proof"):
        lib.add(harness="h", artifact=1.0, metric=1.0, cost_usd=0.0,
                ledger_ref="ref-2", validated=True)


def test_library_phantom_refs_refuse(tmp_path):
    from kubona.library import AlgorithmLibrary, LibraryError
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    for phantom in ("", "none", "n/a", "  NONE  "):
        with pytest.raises(LibraryError, match="phantom"):
            lib.add(harness="h", artifact=1.0, metric=1.0, cost_usd=0.0,
                    ledger_ref=phantom)


def test_library_ledger_resolver(tmp_path):
    from kubona.library import AlgorithmLibrary, LibraryError
    from kubona.provenance import Ledger
    ledger = Ledger(path=str(tmp_path / "l.jsonl"))
    rec = ledger.append("trial", {"x": 1})
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"), ledger=ledger)
    lib.add(harness="h", artifact=1.0, metric=1.0, cost_usd=0.0,
            ledger_ref=rec.record_id)  # resolves: ok
    with pytest.raises(LibraryError, match="no ledger record"):
        lib.add(harness="h", artifact=1.0, metric=1.0, cost_usd=0.0,
                ledger_ref="deadbeef-nope")


def test_library_best_direction_explicit(tmp_path):
    from kubona.library import AlgorithmLibrary, LibraryError
    from kubona.provenance import Ledger
    ledger = Ledger(path=str(tmp_path / "l.jsonl"))
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"), ledger=ledger)
    r1 = ledger.append("t", {"i": 1}).record_id
    r2 = ledger.append("t", {"i": 2}).record_id
    lib.add(harness="h", artifact="lo", metric=1.0, cost_usd=0.0,
            ledger_ref=r1, validated=True, validation_proof=r1)
    lib.add(harness="h", artifact="hi", metric=2.0, cost_usd=0.0,
            ledger_ref=r2, validated=True, validation_proof=r2)
    with pytest.raises(TypeError):
        lib.best("h")  # direction required — never guessed
    with pytest.raises(LibraryError, match="direction"):
        lib.best("h", direction="sideways")
    assert lib.best("h", direction="max").artifact == "hi"
    assert lib.best("h", direction="min").artifact == "lo"


def test_library_serializability_and_tenant(tmp_path):
    from kubona.library import AlgorithmLibrary, LibraryAccessError, LibraryError
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    with pytest.raises(LibraryError, match="serializable"):
        lib.add(harness="h", artifact=object(), metric=1.0, cost_usd=0.0,
                ledger_ref="ref-1")
    scoped = lib.add(harness="h", artifact=1.0, metric=1.0, cost_usd=0.0,
                     ledger_ref="ref-2", owner="pi", tenant="lab-a")
    assert scoped.owner == "pi" and scoped.tenant == "lab-a"
    with pytest.raises(LibraryAccessError):
        lib.get(scoped.entry_id, requester_tenant="lab-b")
    assert lib.get(scoped.entry_id, requester_tenant="lab-a").entry_id == scoped.entry_id
    pub = lib.add(harness="h", artifact=2.0, metric=2.0, cost_usd=0.0,
                  ledger_ref="ref-3")
    assert lib.get(pub.entry_id).entry_id == pub.entry_id  # public reads open
    with pytest.raises(LibraryAccessError):
        lib.authorize_sale(scoped.entry_id, buyer_tenant="lab-b")
    lib.authorize_sale(scoped.entry_id, buyer_tenant="lab-a")


# ------------------------------------------- W1.20 negative results ---

def test_negative_results_chain_and_labels(tmp_path):
    from kubona.ledger.negative_results import NegativeResultsLedger, UNSIGNED_MODE
    led = NegativeResultsLedger(storage_dir=str(tmp_path))
    r1 = led.record_refutation(
        "vqe_ground_state", "Z0Z1 + 0.5 X0X1", "HardwareEfficient",
        "BARREN_PLATEAU", -0.5, -1.07, attributor="agent-1",
        objective_direction="min")
    r2 = led.record_refutation(
        "vqe_ground_state", "0.5X0X1+Z0Z1", "HardwareEfficient",
        "CLASSICAL_SUPERIOR", -0.4, -1.07, attributor="agent-1",
        objective_direction="min")
    assert r2.prev_hash == r1.record_hash
    assert r1.signing_mode == UNSIGNED_MODE
    assert r1.signature_hex is None
    assert led.verify_chain()[0] is True
    # Normalized hash: spacing/ordering-insensitive similarity.
    assert r1.hamiltonian_hash == r2.hamiltonian_hash
    assert len(led.search_similar_failures("Z0Z1+0.5 X0X1", "HardwareEfficient")) == 2


def test_negative_results_signed_mode(tmp_path):
    import os
    from kubona.ledger.negative_results import NegativeResultsLedger
    led = NegativeResultsLedger(storage_dir=str(tmp_path),
                                signing_key=os.urandom(32))
    rec = led.record_refutation(
        "vqe_ground_state", "H2", "UCCSD", "BUDGET_EXHAUSTED", -1.0, -1.07,
        attributor="pi", objective_direction="min")
    assert rec.signing_mode == "ed25519-signed"
    assert rec.signature_hex and rec.signer_pubkey_hex
    assert led.verify_chain()[0] is True


def test_negative_results_refusals_and_corrupt_count(tmp_path):
    import pytest as _pt
    from kubona.ledger.negative_results import NegativeResultsLedger
    led = NegativeResultsLedger(storage_dir=str(tmp_path))
    with _pt.raises(ValueError, match="attributor"):
        led.record_refutation("t", "H", "A", "BARREN_PLATEAU", 0.0, 0.0,
                              attributor="  ", objective_direction="min")
    with _pt.raises(ValueError, match="failure_mode"):
        led.record_refutation("t", "H", "A", "NOPE", 0.0, 0.0,
                              attributor="x", objective_direction="min")
    with _pt.raises(ValueError, match="finite"):
        led.record_refutation("t", "H", "A", "BARREN_PLATEAU", float("nan"),
                              0.0, attributor="x", objective_direction="min")
    with _pt.raises(ValueError, match="objective_direction"):
        led.record_refutation("t", "H", "A", "BARREN_PLATEAU", 0.0, 0.0,
                              attributor="x", objective_direction="up")
    led.record_refutation("t", "H", "A", "BARREN_PLATEAU", 0.0, 0.0,
                          attributor="x", objective_direction="min")
    with open(led.ledger_path, "a") as f:
        f.write("{corrupt line\n")
    recs, skipped = led.load()
    assert len(recs) == 1 and skipped == 1
    assert led.last_skipped_corrupt == 1


def test_negative_results_default_path_under_state(tmp_path, monkeypatch):
    import os
    from kubona.config import state_path
    from kubona.ledger.negative_results import NegativeResultsLedger
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    led = NegativeResultsLedger()
    assert led.ledger_path == state_path("negative_results.jsonl")
    assert "/tmp/" not in str(led.ledger_path)


# ------------------------------------------------- W1.20 canary ---

def test_canary_empty_counts_refuse():
    import kubona.providers.local_simulator as ls
    from kubona.canary import canary_check
    orig = ls.LocalSimulator.submit

    def empty(self, circuit, shots, seed=None):
        return {"counts": {}, "shots": shots, "seed": seed}

    ls.LocalSimulator.submit = empty
    try:
        res = canary_check("local-simulator", shots=64)
        assert res["ok"] is False
        assert "empty counts" in res["error"]
    finally:
        ls.LocalSimulator.submit = orig


def test_canary_calibration_false_in_checks():
    import kubona.schema as schema_pkg
    from kubona.canary import canary_check
    from kubona.schema.provider import CalibrationSnapshot
    real_load = schema_pkg.load_provider
    cap = real_load("local-simulator").model_copy(
        update={"declared_calibration": CalibrationSnapshot(
            t1_us=10.0, t2_us=10.0, single_qubit_error=0.0,
            two_qubit_error=0.0, readout_error=0.0, source="fabricated")})
    schema_pkg.load_provider = lambda name: cap
    try:
        res = canary_check("local-simulator", shots=256)
        assert res["attestation"]["calibration"]["calibration_ok"] is False
        assert res["checks"]["calibration_ok"] is False  # in checks, not side-channel
        assert res["ok"] is False
    finally:
        schema_pkg.load_provider = real_load


def test_canary_capacity_range_rule():
    import kubona.schema as schema_pkg
    from kubona.canary import attest_capacity
    real_load = schema_pkg.load_provider
    cap = real_load("local-simulator").model_copy(update={"physical_qubits": 999})
    schema_pkg.load_provider = lambda name: cap
    try:
        assert attest_capacity("local-simulator", True)["qubit_count_ok"] is False
    finally:
        schema_pkg.load_provider = real_load
    assert attest_capacity("local-simulator", True)["qubit_count_ok"] is True
