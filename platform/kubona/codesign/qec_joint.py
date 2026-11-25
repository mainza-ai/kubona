"""Phase 4 Cross-Domain QEC + Circuit Codesign verification rail.

Provides real Stim-backed verification for logical gates on QEC codes
(stabilizer preservation, logical Pauli action, exhaustive single-fault
propagation), exact S5 magic-state factory verification (distance >= 3 and
yield exponent gamma via `kubona.qec.magic_state`), and joint QEC/Codesign
claim bundling. Fail-closed: never fabricates fault-tolerance verdicts.
"""

from __future__ import annotations

import hashlib
import json
from typing import Any

import numpy as np
import stim

from kubona.codesign.claims import canonical_circuit_sha256
from kubona.codesign.factories import factory_report
from kubona.qec.magic_state import (
    S5_MAGIC_STATE_TARGETS,
    compute_gamma,
    hamming_15_1_3_parity_matrix,
    verify_magic_state_syndromes,
)

# Published FunSearch depth benchmark for [[5,1,3]] logical CNOT synthesis
# (reference metadata only — not reproduced in this repository).
FUNSEARCH_5_1_3_CNOT_BENCHMARK: dict[str, Any] = {
    "code": "[[5,1,3]]",
    "gate": "logical_cnot",
    "funsearch_depth": 9,
    "baseline_depth": 84,
    "status": "reference_only_not_reproduced",
    "reproduced_in_repo": False,
    "source": "FunSearch / logical-gate synthesis benchmark (literature reference only)",
}

# Canonical Steane [[7,1,3]] CSS stabilizers (qubits 0..6)
STEANE_7_1_3_STABS: tuple[str, ...] = (
    "IIIXXXX",
    "IXXIIXX",
    "XIXIXIX",
    "IIIZZZZ",
    "IZZIIZZ",
    "ZIZIZIZ",
)
STEANE_7_1_3_LOG_X = "XXXXXXX"
STEANE_7_1_3_LOG_Z = "ZZZZZZZ"

# Canonical Five-Qubit [[5,1,3]] stabilizers (qubits 0..4)
FIVE_QUBIT_5_1_3_STABS: tuple[str, ...] = (
    "XZZXI",
    "IXZZX",
    "XIXZZ",
    "ZXIXZ",
)
FIVE_QUBIT_5_1_3_LOG_X = "XXXXX"
FIVE_QUBIT_5_1_3_LOG_Z = "ZZZZZ"


def _pauli_to_symplectic(p: stim.PauliString) -> np.ndarray:
    """Convert a Stim PauliString of length n into a 2n binary symplectic vector [x | z]."""
    n = len(p)
    x = np.zeros(n, dtype=np.uint8)
    z = np.zeros(n, dtype=np.uint8)
    for i in range(n):
        v = p[i]
        if v in (1, 2):  # X or Y
            x[i] = 1
        if v in (2, 3):  # Y or Z
            z[i] = 1
    return np.concatenate([x, z])


def _gf2_rank(mat: np.ndarray) -> int:
    """Exact rank of a binary matrix over GF(2)."""
    A = (np.asarray(mat, dtype=np.uint8) & 1).copy()
    if A.size == 0:
        return 0
    rows, cols = A.shape
    r = 0
    for c in range(cols):
        if r >= rows:
            break
        piv = None
        for i in range(r, rows):
            if A[i, c]:
                piv = i
                break
        if piv is None:
            continue
        if piv != r:
            A[[r, piv]] = A[[piv, r]]
        for i in range(rows):
            if i != r and A[i, c]:
                A[i] ^= A[r]
        r += 1
    return r


def _in_gf2_rowspace(vec: np.ndarray, basis: np.ndarray) -> bool:
    """Return True iff binary row vector `vec` lies in the GF(2) rowspace of `basis`."""
    if basis.size == 0:
        return not np.any(vec & 1)
    base_rank = _gf2_rank(basis)
    aug = np.vstack([basis, (vec & 1).reshape(1, -1)])
    return _gf2_rank(aug) == base_rank


def _commutes_with_all(p: stim.PauliString, stabs: list[stim.PauliString]) -> bool:
    return all(p.commutes(s) for s in stabs)


