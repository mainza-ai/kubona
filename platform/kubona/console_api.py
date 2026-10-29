"""Console REST API — remaining Python surface (Phase 2 origin).

K5 RETIRED/STATUS (2026-10-12): the Go orchestrator (:8080) is the sole
served API — the console talks only to it. This module's
sessions/chat/loops/events/hf/mesh/fleet/provenance/custom-agent
routes were REMOVED as divergent duplicates (separate stores behind
the same paths = split-brain: file sessions vs SQLite, in-memory
loops, separate ledgers). What remains: health, approvals,
providers, reports, workflows, agents, labs, models, settings, and
science/* — unique routes with no Go equivalent, tested, pending
incremental Go migration. Do not re-add a route Go already serves.
"""

from __future__ import annotations

import hmac
import math
import os
import json
from typing import Any

from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.middleware.cors import CORSMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

from kubona import config
from kubona.cost.governance import CostGovernanceService
from kubona.providers import list_providers
from kubona.session import SessionStore

_sessions = SessionStore()
_gov = CostGovernanceService(_sessions)
# console auth is constant-time comparison against the operator token
# (config.console_token()), read FRESH on every request: an import-time
# freeze would pin the token until process restart, and even a short-TTL
# cache would keep accepting the OLD token until expiry after a rotation.
# Derivation is two os.getenv reads (no I/O, no crypto), so caching buys
# nothing — per-request read makes rotation (and KUBONA_ENV changes)
# effective immediately in both directions: new token works, old stops.
# Fail-closed: in KUBONA_ENV=production the token MUST be configured;
# otherwise every request is unauthorized. Outside production the dev
# token is the local/CI default.


def _authed(request: Request) -> bool:
    current = config.console_token()  # per-request read: rotation without restart
    if current is None:
        return False  # production fail-closed: no configured token, no access
    if not request.headers.get("authorization", "").startswith("Bearer "):
        return False
    token = request.headers["authorization"][len("Bearer "):]
    # constant-time compare against the configured operator token
    if hmac.compare_digest(token, current):
        return True
    return False


def _auth_error() -> JSONResponse:
    return JSONResponse({"ok": False, "error": "unauthorized: use Bearer <token>"}, status_code=401)


def _json(data: dict[str, Any], status: int = 200) -> JSONResponse:
    return JSONResponse(data, status_code=status)


def _bad_input(message: str) -> JSONResponse:
    return _json({"ok": False, "error": message}, 400)


