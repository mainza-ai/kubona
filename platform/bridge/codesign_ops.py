"""Codesign operations for the bridge gRPC rail (T-01b).

Single home for the validation + harness logic that lived in Go string
constants (codesign_eval.go snippets). The dev fallback imports this
module through a 6-line shim, so there is one shared module — not five
stranded copies. Error strings are pinned byte-identical to the retired
snippets: handler tests assert them, and the Go rail maps the outer
``ok`` flag to status exactly as before.
"""

from kubona.codesign import (
    BudgetEntry,
    CircuitCodesignHarness,
    FamilySearch,
    MapElitesFrontier,
    build_spec,
    build_truth_table_spec,
    kappa_sweep,
    ledger_rows,
    union_bound,
)
from kubona.codesign.factories import factory_report


def strict_int(v, name, lo, hi):
    if isinstance(v, bool) or not isinstance(v, int) or not (lo <= v <= hi):
        raise ValueError(f"{name} must be an int in [{lo},{hi}]: refusing")
    return v


def _spec_and_label(body):
    """Shared spec_table / spec_kind block (eval, batch, families)."""
    if "spec_table" in body and body["spec_table"] is not None:
        st = body["spec_table"]
        if not isinstance(st, dict):
            raise ValueError("spec_table must be an object: refusing")
        ins, outs, table = st.get("inputs", []), st.get("outputs", []), st.get("table", {})
        if not isinstance(ins, list) or not isinstance(outs, list) or not isinstance(table, dict):
            raise ValueError("spec_table needs inputs[]/outputs[]/table{}: refusing")
        spec = build_truth_table_spec(
            strict_int(st.get("num_qubits", 0), "num_qubits", 1, 64),
            [strict_int(q, "inputs[]", 0, 63) for q in ins],
            [strict_int(q, "outputs[]", 0, 63) for q in outs],
            table)
        return spec, "table"
    kind = body.get("spec_kind")
    return build_spec(str(kind)), str(kind)


def op_spec(body):
    """Return the fixed setup metadata for a spec without evaluating an
    artifact. Stateless; the Go rerun rail uses it to size the prompt and
    to fail before any LLM egress when the spec is invalid."""
    spec, label = _spec_and_label(body)
    return {"ok": True, "spec_kind": label, "num_qubits": spec.num_qubits,
            "inputs": list(spec.inputs), "outputs": list(spec.outputs),
            "ancillas": list(spec.ancillas)}


def op_evaluate(body):
    spec, label = _spec_and_label(body)
    num_tests = strict_int(body.get("num_tests", 64), "num_tests", 1, 4096)
    max_failures = strict_int(body.get("max_failures", 0), "max_failures", 0, num_tests)
    kappa = strict_int(body.get("kappa", 1), "kappa", 1, 3)
    seed = strict_int(body.get("seed", 7), "seed", 0, 2**31 - 1)
    h = CircuitCodesignHarness(spec, num_tests=num_tests,
                               max_failures=max_failures, kappa=kappa, seed=seed)
    res = h.run(body.get("artifact", []))
    if res.get("valid"):
        res["kappa_sweep"] = {str(k): v for k, v in
                              kappa_sweep(res["toffoli"], res["peak_qubits"]).items()}
    fac, cd, cu = body.get("factory"), body.get("code_distance"), body.get("cycle_us")
    if fac is None and (cd is not None or cu is not None):
        raise ValueError("factory diagnostics need factory + code_distance + cycle_us together: refusing")
    if fac is not None:
        if cd is None or cu is None:
            raise ValueError("factory diagnostics need factory + code_distance + cycle_us together: refusing (no silent physics)")
        res["factory"] = factory_report(toffoli=res["toffoli"], qubits=res["peak_qubits"],
                                        toffoli_depth=res.get("toffoli_depth", 0), factory=fac,
                                        code_distance=cd, cycle_us=cu)
    return {"ok": True, "spec_kind": label, "result": res}


def op_batch(body):
    spec, label = _spec_and_label(body)
    artifacts = body.get("artifacts", [])
    kappa = strict_int(body.get("kappa", 1), "kappa", 1, 3)
    if not isinstance(artifacts, list) or len(artifacts) > 64:
        raise ValueError("artifacts must be a list of <=64: refusing")
    h = CircuitCodesignHarness(spec, num_tests=32, kappa=kappa)
    f = MapElitesFrontier(kappa=kappa)
    details = []
    for art in artifacts:
        r = h.run(art)
        details.append({"valid": r["valid"], "toffoli": r["toffoli"],
                        "peak_qubits": r["peak_qubits"], "tq": r["tq"],
                        "eps_bound_97_5": r.get("eps_bound_97_5"),
                        "depth": r.get("depth"),
                        "toffoli_depth": r.get("toffoli_depth"),
                        "active_qubits": r.get("active_qubits")})
        if r["valid"]:
            f.offer(art, r["toffoli"], r["peak_qubits"], {"tq": r["tq"]})
    return {"ok": True, "spec_kind": label,
            "frontier": f.to_dict(), "details": details}