def _kickmix_to_stim_circuit(ops: list[tuple[str, list[int], list[int]]], num_qubits: int) -> tuple[stim.Circuit, int]:
    """Convert a Clifford Kickmix op list into a Stim circuit and compute its parallel depth."""
    circ = stim.Circuit()
    if num_qubits > 0:
        circ.append("I", list(range(num_qubits)))
    qubit_layer = [0] * num_qubits
    gate_map = {"x": "X", "y": "Y", "z": "Z", "h": "H", "s": "S", "cx": "CX", "cnot": "CX", "cz": "CZ", "swap": "SWAP"}
    for op in ops:
        if not isinstance(op, (list, tuple)) or len(op) < 2:
            raise ValueError(f"malformed op {op!r}: expected [gate, qubits, ...]")
        gname = str(op[0]).lower()
        qlist = [int(q) for q in op[1]]
        if any(q < 0 or q >= num_qubits for q in qlist):
            raise ValueError(f"qubit index out of range [0, {num_qubits - 1}] in {op!r}")
        if gname not in gate_map:
            raise ValueError(f"non-Clifford or unsupported gate {gname!r} for Stim logical-gate verification")
        stim_gate = gate_map[gname]
        circ.append(stim_gate, qlist)
        layer = max((qubit_layer[q] for q in qlist), default=0) + 1
        for q in qlist:
            qubit_layer[q] = layer
    return circ, max(qubit_layer, default=0)


def verify_logical_gate_stim(
    *,
    code_name: str,
    num_qubits: int,
    stabilizers: list[str],
    logical_checks: list[tuple[str, str]],
    ops: list[tuple[str, list[int], list[int]]],
    check_single_faults: bool = True,
) -> dict[str, Any]:
    """Verify a Clifford logical gate circuit using Stim's exact tableau propagation.

    Checks:
      1. Stabilizer preservation: every stabilizer generator maps into <S>.
      2. Logical action: for each (in_pauli, expected_out_pauli), U(in_pauli) * expected_out_pauli in <S>.
      3. Exhaustive single-fault check: every single-qubit Pauli {X, Y, Z} injected after
         every gate either triggers a non-zero stabilizer syndrome or lies in <S>
         (no undetectable logical error from any single fault).
    """
    circ, depth = _kickmix_to_stim_circuit(ops, num_qubits)
    tableau = stim.Tableau.from_circuit(circ)

    stab_ps = [stim.PauliString(s) for s in stabilizers]
    stab_basis = np.vstack([_pauli_to_symplectic(s) for s in stab_ps]) if stab_ps else np.zeros((0, 2 * num_qubits), dtype=np.uint8)

    # 1. Stabilizer preservation
    stabilizers_preserved = True
    broken_stabilizers: list[str] = []
    for s_str, s_p in zip(stabilizers, stab_ps):
        img = tableau(s_p)
        if not _in_gf2_rowspace(_pauli_to_symplectic(img), stab_basis):
            stabilizers_preserved = False
            broken_stabilizers.append(s_str)

    # 2. Logical action equivalence up to stabilizers
    logical_action_exact = True
    failed_logical_checks: list[dict[str, str]] = []
    for inp_str, exp_str in logical_checks:
        img = tableau(stim.PauliString(inp_str))
        exp = stim.PauliString(exp_str)
        diff = img * exp
        if not _in_gf2_rowspace(_pauli_to_symplectic(diff), stab_basis):
            logical_action_exact = False
            failed_logical_checks.append({"input": inp_str, "expected": exp_str, "got": str(img)})

    # 3. Exhaustive single-fault propagation on ALL qubits before gate 0 and after every gate
    # (C6 fix: covers pre-gate, post-gate, and idle-qubit faults at arbitrary circuit depth).
    faults_tested = 0
    undetectable_logical_faults = 0
    if check_single_faults and stabilizers_preserved:
        n_ops = len(ops)
        all_qubits = list(range(num_qubits))
        for step_idx in range(n_ops + 1):
            suffix_circ, _ = _kickmix_to_stim_circuit(ops[step_idx:], num_qubits)
            suffix_tab = stim.Tableau.from_circuit(suffix_circ)
            for q in all_qubits:
                for pchar in ("X", "Y", "Z"):
                    faults_tested += 1
                    err = stim.PauliString(num_qubits)
                    err[q] = pchar
                    prop = suffix_tab(err)
                    if _commutes_with_all(prop, stab_ps):
                        # Undetectable by stabilizers: safe iff it is itself a stabilizer
                        if not _in_gf2_rowspace(_pauli_to_symplectic(prop), stab_basis):
                            undetectable_logical_faults += 1

    passed = stabilizers_preserved and logical_action_exact and (undetectable_logical_faults == 0)
    return {
        "ok": True,
        "code_name": code_name,
        "num_qubits": num_qubits,
        "gate_count": len(ops),
        "depth": depth,
        "stabilizers_preserved": stabilizers_preserved,
        "broken_stabilizers": broken_stabilizers[:8],
        "logical_action_exact": logical_action_exact,
        "failed_logical_checks": failed_logical_checks[:8],
        "single_fault_check": {
            "enabled": check_single_faults,
            "scope": "all_qubits_every_step_pre_and_post",
            "faults_tested": faults_tested,
            "undetectable_logical_faults": undetectable_logical_faults,
            "passed": undetectable_logical_faults == 0,
        },
        "passed": passed,
        "artifact_sha256": canonical_circuit_sha256(ops),
        "verifier": "stim_tableau_exact",
    }


