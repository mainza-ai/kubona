"""Named + truth-table TaskSpec registry (fail-closed).

Named kinds keep the console/API contract small and auditable; generic
truth tables cover new toys without code changes, but only when the table
is exhaustive for n_inputs <= 8 (proof by enumeration). Larger domains
refuse here — the sampling path (named specs + input_distribution) is the
honest route, not a partial table presented as verification.
"""

from __future__ import annotations

from typing import Any

from .kickmix import TaskSpec

NAMED_KINDS = ("and2", "cz-phase", "copy1", "xor2")


def build_spec(kind: str) -> TaskSpec:
    if kind == "and2":
        def _perm(bits):
            out = dict(bits)
            out[2] = bits.get(2, 0) ^ (bits.get(0, 0) & bits.get(1, 0))
            return out
        return TaskSpec(num_qubits=3, inputs=(0, 1), outputs=(2,), target_perm=_perm)
    if kind == "cz-phase":
        return TaskSpec(
            num_qubits=2, inputs=(0, 1), outputs=(),
            target_perm=lambda bits: dict(bits),
            target_phase=lambda bits: bits.get(0, 0) & bits.get(1, 0),
        )
    if kind == "copy1":
        def _copy(bits):
            out = dict(bits)
            out[1] = bits.get(0, 0)
            return out
        return TaskSpec(num_qubits=2, inputs=(0,), outputs=(1,), target_perm=_copy)
    if kind == "xor2":
        def _xor(bits):
            out = dict(bits)
            out[2] = bits.get(2, 0) ^ bits.get(0, 0) ^ bits.get(1, 0)
            return out
        return TaskSpec(num_qubits=3, inputs=(0, 1), outputs=(2,), target_perm=_xor)
    raise ValueError(f"unknown spec_kind {kind!r} (known: {list(NAMED_KINDS)}): refusing")


def build_truth_table_spec(
    num_qubits: int,
    inputs: list[int],
    outputs: list[int],
    table: dict[str, dict[str, Any]],
) -> TaskSpec:
    """Build a spec from an explicit truth table. Fail-closed validation.

    table: input-bitstring (len == len(inputs), chars 0/1) ->
           {"out": {qubit_id_str: 0/1, ...}, "phase": 0/1 (opt)}.
    Requires exhaustive coverage when len(inputs) <= 8; larger refuses.
    Qubit ids must be in range; inputs/outputs disjoint-check is structural
    (outputs may overlap inputs for in-place XOR tasks — allowed, since the
    evaluator checks preservation of *inputs* and correctness of *outputs*).
    """
    if not isinstance(num_qubits, int) or num_qubits < 1:
        raise ValueError(f"num_qubits must be positive int, got {num_qubits!r}")
    ins = tuple(int(x) for x in inputs)
    outs = tuple(int(x) for x in outputs)
    if len(set(ins)) != len(ins) or any(q < 0 or q >= num_qubits for q in ins):
        raise ValueError(f"bad inputs {inputs!r}: refusing")
    if len(set(outs)) != len(outs) or any(q < 0 or q >= num_qubits for q in outs):
        raise ValueError(f"bad outputs {outputs!r}: refusing")
    n = len(ins)
    if n > 8:
        raise ValueError(f"truth-table specs refuse n_inputs={n} > 8: use sampling path")
    if not isinstance(table, dict):
        raise ValueError("table must be a dict: refusing")
    want_keys = {format(m, f"0{n}b") for m in range(1 << n)} if n else {""}
    if set(table) != want_keys:
        missing = sorted(want_keys - set(table))[:4]
        extra = sorted(set(table) - want_keys)[:4]
        raise ValueError(
            f"table must cover all {1 << n} inputs exactly (missing={missing}, extra={extra}): refusing")
    norm: dict[str, tuple[dict[int, int], int]] = {}
    for k, v in table.items():
        if not isinstance(v, dict) or "out" not in v or not isinstance(v["out"], dict):
            raise ValueError(f"table[{k!r}] needs {{'out': {{qid: bit}}}}: refusing")
        out_bits: dict[int, int] = {}
        for qk, bv in v["out"].items():
            try:
                q = int(qk)
            except (TypeError, ValueError):
                raise ValueError(f"table[{k!r}] bad qubit id {qk!r}: refusing")
            if q not in outs:
                raise ValueError(f"table[{k!r}] output {q} not in outputs {outs}: refusing")
            if bv not in (0, 1):
                raise ValueError(f"table[{k!r}] bit must be 0/1, got {bv!r}: refusing")
            out_bits[q] = bv
        if set(out_bits) != set(outs):
            raise ValueError(f"table[{k!r}] must assign every output {sorted(outs)}: refusing")
        ph = v.get("phase", 0)
        if ph not in (0, 1):
            raise ValueError(f"table[{k!r}] phase must be 0/1: refusing")
        norm[k] = (out_bits, ph)

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        key = "".join(str(bits.get(q, 0) & 1) for q in ins)
        out_bits, _ = norm[key]
        res = dict(bits)
        res.update(out_bits)
        return res

    def _phase(bits: dict[int, int]) -> int:
        key = "".join(str(bits.get(q, 0) & 1) for q in ins)
        return norm[key][1]

    return TaskSpec(num_qubits=num_qubits, inputs=ins, outputs=outs,
                    target_perm=_perm, target_phase=_phase)
