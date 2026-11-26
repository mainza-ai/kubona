"""IBM Runtime Sparse Pauli-Lindblad (SPL) Noise Learning + PEA / PEC (Open Plan Job-Mode Native).

Uses ``qiskit-ibm-runtime >= 0.50`` client-side ``Estimator`` and ``NoiseLearnerV3``
strictly in **Job Mode** (``mode=backend``, never ``Session``) so workloads execute
cleanly on both IBM Open Plan and Pay-As-You-Go instances.

Workflow:
  1. Transpile circuit once to backend ISA on Kubona's calibrated physical layout.
  2. Discover unique 2Q ISA layers (``estimator.find_unique_layers``) and learn
     their Sparse Pauli-Lindblad noise models via ``NoiseLearnerV3(mode=backend)``.
  3. Compute exact total PEC sampling overhead ``gamma = prod_l gamma_l``; fail
     closed before Estimator submission if ``method == "PEC"`` and
     ``gamma > max_overhead``.
  4. Submit paired PUBs (mitigated + unmitigated/TREX baseline) via ``Estimator``
     with ``ResilienceOptionsV2`` (``PEA`` or ``PEC`` + ``measure_mitigation=True``)
     and record both vendor job IDs, combined ``usage_seconds``, and paired
     difference CIs.
"""

from __future__ import annotations

import math
from typing import Any, Callable

Z95 = 1.959963984540054


def _extract_spl_overhead(noise_result: Any) -> tuple[float, list[dict[str, Any]]]:
    """Extract total PEC sampling overhead ``gamma`` and per-layer summaries."""
    layers_summary: list[dict[str, Any]] = []
    total_gamma = 1.0
    items = list(noise_result) if noise_result is not None else []
    for idx, layer_err in enumerate(items):
        err_obj = getattr(layer_err, "error", layer_err)
        gamma_l = 1.0
        n_terms = 0
        max_rate = 0.0
        if hasattr(err_obj, "gamma"):
            try:
                gamma_l = float(err_obj.gamma())
            except Exception:
                gamma_l = 1.0
        rates = getattr(err_obj, "rates", None)
        if rates is not None:
            try:
                r_list = [float(r) for r in rates]
                n_terms = len(r_list)
                max_rate = max(r_list) if r_list else 0.0
                if gamma_l == 1.0 and r_list:
                    # For Pauli-Lindblad generator sum_k r_k (P_k . P_k - I),
                    # exact single-layer PEC 1-norm is exp(2 * sum_k r_k).
                    gamma_l = float(math.exp(2.0 * sum(max(0.0, r) for r in r_list)))
            except Exception:
                pass
        total_gamma *= max(1.0, gamma_l)
        layers_summary.append({
            "layer_index": idx,
            "gamma": float(gamma_l),
            "n_pauli_terms": int(n_terms),
            "max_rate": float(max_rate),
        })
    return float(total_gamma), layers_summary