def verify_steane_logical_cnot(ops: list[tuple[str, list[int], list[int]]] | None = None) -> dict[str, Any]:
    """Verify transversal logical CNOT on two Steane [[7,1,3]] blocks (14 qubits)."""
    if ops is None:
        ops = [("cx", [i, i + 7], []) for i in range(7)]
    stabs = [s + "I" * 7 for s in STEANE_7_1_3_STABS] + ["I" * 7 + s for s in STEANE_7_1_3_STABS]
    logical_checks = [
        (STEANE_7_1_3_LOG_X + "I" * 7, STEANE_7_1_3_LOG_X + STEANE_7_1_3_LOG_X),
        ("I" * 7 + STEANE_7_1_3_LOG_X, "I" * 7 + STEANE_7_1_3_LOG_X),
        (STEANE_7_1_3_LOG_Z + "I" * 7, STEANE_7_1_3_LOG_Z + "I" * 7),
        ("I" * 7 + STEANE_7_1_3_LOG_Z, STEANE_7_1_3_LOG_Z + STEANE_7_1_3_LOG_Z),
    ]
    return verify_logical_gate_stim(
        code_name="steane_7_1_3",
        num_qubits=14,
        stabilizers=stabs,
        logical_checks=logical_checks,
        ops=ops,
    )