def op_families(body):
    spec, label = _spec_and_label(body)
    families = body.get("families", [])
    kappa = strict_int(body.get("kappa", 1), "kappa", 1, 3)
    if not isinstance(families, list) or not (1 <= len(families) <= 8):
        raise ValueError("families must be a list of 1..8 distinct names: refusing")
    offers_in = body.get("offers", [])
    if not isinstance(offers_in, list) or len(offers_in) > 64:
        raise ValueError("offers must be a list of <=64: refusing")
    share_rounds = strict_int(body.get("share_rounds", 1), "share_rounds", 0, 3)
    num_tests = 32
    fs = FamilySearch(families, kappa=kappa)
    h = CircuitCodesignHarness(spec, num_tests=num_tests, max_failures=0,
                               kappa=kappa, seed=7)
    offers = []
    for o in offers_in:
        if not isinstance(o, dict) or "family" not in o or "artifact" not in o:
            raise ValueError("each offer needs {family, artifact}: refusing")
        fam = o["family"]
        r = h.run(o["artifact"])
        if not r["valid"]:
            offers.append({"family": fam, "valid": False})
            continue
        improved = fs.offer(fam, o["artifact"], r["toffoli"], r["peak_qubits"])
        offers.append({"family": fam, "valid": True,
                       "toffoli": r["toffoli"], "qubits": r["peak_qubits"],
                       "tq": r["tq"], "improved": improved})
    shares = []
    for _ in range(share_rounds):
        shares.append(fs.share_elites())
    return {"ok": True, "spec_kind": label, "kappa": kappa,
            "offers": offers, "elites": fs.elites(),
            "stagnation": dict(fs._stagnant),
            "lineage_len": len(fs.lineage),
            "lineage": fs.lineage[-128:],
            "shares": shares, "widen_nudge": fs.widen_nudge()}


def op_packs(body):
    from kubona.codesign.packs import PACK_ORDER, default_registry
    reg = default_registry()
    return {"ok": True, "order": list(PACK_ORDER), "packs": [
        {"name": name, "version": p.version,
         "anchor_required": p.anchor_required,
         "evidence_reqs": list(p.evidence_reqs), "status": p.status,
         "anchor_ref": p.anchor_ref, "proof_ref": p.proof_ref}
        for name, p in reg.items()]}


def op_budget(body):
    raw = body.get("entries", [])
    if not isinstance(raw, list) or len(raw) > 32:
        raise ValueError("entries must be a list of <=32: refusing")
    entries = []
    for e in raw:
        if not isinstance(e, dict):
            raise ValueError(f"entry must be an object, got {e!r}: refusing")
        rate = e.get("per_use_rate", float("nan"))
        if isinstance(rate, bool) or not isinstance(rate, (int, float)):
            raise ValueError(f"per_use_rate must be a number, got {rate!r}: refusing")
        entries.append(BudgetEntry(str(e.get("source", "")),
                                   float(rate),
                                   strict_int(e.get("uses", -1), "uses", 0, 10**9)))
    return {"ok": True, "rows": ledger_rows(entries),
            "union_bound": union_bound(entries)}


# Closed operation vocabulary: the RPC maps names to functions, never
# execs caller-supplied code. Failure labels match the retired snippets
# byte-for-byte (handler tests pin them).
_OPERATIONS = {
    "spec": (op_spec, "spec refused"),
    "evaluate": (op_evaluate, "evaluation refused"),
    "batch": (op_batch, "frontier refused"),
    "families": (op_families, "families refused"),
    "packs": (op_packs, "registry refused"),
    "budget": (op_budget, "budget refused"),
}


def run_op(operation, body):
    fn_entry = _OPERATIONS.get(operation)
    if fn_entry is None:
        return {"ok": False, "error": f"unknown codesign operation {operation!r}: refusing"}
    fn, failure_label = fn_entry
    try:
        if not isinstance(body, dict):
            raise ValueError("body must be a JSON object: refusing")
        return fn(body)
    except ValueError as e:
        return {"ok": False, "error": str(e)[:280]}
    except Exception as e:
        return {"ok": False, "error": f"{failure_label}: {e}"[:280]}