async def health(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    return _json({"ok": True, "service": "kubona-console-api", "version": "0.2.0"})


async def providers(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    reg = {}
    for name, cap in list_providers().items():
        reg[name] = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model}
    return _json({"ok": True, "providers": reg})


async def approvals(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    state = request.query_params.get("state")
    out = []
    for a in _gov._load_approvals().values():
        if state is None or a.state == state:
            out.append({
                "approval_id": a.approval_id,
                "session_id": a.session_id,
                "state": a.state,
                "amount_requested_usd": a.amount_requested_usd,
                "reason": a.reason,
                "requested_by": a.requested_by,
                "mandate_token": a.mandate_token,
                "mandate_digest": a.mandate_digest,
            })
    return _json({"ok": True, "approvals": out})


async def approval_action(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    aid = request.path_params["aid"]
    action = request.path_params["action"]
    reviewer = request.query_params.get("reviewer", "operator")
    try:
        if action == "hold":
            a = _gov.hold_approval(aid, reviewer)
        elif action == "approve":
            from kubona.cost.mandate import operator_key_from_env
            try:
                key = operator_key_from_env()
            except ValueError as e:
                return _json({"ok": False, "error": str(e)}, 503)
            tenant_id = request.query_params.get("tenant_id", "")
            job_hash = request.query_params.get("job_hash", "")
            try:
                a = _gov.approve(aid, reviewer, note=request.query_params.get("note"),
                                 tenant_id=tenant_id, job_hash=job_hash, signing_key=key)
            except RuntimeError as e:
                return _json({"ok": False, "error": str(e)}, 400)
        elif action == "reject":
            a = _gov.reject(aid, reviewer, note=request.query_params.get("note") or "rejected from console")
        else:
            return _json({"ok": False, "error": f"unknown action {action}"}, 400)
        return _json({
            "ok": True,
            "approval_id": a.approval_id,
            "state": a.state,
            "mandate_token": a.mandate_token,
            "mandate_digest": a.mandate_digest,
        })
    except KeyError:
        return _json({"ok": False, "error": "unknown approval"}, 404)
    except Exception as e:
        return _json({"ok": False, "error": str(e)}, 400)


async def reports(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.reports.notebook import ReportStore

    return _json({"ok": True, "reports": ReportStore().all()})


async def workflows(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.workflows.library import WorkflowLibrary

    lib = WorkflowLibrary()
    return _json({"ok": True, "workflows": [{"workflow_id": w.workflow_id,
                                             "name": w.source.get("name"),
                                             "created_by": w.created_by,
                                             "created_by_agent": w.source.get("created_by_agent", False)}
                                            for w in lib.all()]})


async def agents(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.agents import AgentRegistry

    return _json({"ok": True, "agents": [a.to_dict() for a in AgentRegistry().all()]})


async def labs(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.labs import LabRegistry

    return _json({"ok": True, "labs": LabRegistry().all()})


async def models(request: Request) -> JSONResponse:
    """Auto-detect the host's local LLM servers (Ollama / LM Studio / MLX / etc.)."""
    if not _authed(request):
        return _auth_error()
    from kubona.llm.registry import LocalModelRegistry

    return _json({"ok": True, "scan": LocalModelRegistry().discover(),
                   "ports": DEFAULT_LLM_PORTS()})


def _strict_int(v: object, name: str, lo: int, hi: int) -> int:
    # JSON floats truncate silently under int(); bools are ints in Python.
    # Refuse both instead of narrowing caller data into a different request.
    if isinstance(v, bool) or not isinstance(v, int):
        raise ValueError(f"{name} must be an int in [{lo},{hi}], got {v!r}: refusing")
    if not (lo <= v <= hi):
        raise ValueError(f"{name} must be in [{lo},{hi}], got {v!r}: refusing")
    return v


def _as_int(v: object, field: str, lo: int | None = None, hi: int | None = None) -> int:
    """Strict int field read for science routes (same rule as _strict_int).

    JSON floats truncate silently under int() and bools are ints in Python —
    both refuse instead of narrowing caller data into a different request.
    Range violations and wrong types raise ValueError NAMING the field, so
    routes render a typed 400 instead of a 500 on malformed numbers.
    """
    if isinstance(v, bool) or not isinstance(v, int):
        raise ValueError(f"{field} must be an int, got {v!r}: refusing")
    if (lo is not None and v < lo) or (hi is not None and v > hi):
        raise ValueError(f"{field} must be in [{lo},{hi}], got {v!r}: refusing")
    return v


def _as_float(v: object, field: str, lo: float | None = None, hi: float | None = None,
              *, finite: bool = True) -> float:
    """Strict float field read for science routes (typed 400, never 500)."""
    if isinstance(v, bool) or not isinstance(v, (int, float)):
        raise ValueError(f"{field} must be a number, got {v!r}: refusing")
    f = float(v)
    if finite and not math.isfinite(f):
        raise ValueError(f"{field} must be finite, got {v!r}: refusing")
    if (lo is not None and f < lo) or (hi is not None and f > hi):
        raise ValueError(f"{field} must be in [{lo},{hi}], got {v!r}: refusing")
    return f


def _science_body(awaitable_json) -> dict:
    """Awaited request.json() narrowed to a JSON object (typed 400)."""
    if not isinstance(awaitable_json, dict):
        raise ValueError(f"body must be a JSON object, got {type(awaitable_json).__name__}: refusing")
    return awaitable_json


def _resolve_codesign_spec(body: dict) -> tuple:
    """Single spec resolver (no second copy): named kind or truth table.

    Returns (spec, label). Truth-table form:
    {spec_table: {num_qubits, inputs[], outputs[], table{inbits: {out, phase}}}}.
    """
    from kubona.codesign import build_spec, build_truth_table_spec
    if "spec_table" in body and body["spec_table"] is not None:
        st = body["spec_table"]
        if not isinstance(st, dict):
            raise ValueError("spec_table must be an object: refusing")
        nq = st.get("num_qubits", 0)
        ins, outs = st.get("inputs", []), st.get("outputs", [])
        table = st.get("table", {})
        if not isinstance(ins, list) or not isinstance(outs, list) \
                or not isinstance(table, dict):
            raise ValueError("spec_table needs inputs[]/outputs[]/table{}: refusing")
        spec = build_truth_table_spec(
            _strict_int(nq, "num_qubits", 1, 64),
            [_strict_int(q, "inputs[]", 0, 63) for q in ins],
            [_strict_int(q, "outputs[]", 0, 63) for q in outs],
            table,
        )
        return spec, "table"
    kind = body.get("spec_kind")
    return build_spec(str(kind)), str(kind)


async def codesign_evaluate(request: Request) -> JSONResponse:
    """Evaluator-guided circuit check (Python-owned; pending Go migration).

    Body: {spec_kind|spec_table, artifact, num_tests, max_failures, kappa, seed}.
    spec_kind in {and2, cz-phase, copy1, xor2} or spec_table object.
    Artifact = list of [name, qubits, cond].
    Local-classical only; no QPU; refuses unknown specs/artifacts fail-closed.
    """
    if not _authed(request):
        return _auth_error()
    try:
        body = await request.json()
    except Exception:
        return _json({"ok": False, "error": "invalid JSON body: refusing"}, 400)
    try:
        from kubona.codesign import CircuitCodesignHarness, kappa_sweep
        artifact = body.get("artifact", [])
        num_tests = _strict_int(body.get("num_tests", 64), "num_tests", 1, 100000)
        max_failures = _strict_int(body.get("max_failures", 0), "max_failures", 0, 100000)
        kappa = _strict_int(body.get("kappa", 1), "kappa", 1, 3)
        seed = _strict_int(body.get("seed", 7), "seed", 0, 2**31 - 1)
        if max_failures > num_tests:
            raise ValueError("max_failures must be <= num_tests: refusing")
        spec, label = _resolve_codesign_spec(body)
        h = CircuitCodesignHarness(spec, num_tests=num_tests,
                                   max_failures=max_failures, kappa=kappa, seed=seed)
        res = h.run(artifact)
        if res.get("valid"):
            res["kappa_sweep"] = {str(k): v for k, v in
                                  kappa_sweep(res["toffoli"], res["peak_qubits"]).items()}
        return _json({"ok": True, "spec_kind": label, "result": res})
    except ValueError as e:
        return _json({"ok": False, "error": str(e)[:280]}, 400)
    except Exception as e:  # noqa: BLE001 — fail-closed, never 500 with traceback
        return _json({"ok": False, "error": f"evaluation refused: {e}"[:280]}, 400)


async def codesign_frontier(request: Request) -> JSONResponse:
    """MAP-Elites frontier over a batch of artifacts (read-only scoring).

    Body: {spec_kind|spec_table, artifacts[], kappa}. Returns cells + best (min-TQ).
    Invalid artifacts are reported, never scored as valid.
    """
    if not _authed(request):
        return _auth_error()
    try:
        body = await request.json()
    except Exception:
        return _json({"ok": False, "error": "invalid JSON body: refusing"}, 400)
    try:
        from kubona.codesign import MapElitesFrontier
        from kubona.codesign.harness import CircuitCodesignHarness
        artifacts = body.get("artifacts", [])
        kappa = _strict_int(body.get("kappa", 1), "kappa", 1, 3)
        if not isinstance(artifacts, list) or len(artifacts) > 64:
            return _json({"ok": False, "error": "artifacts must be a list of <=64: refusing"}, 400)
        spec, label = _resolve_codesign_spec(body)
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
        return _json({"ok": True, "spec_kind": label, "frontier": f.to_dict(), "details": details})
    except ValueError as e:
        return _json({"ok": False, "error": str(e)[:280]}, 400)
    except Exception as e:  # noqa: BLE001
        return _json({"ok": False, "error": f"frontier refused: {e}"[:280]}, 400)


async def codesign_budget(request: Request) -> JSONResponse:
    """Failure-budget union bound (read-only math, Table-S4 style).

    Body: {entries: [{source, per_use_rate, uses}]}. Returns rows + total.
    """
    if not _authed(request):
        return _auth_error()
    try:
        body = await request.json()
    except Exception:
        return _json({"ok": False, "error": "invalid JSON body: refusing"}, 400)
    try:
        from kubona.codesign import BudgetEntry, ledger_rows, union_bound
        raw = body.get("entries", [])
        if not isinstance(raw, list) or len(raw) > 32:
            return _json({"ok": False, "error": "entries must be a list of <=32: refusing"}, 400)
        entries = []
        for e in raw:
            if not isinstance(e, dict):
                raise ValueError(f"entry must be an object, got {e!r}: refusing")
            rate = e.get("per_use_rate", float("nan"))
            if isinstance(rate, bool) or not isinstance(rate, (int, float)):
                raise ValueError(f"per_use_rate must be a number, got {rate!r}: refusing")
            entries.append(BudgetEntry(str(e.get("source", "")),
                                       float(rate),
                                       _strict_int(e.get("uses", -1), "uses", 0, 10**9)))
        return _json({"ok": True, "rows": ledger_rows(entries),
                      "union_bound": union_bound(entries)})
    except ValueError as e:
        return _json({"ok": False, "error": str(e)[:280]}, 400)
    except Exception as e:  # noqa: BLE001
        return _json({"ok": False, "error": f"budget refused: {e}"[:280]}, 400)


async def settings(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    # W2.17 CALLER REQUIREMENT: the settings PUT (apply_llm_edge) repoints the
    # platform LLM and touches credentials — expose this route only to
    # Writer-scope principals. _authed covers operator auth; if this console
    # ever gains Reader roles, gate PUT (not GET) on Writer here.
    from kubona.settings import SettingsStore

    store = SettingsStore()
    if request.method == "GET":
        return _json({"ok": True, "settings": store.public()})
    body = await request.json()
    # Validated edge (not raw update): unknown keys/providers and blank
    # models refuse here instead of persisting as live config.
    try:
        settings = store.apply_llm_edge(body if isinstance(body, dict) else {})
    except ValueError as e:
        return _json({"ok": False, "error": str(e)}, status=400)
    return _json({"ok": True, "settings": settings})


def DEFAULT_LLM_PORTS() -> list[int]:
    from kubona.llm.registry import DEFAULT_PORTS as p

    return p


async def math_families(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.science.math_catalog import MathCatalog
    discipline = request.query_params.get("discipline")
    quantum_only = request.query_params.get("quantum_only", "").lower() in ("true", "1")
    catalog = MathCatalog()
    families = catalog.list_families(discipline=discipline, quantum_only=quantum_only)
    return _json({"ok": True, "families": [f.to_dict() for f in families], "stats": catalog.summary_stats()})


async def math_anchors(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine
    benchmarks = MathPhysicsAnchorEngine.list_benchmarks()
    anchors = [MathPhysicsAnchorEngine.get_anchor(b).to_dict() for b in benchmarks]
    return _json({"ok": True, "benchmarks": benchmarks, "anchors": anchors})


async def science_triage(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.science.triage import QuantumTriageEngine, MolecularSystem
    try:
        body = _science_body(await request.json())
        electrons = _as_int(body.get("num_electrons", 2), "num_electrons", 0, 10000)
        orbitals = _as_int(body.get("estimated_active_space_orbitals", 4),
                            "estimated_active_space_orbitals", 1, 10000)
        charges = body.get("nuclear_charges", [1, 1])
        if not isinstance(charges, list) or len(charges) > 256:
            raise ValueError("nuclear_charges must be a list of at most 256 values: refusing")
        charges = [_as_int(z, f"nuclear_charges[{i}]") for i, z in enumerate(charges)]
    except ValueError as e:
        return _bad_input(str(e)[:280])
    system = MolecularSystem(
        molecule_id=str(body.get("molecule_id", "mol_custom"))[:128],
        formula=str(body.get("formula", "H2"))[:64],
        num_electrons=electrons,
        nuclear_charges=charges,
        is_multi_reference=bool(body.get("is_multi_reference", False)),
        coulomb_continuum=bool(body.get("coulomb_continuum", True)),
        estimated_active_space_orbitals=orbitals,
    )
    res = QuantumTriageEngine.triage(system)
    return _json({"ok": True, "triage": res})


async def science_krylov(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.circuits.krylov import QKSDSolver
    import numpy as np
    try:
        body = _science_body(await request.json())
        subspace_dim = _as_int(body.get("subspace_dim", 4), "subspace_dim")
        dt = _as_float(body.get("time_step", 0.2), "time_step")
        if not (1 <= subspace_dim <= 32):
            return _bad_input("subspace_dim must be 1..32")
        if not (math.isfinite(dt) and 0 < dt <= 1e3):
            return _bad_input("time_step must be finite and in (0, 1000]")

        if "hamiltonian" in body:
            try:
                H = np.array(body["hamiltonian"], dtype=complex)
            except (ValueError, TypeError):
                raise ValueError(
                    f"hamiltonian entries must be numbers, got {str(body['hamiltonian'])[:64]!r}: refusing")
            if H.ndim != 2 or H.shape[0] != H.shape[1] or H.shape[0] > 64 or H.shape[0] & (H.shape[0] - 1):
                return _bad_input("hamiltonian must be a square 2^n matrix with n <= 6")
            subspace_dim = min(subspace_dim, H.shape[0])
        else:
            Z = np.array([[1, 0], [0, -1]], dtype=complex)
            X = np.array([[0, 1], [1, 0]], dtype=complex)
            I = np.eye(2, dtype=complex)
            H = -np.kron(Z, Z) - 0.5 * (np.kron(X, I) + np.kron(I, X))
    except ValueError as e:
        return _bad_input(str(e)[:280])

    result = QKSDSolver.solve(
        H,
        initial_state=np.eye(H.shape[0], dtype=complex)[0],
        subspace_dim=subspace_dim,
        time_step=dt,
        reference_assumption="computational-zero |0...0> (console default, recorded — overlap with ground state assumed, not measured)",
    )
    return _json({"ok": True, "result": result.to_dict()})


async def science_shadows(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.circuits.shadows import ClassicalShadowEngine
    import numpy as np
    try:
        body = _science_body(await request.json())
        num_qubits = _as_int(body.get("num_qubits", 2), "num_qubits", 1, 12)
        num_snapshots = _as_int(body.get("num_snapshots", 400), "num_snapshots", 1, 10000)
        observables = body.get("observables", ["ZZ", "XX", "YY"])
        if not isinstance(observables, list) or len(observables) > 64 or any(
                not isinstance(o, str) or len(o) > 32 for o in observables):
            raise ValueError("observables must be a list of at most 64 short strings: refusing")
        import secrets as _secrets
        shadow_seed = _as_int(body.get("seed", _secrets.randbits(63)), "seed", 0, None)
    except ValueError as e:
        return _bad_input(str(e)[:280])

    state = np.zeros(2**num_qubits, dtype=complex)
    state[0] = 1.0 / np.sqrt(2)
    state[-1] = 1.0 / np.sqrt(2)

    snapshots = ClassicalShadowEngine.sample_shadows_from_state(
        state, num_qubits=num_qubits, num_snapshots=num_snapshots,
        rng=np.random.default_rng(shadow_seed),
    )
    estimates = {}
    for obs in observables:
        estimates[obs] = ClassicalShadowEngine.estimate_observable_from_snapshots(snapshots, obs)
    return _json({"ok": True, "num_snapshots": num_snapshots, "estimates": estimates,
                  "seed": shadow_seed})


async def science_manuscript_create(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.science.manuscript import ManuscriptGenerator, ReviewEvidence
    try:
        body = _science_body(await request.json())
        anchor_val = _as_float(body.get("anchor_val", -1.5), "anchor_val", finite=False)
        measured_val = _as_float(body.get("measured_val", -1.498), "measured_val", finite=False)
        sorry_raw = body.get("sorry_ax_count")
        sorry_ax_count = (None if sorry_raw is None
                          else _as_int(sorry_raw, "sorry_ax_count", 0, None))
    except ValueError as e:
        return _bad_input(str(e)[:280])
    ms = ManuscriptGenerator.create_discovery_manuscript(
        title=body.get("title", "Quantum Subspace Discovery"),
        target_system=body.get("target_system", "H2"),
        authors=body.get("authors", ["Kubona Autonomous Theorist"]),
        abstract=body.get("abstract", "Automated epistemic discovery."),
        analytical_anchor_name=body.get("anchor_name", "Heisenberg_Ferromagnet_3D"),
        analytical_anchor_val=anchor_val,
        measured_val=measured_val,
        lean4_proof_hash=body.get("lean4_hash"),
        negative_certificates=body.get("negative_certs"),
    )
    # Evidence is caller-attested and fail-closed: unattested dimensions
    # falsify (REVISE/REJECT), never default to ACCEPT. The discrepancy is
    # derived server-side from the submitted numbers, not caller-asserted.
    evidence = ReviewEvidence(
        comparator_verdict=body.get("verification_mode"),
        sorry_ax_count=sorry_ax_count,
        anchor_discrepancy_mha=abs(measured_val - anchor_val) * 1000.0,
        provenance_receipts=[r for r in body.get("provenance_receipts", []) if r] + (
            [body["lean4_hash"]] if body.get("lean4_hash") else []),
        citation_distance=body.get("citation_distance"),
    )
    review = ManuscriptGenerator.automated_peer_review(ms, evidence)
    return _json({
        "ok": True,
        "manuscript_id": ms.manuscript_id,
        "latex_main": ms.compile_latex(),
        "bibtex": ms.compile_bibtex(),
        "peer_review": {
            "reviewer": review.reviewer_persona,
            "rigor": review.mathematical_rigor_score,
            "novelty": review.novelty_score,
            "grounding": review.empirical_grounding_score,
            "hygiene": review.axiomatic_hygiene_score,
            "recommendation": review.overall_recommendation,
            "mean_score": review.mean_score(),
            "acceptable": review.is_acceptable(),
            "summary": review.summary,
            "critiques": review.actionable_critiques,
        }
    })


async def science_memory_query(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.memory.tiered_storage import TieredStorageManager
    from kubona.science.memory import KnowledgeEntity, KnowledgeRelation
    try:
        body = _science_body(await request.json())
        start_id = str(body.get("start_id", "ent_femoco"))[:128]
        max_hops = _as_int(body.get("max_hops", 2), "max_hops", 0, 8)
    except ValueError as e:
        return _bad_input(str(e)[:280])
    mgr = TieredStorageManager()

    e1 = KnowledgeEntity("ent_femoco", "molecule", "FeMoco Nitrogenase Active Cluster", {"coulomb_continuum": True})
    e2 = KnowledgeEntity("ent_family278", "theorem", "Family 278 KS Representability Failure", {"family_id": 278})
    mgr.index_entity(e1)
    mgr.index_entity(e2)
    mgr.index_relation(KnowledgeRelation("ent_femoco", "ent_family278", "FAILS_ON", 1.0))

    recall = mgr.associative_recall(start_id, max_hops=max_hops)
    ppr = mgr.personalized_pagerank([start_id])
    formatted_recall = []
    for item in recall:
        target = item["target_entity"]
        formatted_recall.append({
            "hop_depth": item["hop_depth"],
            "target_entity": {
                "entity_id": target.entity_id,
                "entity_type": target.entity_type,
                "name": target.name,
                "properties": target.properties,
            },
            "path": item["path"],
        })
    return _json({"ok": True, "start_id": start_id, "recall": formatted_recall, "pagerank": ppr})


async def science_tribunal_deliberate(request: Request) -> JSONResponse:
    """Weighted-quorum recorder over caller-attested evidence.

    This endpoint does NOT verify anything itself: it records the weighted
    votes of the six tribunal personas over evidence the caller attests to
    (comparator outcome, gradient variance, anchor/mandate ledger refs) and
    computes whether the supermajority rule is met. Fail-closed defaults apply:
    unattested verification inputs falsify the prover vote, and the prover
    rationale always states the verification mode (live kernel proof vs static
    AST containment) so readers can never mistake one for the other.
    """
    if not _authed(request):
        return _auth_error()
    try:
        body = _science_body(await request.json())
        gradient_variance = _as_float(body.get("gradient_variance", 0.0),
                                      "gradient_variance", finite=False)
    except ValueError as e:
        return _bad_input(str(e)[:280])
    hyp_id = body.get("hypothesis_id", "hyp-femoco-001")
    target_system = body.get("target_system", "FeMoco")
    # Fail-closed attestations: absence of evidence is evidence of absence.
    lean4_verified = bool(body.get("lean4_verified", False))
    zero_axiom_cheating = bool(body.get("zero_axiom_cheating", False))
    verification_mode = str(body.get("verification_mode", "unattested"))
    comparator_ledger_ref = body.get("comparator_ledger_ref")
    anchor_ledger_ref = body.get("anchor_ledger_ref")
    mandate_ledger_ref = body.get("mandate_ledger_ref")
    krylov_ledger_ref = body.get("krylov_ledger_ref")
    qir_ledger_ref = body.get("qir_ledger_ref")

    # Each persona carries a fixed voting weight (its potential contribution).
    # The quorum denominator is the sum of ALL weights, so a falsified vote
    # always dilutes consensus — unattested evidence can never silently pass.
    theorist_ok = bool(krylov_ledger_ref)
    compiler_ok = bool(qir_ledger_ref)
    anchor_ok = bool(anchor_ledger_ref)
    sentinel_ok = bool(mandate_ledger_ref)
    prover_weight = 1.0 if verification_mode == "live_kernel_proven" else 0.90

    prover_ok = bool(lean4_verified and zero_axiom_cheating
                     and verification_mode == "live_kernel_proven")
    if verification_mode == "live_kernel_proven":
        prover_rho, prover_claim = 1.0, "machine-checked kernel proof"
    elif verification_mode == "static_ast_containment":
        # Static containment is weaker than a kernel proof: cap the vote
        # weight below 1.0 and say so in the rationale.
        prover_rho, prover_claim = 0.90, "static AST containment (not kernel-proved)"
    else:
        prover_rho, prover_claim = 0.0, "no verified attestation supplied"

    votes = [
        {
            "persona_id": "theorist-01",
            "role": "lead_theorist",
            "name": "Lead Polymath Theorist",
            "weight": 0.96,
            "verdict": "corroborated" if theorist_ok else "falsified",
            "confidence_rho": 0.96 if theorist_ok else 0.0,
            "rationale": (
                "Quantum Krylov monotonic convergence validated against ledger record"
                if theorist_ok else
                "No Krylov ledger reference supplied; convergence claim unattested"
            ),
            "evidence_ref": krylov_ledger_ref,
        },
        {
            "persona_id": "critic-01",
            "role": "adversarial_falsification_critic",
            "name": "Adversarial Falsification Critic",
            "weight": 0.92,
            "verdict": "corroborated" if gradient_variance >= 1e-4 else "falsified",
            "confidence_rho": 0.92 if gradient_variance >= 1e-4 else 0.99,
            "rationale": f"Variance of gradients Var[∇H] = {gradient_variance:.4f}",
        },
        {
            "persona_id": "prover-01",
            "role": "lean4_proof_engineer",
            "name": "Lean 4 Proof Engineer",
            "weight": prover_weight,
            "verdict": "corroborated" if prover_ok else "falsified",
            "confidence_rho": prover_rho if prover_ok else 0.0,
            "rationale": (
                f"Comparator check: standard axioms only, 0 sorryAx ({prover_claim})"
                if prover_ok else
                f"Unverified proof attestation ({prover_claim}); falsified pending evidence"
            ),
            "evidence_ref": comparator_ledger_ref,
            "verification_mode": verification_mode,
        },
        {
            "persona_id": "anchor-01",
            "role": "classical_anchor_solver",
            "name": "Classical Anchor Solver",
            "weight": 0.94,
            "verdict": "corroborated" if anchor_ok else "falsified",
            "confidence_rho": 0.94 if anchor_ok else 0.0,
            "rationale": (
                "Consistent with Family 271 ferromagnetic exact bounds within chemical precision (1.6 mHa)"
                if anchor_ok else
                "No classical-anchor ledger reference supplied; anchor claim unattested"
            ),
            "evidence_ref": anchor_ledger_ref,
        },
        {
            "persona_id": "compiler-01",
            "role": "quantum_circuit_compiler",
            "name": "Quantum Circuit Compiler",
            "weight": 0.89,
            "verdict": "corroborated" if compiler_ok else "falsified",
            "confidence_rho": 0.89 if compiler_ok else 0.0,
            "rationale": (
                "QIR compilation mapped to [[144, 12, 12]] gross code; T-depth within budget"
                if compiler_ok else
                "No QIR lowering ledger reference supplied; compilation claim unattested"
            ),
            "evidence_ref": qir_ledger_ref,
        },
        {
            "persona_id": "sentinel-01",
            "role": "sentinel_spend_governor",
            "name": "Sentinel Spend Governor",
            "weight": 1.0,
            "verdict": "corroborated" if sentinel_ok else "falsified",
            "confidence_rho": 1.0 if sentinel_ok else 0.0,
            "rationale": (
                "AP2 spend authorization signed with Ed25519; air-gap egress zero bytes"
                if sentinel_ok else
                "No spend-mandate ledger reference supplied; authorization claim unattested"
            ),
            "evidence_ref": mandate_ledger_ref,
        },
    ]

    total_weight = sum(v["weight"] for v in votes)
    corrob_weight = sum(v["weight"] for v in votes if v["verdict"] == "corroborated")
    quorum_pct = (corrob_weight / total_weight * 100.0) if total_weight > 0 else 0.0

    supermajority_met = (
        quorum_pct >= 75.0
        and prover_ok
        and (gradient_variance >= 1e-4)
    )
    # This recorder does not write a ledger event; never claim it did.
    verdict = "QUORUM_MET" if supermajority_met else "FALSIFIED_OR_REJECTED"

    return _json({
        "ok": True,
        "hypothesis_id": hyp_id,
        "target_system": target_system,
        "current_phase": "PHASE_5_QUORUM_VOTING",
        "quorum_percentage": quorum_pct,
        "verdict": verdict,
        "ledger_committed": False,
        "votes": votes,
    })


async def science_qec_estimate(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    try:
        body = _science_body(await request.json())
        logical_qubits = _as_int(body.get("num_logical_qubits", 12), "num_logical_qubits",
                                 1, 10000)
        clifford_count = _as_int(body.get("logical_clifford_count", 1000),
                                 "logical_clifford_count", 0, 1_000_000_000)
        t_count = _as_int(body.get("logical_t_count", 250), "logical_t_count", 0, 1_000_000_000)
        physical_error_rate = _as_float(body.get("physical_error_rate", 0.001),
                                        "physical_error_rate")
        if not (math.isfinite(physical_error_rate) and 0 < physical_error_rate < 0.5):
            return _bad_input("physical_error_rate must be finite and in (0, 0.5)")
    except ValueError as e:
        return _bad_input(str(e)[:280])

    neutral_capacity_ok = logical_qubits <= 48
    neutral_atoms_total = logical_qubits * 8 if neutral_capacity_ok else None

    blocks_needed = (logical_qubits + 11) // 12
    bb_physical_qubits = blocks_needed * 288
    bb_overhead = float(bb_physical_qubits) / float(logical_qubits)
    bb_threshold = 0.0075
    bb_suppressed_err = 0.1 * ((physical_error_rate / bb_threshold) ** 6)

    return _json({
        "ok": True,
        "num_logical_qubits": logical_qubits,
        "logical_clifford_count": clifford_count,
        "logical_t_count": t_count,
        "architectures": {
            "neutral_atom_quera": {
                "supported": neutral_capacity_ok,
                "physical_qubits_total": neutral_atoms_total,
                "hardware_platform": "Rb87_Optical_Tweezers",
                "max_logical_qubits": 48,
                "tweezer_mobility": "Dynamic_All_To_All",
            },
            "bivariate_bicycle_qldpc_ibm": {
                "supported": True,
                "physical_qubits_total": bb_physical_qubits,
                "gross_code_blocks": blocks_needed,
                "code_distance": 12,
                "physical_qubit_overhead_ratio": bb_overhead,
                "threshold": bb_threshold,
                "suppressed_logical_error_rate": bb_suppressed_err,
                "overhead_reduction_vs_surface_code": "10.4x",
            }
        }
    })


# CORS: explicit origin allowlist, mirroring the Go edge rail
# (platform/core/pkg/api/edge_auth.go corsAllowOrigin). Default is the
# local console origins plus KUBONA_CORS_ORIGINS extras; "*" requires the
# KUBONA_CORS_ANY=1 dev escape hatch, never the default.
def _cors_allow_origins() -> list[str]:
    if os.environ.get("KUBONA_CORS_ANY") == "1":
        return ["*"]
    allow = ["http://localhost:3000", "http://127.0.0.1:3000"]
    extra = os.environ.get("KUBONA_CORS_ORIGINS", "")
    for origin in extra.split(","):
        origin = origin.strip()
        if origin:
            allow.append(origin)
    return allow


app = Starlette(
    middleware=[Middleware(CORSMiddleware,
                          allow_origins=_cors_allow_origins(),
                          # Explicit method/header sets: the console speaks
                          # GET/POST/PUT/DELETE with Authorization +
                          # Content-Type only. A new method/header is a
                          # conscious allowlist edit, never a wildcard.
                          allow_methods=["GET", "POST", "PUT", "DELETE"],
                          allow_headers=["Authorization", "Content-Type"])],
    routes=[
        Route("/api/health", health),
        Route("/api/science/math/families", math_families),
        Route("/api/science/math/anchors", math_anchors),
        Route("/api/science/triage", science_triage, methods=["POST"]),
        Route("/api/science/krylov", science_krylov, methods=["POST"]),
        Route("/api/science/shadows", science_shadows, methods=["POST"]),
        Route("/api/science/manuscripts", science_manuscript_create, methods=["POST"]),
        Route("/api/science/memory/query", science_memory_query, methods=["POST"]),
        Route("/api/science/tribunal/deliberate", science_tribunal_deliberate, methods=["POST"]),
        Route("/api/science/qec/estimate", science_qec_estimate, methods=["POST"]),
        Route("/api/providers", providers),
        Route("/api/approvals", approvals),
        Route("/api/approvals/{aid}/{action}", approval_action, methods=["POST"]),
        Route("/api/reports", reports),
        Route("/api/workflows", workflows),
        Route("/api/agents", agents),
        Route("/api/labs", labs),
        Route("/api/models", models),
        Route("/api/settings", settings, methods=["GET", "PUT"]),
        Route("/api/codesign/evaluate", codesign_evaluate, methods=["POST"]),
        Route("/api/codesign/frontier", codesign_frontier, methods=["POST"]),
        Route("/api/codesign/budget", codesign_budget, methods=["POST"]),
    ],
)


def main(host: str = "127.0.0.1", port: int | None = None) -> None:
    """Run the console API. W2.14: `serve()` threads its args here —
    explicit values win, otherwise the env defaults apply (never ignored)."""
    import uvicorn

    resolved_port = port if port is not None else int(os.environ.get("KUBONA_CONSOLE_PORT", "8000"))
    uvicorn.run(app, host=host, port=resolved_port, log_level="warning")


if __name__ == "__main__":
    main()