def verify_five_qubit_logical_gate(
    ops: list[tuple[str, list[int], list[int]]] | None = None,
    *,
    gate: str = "logical_sh",
) -> dict[str, Any]:
    """Verify a logical gate on [[5,1,3]] with Stim and compare depth to the FunSearch benchmark."""
    if gate == "logical_cnot":
        # 2-block [[5,1,3]] logical CNOT verifier (10 qubits)
        if ops is None:
            # Explicit refusal if caller provides no circuit; we never invent a fake pass
            raise ValueError("ops is required for [[5,1,3]] logical_cnot verification")
        stabs = [s + "I" * 5 for s in FIVE_QUBIT_5_1_3_STABS] + ["I" * 5 + s for s in FIVE_QUBIT_5_1_3_STABS]
        logical_checks = [
            (FIVE_QUBIT_5_1_3_LOG_X + "I" * 5, FIVE_QUBIT_5_1_3_LOG_X + FIVE_QUBIT_5_1_3_LOG_X),
            ("I" * 5 + FIVE_QUBIT_5_1_3_LOG_X, "I" * 5 + FIVE_QUBIT_5_1_3_LOG_X),
            (FIVE_QUBIT_5_1_3_LOG_Z + "I" * 5, FIVE_QUBIT_5_1_3_LOG_Z + "I" * 5),
            ("I" * 5 + FIVE_QUBIT_5_1_3_LOG_Z, FIVE_QUBIT_5_1_3_LOG_Z + FIVE_QUBIT_5_1_3_LOG_Z),
        ]
        res = verify_logical_gate_stim(
            code_name="five_qubit_5_1_3",
            num_qubits=10,
            stabilizers=stabs,
            logical_checks=logical_checks,
            ops=ops,
        )
        res["benchmark"] = dict(FUNSEARCH_5_1_3_CNOT_BENCHMARK)
        res["depth_vs_funsearch"] = res["depth"] - FUNSEARCH_5_1_3_CNOT_BENCHMARK["funsearch_depth"]
        return res

    # Default: transversal K = H*S (cycles X->Y->Z->X) on [[5,1,3]]
    if ops is None:
        ops = [("s", [i], []) for i in range(5)] + [("h", [i], []) for i in range(5)]
    # Under H*S: X -> S(X)=Y -> H(Y)=-Y; Z -> S(Z)=Z -> H(Z)=X (up to sign/stabilizers)
    logical_checks = [
        (FIVE_QUBIT_5_1_3_LOG_Z, FIVE_QUBIT_5_1_3_LOG_X),
        (FIVE_QUBIT_5_1_3_LOG_X, "YYYYY"),
    ]
    res = verify_logical_gate_stim(
        code_name="five_qubit_5_1_3",
        num_qubits=5,
        stabilizers=list(FIVE_QUBIT_5_1_3_STABS),
        logical_checks=logical_checks,
        ops=ops,
    )
    res["benchmark"] = dict(FUNSEARCH_5_1_3_CNOT_BENCHMARK)
    return res


def verify_magic_state_factory(
    *,
    n: int = 15,
    k: int = 1,
    d: int = 3,
    h_mat: np.ndarray | None = None,
    factory_name: str = "gf2019_ccz",
    code_distance: int = 15,
    cycle_us: float = 1.0,
) -> dict[str, Any]:
    """Verify a magic-state distillation factory via exact S5 syndromes + gamma + footprint."""
    if h_mat is None:
        if (n, k, d) != (15, 1, 3):
            raise ValueError("h_mat is required for non-(15,1,3) magic-state factories: refusing")
        h_mat = hamming_15_1_3_parity_matrix()
    syn = verify_magic_state_syndromes(h_mat)
    gamma = compute_gamma(n, k, d)
    footprint = factory_report(
        toffoli=15,
        qubits=n,
        toffoli_depth=1,
        factory=factory_name,
        code_distance=code_distance,
        cycle_us=cycle_us,
    )
    return {
        "ok": True,
        "n": n,
        "k": k,
        "d": d,
        "method": syn["method"],
        "distance_ge_3": syn["distance_ge_3"],
        "gamma": gamma,
        "syndromes": syn,
        "footprint": footprint,
        "s5_reference_targets": S5_MAGIC_STATE_TARGETS,
    }


def build_joint_qec_codesign_claim(
    *,
    claim_id: str,
    qec_code_name: str,
    qec_code_record: dict[str, Any],
    stim_verification: dict[str, Any],
    factory_verification: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Assemble a joint QEC + Circuit Codesign claim bundle for sign-off and publication."""
    qec_proved = (
        qec_code_record.get("d_kind") == "proved"
        and int(qec_code_record.get("d", 0)) >= 3
    )
    stim_ok = bool(stim_verification.get("passed"))
    factory_ok = True if factory_verification is None else bool(factory_verification.get("distance_ge_3"))
    ready = qec_proved and stim_ok and factory_ok
    payload = {
        "claim_id": claim_id,
        "kind": "joint_qec_circuit",
        "qec_code_name": qec_code_name,
        "qec_code_record": qec_code_record,
        "stim_verification": stim_verification,
        "factory_verification": factory_verification,
        "gates": {
            "qec_distance_proved": qec_proved,
            "stim_logical_and_single_fault_passed": stim_ok,
            "factory_distance_ge_3": factory_ok,
        },
        "ready_for_signoff": ready,
    }
    digest = hashlib.sha256(json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest()
    payload["bundle_sha256"] = digest
    return payload