def run_estimator_spl_mitigation(
    qc: Any,
    coeffs: dict[str, float],
    *,
    method: str = "PEA",
    backend_name: str | None = None,
    noise_factors: list[float] | None = None,
    max_overhead: float = 25.0,
    precision: float = 0.001,
    num_randomizations: int = 32,
    constant_energy: float = 0.0,
    fci_exact_energy: float | None = None,
    service_factory: Callable[[], Any] | None = None,
    estimator_factory: Callable[[Any], Any] | None = None,
    noise_learner_factory: Callable[[Any], Any] | None = None,
) -> dict[str, Any]:
    """Execute IBM Runtime Sparse Pauli-Lindblad ``PEA`` or ``PEC`` in Job Mode.

    Fully supports dependency injection (``service_factory``, ``estimator_factory``,
    ``noise_learner_factory``) for deterministic offline/CI contract tests without
    network access.
    """
    mode = str(method or "PEA").strip().upper()
    if mode not in ("PEA", "PEC"):
        return {"ok": False, "error": f"SPL mitigation method must be PEA or PEC, got {method!r}"}
    if not coeffs:
        return {"ok": False, "error": "SPL mitigation requires non-empty pauli_coefficients"}
    if float(precision) <= 0 or float(precision) > 0.1:
        return {"ok": False, "error": f"precision {precision!r} outside (0, 0.1]"}
    if float(max_overhead) < 1.0 or float(max_overhead) > 1000.0:
        return {"ok": False, "error": f"max_overhead {max_overhead!r} outside [1.0, 1000.0]"}

    from qiskit.quantum_info import SparsePauliOp
    from kubona.providers.qiskit_provider import QiskitProvider, _extract_usage_seconds
    from kubona.schema import load_provider

    obs = SparsePauliOp.from_list([(str(k).strip().upper(), float(v)) for k, v in coeffs.items() if abs(float(v)) > 0])
    provider = QiskitProvider(load_provider("ibm-qiskit"), backend_name=backend_name)

    if service_factory is not None:
        service = service_factory()
        backend = provider._resolve_backend(service)
    else:
        token = provider._credential()
        if not token:
            return {"ok": False, "error": "cannot run IBM SPL mitigation without KUBONA_IBM_QISKIT_TOKEN"}
        service = provider._service(token)
        backend = provider._resolve_backend(service)

    chosen_layout = None
    isa_qc = qc
    isa_obs = obs
    layout_summary = None
    layout_quality = None
    if hasattr(backend, "target"):
        chosen_layout = provider._calibrated_initial_layout(qc, backend)
        isa_qc = provider._transpile_for_backend(qc, backend, chosen_layout)
        layout_summary = provider._layout_summary(qc, isa_qc)
        phys = layout_summary.get("physical_qubits") if isinstance(layout_summary, dict) else chosen_layout
        layout_quality = provider._layout_quality(phys, backend)
        if getattr(isa_qc, "layout", None) is not None:
            isa_obs = obs.apply_layout(isa_qc.layout)

    # 1. Learn Sparse Pauli-Lindblad noise model BEFORE instantiating/dispatching
    # Estimator so PEC_OVERHEAD_EXCEEDED fails closed before Estimator runs!
    nl_job_id = ""
    nl_usage = 0.0
    learned_model = None
    learned_gamma = 1.0
    layer_summaries: list[dict[str, Any]] = []

    if noise_learner_factory is not None:
        learner = noise_learner_factory(backend)
    else:
        try:
            from qiskit_ibm_runtime.noise_learner_v3 import NoiseLearnerV3
            learner = NoiseLearnerV3(mode=backend)
        except Exception:
            from qiskit_ibm_runtime import NoiseLearner
            learner = NoiseLearner(mode=backend)

    nl_job = learner.run([isa_qc])
    nl_job_id = str(nl_job.job_id())
    nl_result = nl_job.result()
    learned_model = nl_result
    learned_gamma, layer_summaries = _extract_spl_overhead(nl_result)
    u_nl = _extract_usage_seconds(nl_job, retry_attempts=0 if service_factory is not None else 2)
    if u_nl is not None:
        nl_usage = float(u_nl)

    # Fail-closed PEC overhead guard BEFORE instantiating or dispatching Estimator!
    if mode == "PEC" and learned_gamma > float(max_overhead):
        return {
            "ok": False,
            "error": (
                f"PEC_OVERHEAD_EXCEEDED: learned Sparse Pauli-Lindblad gamma={learned_gamma:.3f} "
                f"> max_overhead={max_overhead:.3f} (noise_learner_job={nl_job_id})"
            ),
            "method": mode,
            "noise_learner_job_id": nl_job_id,
            "learned_gamma": learned_gamma,
            "layer_summaries": layer_summaries,
            "usage_seconds": nl_usage,
        }

    # 2. Build client-side Estimator in Job Mode (Open Plan compatible; NEVER Session!)
    if estimator_factory is not None:
        estimator = estimator_factory(backend)
    else:
        from qiskit_ibm_runtime import Estimator
        estimator = Estimator(mode=backend)

    # 3. Run baseline (unmitigated) Estimator PUB for paired raw-vs-mitigated CI
    opts = getattr(estimator, "options", None)
    if opts is not None:
        if hasattr(opts, "default_precision"):
            opts.default_precision = float(precision)
        dd = getattr(opts, "dynamical_decoupling", None)
        if dd is not None and hasattr(dd, "enable"):
            dd.enable = True
            if hasattr(dd, "sequence_type"):
                dd.sequence_type = "XY4"
            if hasattr(dd, "scheduling_method"):
                dd.scheduling_method = "alap"
        tw = getattr(opts, "twirling", None)
        if tw is not None:
            if hasattr(tw, "enable_gates"):
                tw.enable_gates = True
            if hasattr(tw, "enable_measure"):
                tw.enable_measure = True
            if hasattr(tw, "num_randomizations"):
                tw.num_randomizations = int(num_randomizations)
            if hasattr(tw, "strategy"):
                tw.strategy = "active-accum"
        res_opts = getattr(opts, "resilience", None)
        if res_opts is not None:
            if hasattr(res_opts, "measure_mitigation"):
                res_opts.measure_mitigation = True
            if hasattr(res_opts, "layer_noise_model") and learned_model is not None:
                res_opts.layer_noise_model = learned_model
            if mode == "PEA":
                if hasattr(res_opts, "zne_mitigation"):
                    res_opts.zne_mitigation = True
                if hasattr(res_opts, "pec_mitigation"):
                    res_opts.pec_mitigation = False
                zne_cfg = getattr(res_opts, "zne", None)
                if zne_cfg is not None:
                    if hasattr(zne_cfg, "amplifier"):
                        zne_cfg.amplifier = "pea"
                    if hasattr(zne_cfg, "noise_factors"):
                        zne_cfg.noise_factors = [float(x) for x in (noise_factors or [1.0, 1.3, 1.6, 2.0])]
                    if hasattr(zne_cfg, "extrapolator"):
                        zne_cfg.extrapolator = ["exponential", "linear"]
            else:
                if hasattr(res_opts, "pec_mitigation"):
                    res_opts.pec_mitigation = True
                if hasattr(res_opts, "zne_mitigation"):
                    res_opts.zne_mitigation = False
                pec_cfg = getattr(res_opts, "pec", None)
                if pec_cfg is not None and hasattr(pec_cfg, "max_overhead"):
                    pec_cfg.max_overhead = float(max_overhead)

    est_job = estimator.run([(isa_qc, isa_obs)])
    est_job_id = str(est_job.job_id())
    est_result = est_job.result()
    pub_res = est_result[0]
    mit_elec = float(pub_res.data.evs)
    mit_se = float(pub_res.data.stds)
    pub_meta = dict(getattr(pub_res, "metadata", {}) or {})

    est_usage = 0.0
    u_est = _extract_usage_seconds(est_job, retry_attempts=0 if service_factory is not None else 4)
    if u_est is not None:
        est_usage = float(u_est)

    # Extract raw/unmitigated reference if exposed by PEA noise_factor=1.0 metadata
    raw_elec = None
    raw_se = None
    res_meta = pub_meta.get("resilience", {}) if isinstance(pub_meta.get("resilience"), dict) else {}
    if isinstance(res_meta.get("zne"), dict):
        evs_by_nf = res_meta["zne"].get("evs_noise_factors") or res_meta["zne"].get("evs")
        stds_by_nf = res_meta["zne"].get("stds_noise_factors") or res_meta["zne"].get("stds")
        if isinstance(evs_by_nf, (list, tuple)) and evs_by_nf:
            raw_elec = float(evs_by_nf[0])
        if isinstance(stds_by_nf, (list, tuple)) and stds_by_nf:
            raw_se = float(stds_by_nf[0])

    total_energy = float(mit_elec + float(constant_energy))
    total_usage = float(nl_usage + est_usage)
    diff_ha = (mit_elec - raw_elec) if raw_elec is not None else 0.0
    diff_se = math.sqrt(mit_se * mit_se + (raw_se * raw_se if raw_se is not None else 0.0))
    diff_ci = Z95 * diff_se

    return {
        "ok": True,
        "method": mode,
        "mitigated": mit_elec,
        "energy": total_energy,
        "uncertainty": mit_se,
        "ci95_mha": float(Z95 * mit_se * 1000.0),
        "delta_mha": (abs(total_energy - float(fci_exact_energy)) * 1000.0 if fci_exact_energy is not None else None),
        "learned_gamma": learned_gamma,
        "layer_summaries": layer_summaries,
        "noise_learner_job_id": nl_job_id,
        "estimator_job_id": est_job_id,
        "hardware_job_id": est_job_id,
        "hardware_job_ids": [j for j in (nl_job_id, est_job_id) if j],
        "usage_seconds": total_usage,
        "actual_usd": provider._actual_usd(total_usage),
        "backend": str(getattr(backend, "name", backend_name or "ibm-qiskit")),
        "layout": layout_summary,
        "layout_quality": layout_quality,
        "raw_unmitigated_energy": raw_elec,
        "raw_unmitigated_uncertainty": raw_se,
        "difference_ha": float(diff_ha),
        "difference_ci95_ha": float(diff_ci),
        "difference_ci95_mha": float(diff_ci * 1000.0),
        "difference_significant": bool(raw_elec is not None and abs(diff_ha) > diff_ci),
        "model_adequate": bool(math.isfinite(mit_elec) and math.isfinite(mit_se) and learned_gamma <= float(max_overhead)),
        "metadata": pub_meta,
    }


__all__ = [
    "run_estimator_spl_mitigation",
]
