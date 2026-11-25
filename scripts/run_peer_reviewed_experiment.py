#!/usr/bin/env python3
"""Kubona Full-Stack Real-Life Peer-Reviewed Validation Experiment Harness.

Exercises the entire Kubona platform end-to-end against landmark peer-reviewed
quantum benchmarks:
  - Pillar 1 (Chemistry): H2 STO-3G VQE + ZNE + 6-point PES
      (O'Malley et al. PRX 2016; Kandala et al. Nature 2017, 2019)
  - Pillar 2 (Optimization): MaxCut QAOA p=1, p=2
      (Farhi et al. 2014; Harrigan et al. Nature Physics 2021)
  - Pillar 3 (Magnetism): 4-site periodic 1D Critical TFIM (h/J=1.0)
      (Pfeuty Ann. Phys. 1970; Kandala et al. Nature 2017)
  - Pillar 4 (QEC / Codesign): Steane [[7,1,3]] CNOT + 15-to-1 Magic State
      (Steane PRL 1996; Bravyi & Kitaev PRA 2005)
  - Pillar 5 (Governance & Publication): Pre-registered Vault Predictions,
      Hypotheses + Elo Tournament, Research Plans, Governed Hardware Mandates,
      Verification-v1, Pre-Signoff Scholarly Gate (C-B), Ed25519 Signoff,
      LaTeX/PDF Build, Empirical Deposit, RO-Crate Export, & Ledger Chain Audit.

Modes:
  --mode simulator : Zero QPU spend. Executes all simulator pillars, hardware
                     /prepare mandate checks (no dispatch), codesign, and the
                     complete publication + deposit pipeline.
  --mode hardware  : Same as simulator PLUS signs Ed25519 mandates and executes
                     governed IBM Heron QPU batches (requires --confirm-spend).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

# Exact PySCF STO-3G FCI benchmarks (O'Malley PRX 2016 / Kandala Nature 2017, 2019)
H2_STO3G_FCI: dict[float, tuple[float, float]] = {
    0.50: (-1.042996, -1.055160),
    0.735: (-1.116999, -1.137306),
    1.00: (-1.066109, -1.101150),
    1.50: (-0.910874, -0.998149),
    2.00: (-0.783793, -0.948641),
    2.50: (-0.702944, -0.936055),
}
CHEMICAL_ACCURACY_MHA = 1.6
FARHI_P1_3REG_BOUND = 0.6924


def load_operator_token(repo_root: Path) -> str:
    env_tok = os.environ.get("KUBONA_OPERATOR_TOKEN") or os.environ.get("KUBONA_API_TOKEN")
    if env_tok:
        return env_tok.strip()
    env_file = repo_root / ".env"
    if env_file.exists():
        for line in env_file.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            if k.strip() in ("KUBONA_API_TOKEN", "KUBONA_OPERATOR_TOKEN"):
                return v.strip().strip('"').strip("'")
    return ""


class KubonaClient:
    def __init__(self, base_url: str, token: str):
        self.base_url = base_url.rstrip("/")
        self.token = token

    def request(
        self,
        method: str,
        path: str,
        body: Any = None,
        *,
        auth: bool = True,
        timeout: int = 180,
    ) -> tuple[int, Any]:
        url = self.base_url + path
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if auth and self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                raw = resp.read()
                try:
                    return resp.status, json.loads(raw.decode("utf-8"))
                except Exception:
                    return resp.status, raw
        except urllib.error.HTTPError as exc:
            raw = exc.read()
            try:
                return exc.code, json.loads(raw.decode("utf-8"))
            except Exception:
                return exc.code, raw.decode("utf-8", errors="replace")

    def poll_job(self, session_id: str, job_id: str, *, timeout_s: int = 600, interval_s: float = 1.5) -> dict[str, Any]:
        deadline = time.time() + timeout_s
        while time.time() < deadline:
            code, data = self.request("GET", f"/api/jobs/{job_id}?session_id={session_id}")
            if code == 200 and isinstance(data, dict):
                job = data.get("job", data)
                status = str(job.get("status", ""))
                if status in ("JOB_STATUS_COMPLETED", "completed", "JOB_STATUS_FAILED", "failed", "JOB_STATUS_CANCELLED", "cancelled"):
                    return job
            elif code >= 400:
                raise RuntimeError(f"GET /api/jobs/{job_id} returned {code}: {data}")
            time.sleep(interval_s)
        raise TimeoutError(f"job {job_id} did not settle within {timeout_s}s")


def record(checks: list[dict[str, Any]], name: str, ok: bool, detail: str) -> None:
    checks.append({"check": name, "passed": bool(ok), "detail": detail})
    tag = "PASS" if ok else "FAIL"
    print(f"  [{tag}] {name}: {detail}", flush=True)


def sign_mandate(client: KubonaClient, binding: dict[str, Any]) -> str:
    code, resp = client.request("POST", "/api/mandates/sign", {"binding": binding})
    if code != 200 or not isinstance(resp, dict):
        raise RuntimeError(f"mandate sign failed ({code}): {resp}")
    ref = resp.get("mandate_ref") or resp.get("id") or (resp.get("mandate") or {}).get("id")
    if not ref:
        raise RuntimeError(f"mandate sign response missing mandate_ref: {resp}")
    return str(ref)


def main() -> int:
    parser = argparse.ArgumentParser(description="Kubona full-stack peer-reviewed validation harness")
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--mode", choices=("simulator", "hardware"), default="simulator")
    parser.add_argument("--backend", default="ibm_fez")
    parser.add_argument("--budget-usd", type=float, default=25.0)
    parser.add_argument("--shots", type=int, default=4096)
    parser.add_argument(
        "--hardware-pillars",
        default="vqe_raw,vqe_zne,pes,qaoa,tfim",
        help="Comma-separated hardware dispatches when --mode hardware",
    )
    parser.add_argument(
        "--confirm-spend",
        action="store_true",
        help="Required safety flag when --mode hardware to authorize QPU dispatch",
    )
    parser.add_argument("--report-out", default="")
    args = parser.parse_args()

    if args.mode == "hardware" and not args.confirm_spend:
        print("Refusing --mode hardware without --confirm-spend safety flag.", file=sys.stderr)
        return 2

    repo_root = Path(__file__).resolve().parents[1]
    token = load_operator_token(repo_root)
    if not token:
        print("Warning: no operator token found in env or .env; operator endpoints may return 401.", file=sys.stderr)

    client = KubonaClient(args.base_url, token)
    checks: list[dict[str, Any]] = []
    artifacts_summary: dict[str, Any] = {
        "mode": args.mode,
        "backend": args.backend,
        "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }

    print(f"=== Stage 0: Pre-flight Health & Integrity ({args.base_url}) ===")
    code, health = client.request("GET", "/api/health", auth=False)
    record(checks, "health", code == 200 and health.get("status") == "healthy", f"code={code} status={health.get('status')}")

    code, backends = client.request("GET", "/api/quantum/backends", auth=False)
    ibm_ready = False
    if code == 200 and isinstance(backends, dict):
        for b in backends.get("backends", []):
            if b.get("provider") == "ibm-qiskit":
                ibm_ready = bool(b.get("executable_now"))
    record(checks, "ibm_provider_ready", ibm_ready, f"ibm-qiskit executable_now={ibm_ready}")

    code, verify_before = client.request("GET", "/api/ledger/verify")
    chain_ok = code == 200 and isinstance(verify_before, dict) and verify_before.get("valid", True) is not False
    record(checks, "ledger_chain_preflight", chain_ok, f"code={code}")

    print("\n=== Stage 1: Session, Autonomy, Prediction Vault & Hypotheses ===")
    sess_name = f"Peer-Reviewed Benchmark ({args.mode}, {time.strftime('%Y-%m-%d %H:%M')})"
    code, sess_resp = client.request(
        "POST",
        "/api/sessions",
        {"name": sess_name, "budget_usd": args.budget_usd, "target_system": "H2"},
    )
    if code not in (200, 201) or not isinstance(sess_resp, dict):
        record(checks, "create_session", False, f"code={code} resp={sess_resp}")
        return 1
    sid = (sess_resp.get("session") or sess_resp).get("id")
    artifacts_summary["session_id"] = sid
    record(checks, "create_session", bool(sid), f"session_id={sid}")

    code, auto_resp = client.request("GET", f"/api/sessions/{sid}/autonomy")
    record(checks, "session_autonomy", code == 200, f"code={code}")

    code, seal_resp = client.request(
        "POST",
        "/api/vault/predictions/seal",
        {
            "session_id": sid,
            "target_system": "H2",
            "method": "vqe-sto3g",
            "quantity": "ground_state_energy_ha",
            "expected_mean": H2_STO3G_FCI[0.735][1],
            "expected_std": 0.0016,
            "budget_cap_usd": args.budget_usd,
            "author": "kubona-peer-reviewed-benchmark",
        },
    )
    pred_hash = seal_resp.get("prediction_hash") if isinstance(seal_resp, dict) else None
    record(checks, "prediction_vault_seal", code in (200, 201) and bool(pred_hash), f"code={code} hash={str(pred_hash)[:16]}...")

    code, hyp_resp = client.request(
        "POST",
        f"/api/sessions/{sid}/hypotheses",
        {
            "goal": "Reproduce Kandala 2019 ZNE chemical accuracy on H2 STO-3G and verify cross-domain QAOA/TFIM benchmarks",
            "statement": "Hardware-efficient VQE with ZNE achieves <= 1.6 mHa error at H2 equilibrium (0.735 A) and outperforms raw hardware.",
            "rationale": "Kandala et al. Nature 567, 491-495 (2019) demonstrated ZNE suppresses incoherent error into the chemical accuracy window.",
            "falsifier": "ZNE absolute error exceeds raw hardware error or exceeds 1.6 mHa on simulator.",
            "use_llm": False,
        },
    )
    hyp_id = None
    if isinstance(hyp_resp, dict):
        hyp_id = (hyp_resp.get("hypothesis") or hyp_resp).get("id")
        if not hyp_id and isinstance(hyp_resp.get("hypotheses"), list) and hyp_resp["hypotheses"]:
            hyp_id = hyp_resp["hypotheses"][0].get("id")
    record(checks, "hypothesis_create", code in (200, 201), f"code={code} id={hyp_id}")

    if hyp_id:
        client.request("PATCH", f"/api/sessions/{sid}/hypotheses/{hyp_id}", {"status": "shortlisted"})

    code, plan_resp = client.request(
        "POST",
        f"/api/sessions/{sid}/plans",
        {
            "hypothesis_id": hyp_id or "",
            "statement": "Confirm H2 VQE/PES, MaxCut QAOA, 1D Critical TFIM, and Steane QEC against peer-reviewed baselines.",
            "method": "vqe",
            "observable": "ground_state_energy_ha",
            "acceptance_metric": "delta_mha",
            "tolerance": CHEMICAL_ACCURACY_MHA,
            "stop_rule": "halt_if_delta_exceeds_tolerance_or_budget",
            "acknowledge_prior_negative": True,
            "spec": {"kind": "vqe", "molecule": "H2", "bond_distance": 0.735, "layers": 2, "shots": 2048, "seed": 7},
        },
    )
    record(checks, "research_plan_create", code in (200, 201), f"code={code}")

    hw_pillars = {p.strip() for p in args.hardware_pillars.split(",") if p.strip()}

    print("\n=== Stage 2: Pillar 1 — Quantum Chemistry (H2 STO-3G VQE + 6-Point PES) ===", flush=True)
    code, vqe_launch = client.request(
        "POST",
        "/api/experiments",
        {
            "session_id": sid,
            "kind": "vqe",
            "budget_usd": 1.0,
            "spec": {
                "kind": "vqe",
                "molecule": "H2",
                "bond_distance": 0.735,
                "basis": "sto-3g",
                "layers": 2,
                "shots": 4096,
                "seed": 7,
            },
        },
    )
    vqe_job_id = (vqe_launch.get("job") or vqe_launch).get("id") if isinstance(vqe_launch, dict) else None
    vqe_job = client.poll_job(sid, vqe_job_id) if vqe_job_id else {}
    vqe_res = json.loads(vqe_job.get("result_json") or "{}") if isinstance(vqe_job.get("result_json"), str) else (vqe_job.get("result_json") or vqe_job.get("result") or {})
    e_vqe_exact = vqe_res.get("energy_exact")
    e_vqe_meas = vqe_res.get("energy_measured", vqe_res.get("energy"))
    ci95_mha = float(vqe_res.get("ci95_mha") or CHEMICAL_ACCURACY_MHA)
    e_fci_ref = H2_STO3G_FCI[0.735][1]
    exact_delta_mha = abs((float(e_vqe_exact) - e_fci_ref) * 1000.0) if e_vqe_exact is not None else 9999.0
    meas_delta_mha = abs((float(e_vqe_meas) - e_fci_ref) * 1000.0) if e_vqe_meas is not None else 9999.0
    record(
        checks,
        "pillar1_h2_vqe_sim_chem_accuracy",
        vqe_res.get("success") is True and exact_delta_mha <= CHEMICAL_ACCURACY_MHA and meas_delta_mha <= max(CHEMICAL_ACCURACY_MHA, ci95_mha),
        f"job={vqe_job_id} E_exact={e_vqe_exact:.6f} (|Δ|={exact_delta_mha:.3f} mHa) E_meas={e_vqe_meas:.6f} (±{ci95_mha:.2f} mHa) vs FCI={e_fci_ref}",
    )
    if pred_hash and e_vqe_exact is not None:
        ev_code, ev_resp = client.request(
            "POST",
            "/api/vault/predictions/evaluate",
            {"session_id": sid, "prediction_hash": pred_hash, "measured_value": float(e_vqe_exact)},
        )
        record(checks, "prediction_vault_evaluate", ev_code == 200, f"code={ev_code} verdict={ev_resp.get('verdict') if isinstance(ev_resp, dict) else ev_resp}")

    code, vqe_prep = client.request(
        "POST",
        "/api/experiments/vqe/hardware/prepare",
        {"session_id": sid, "source_job_id": vqe_job_id, "backend": args.backend, "budget_usd": 5.0, "shots": args.shots},
    )
    record(checks, "pillar1_vqe_hw_prepare", code == 200 and bool(vqe_prep.get("binding")), f"code={code}")

    if args.mode == "hardware" and "vqe_raw" in hw_pillars and code == 200:
        mref = sign_mandate(client, vqe_prep["binding"])
        c_raw, raw_resp = client.request(
            "POST",
            "/api/experiments/vqe/hardware",
            {
                "session_id": sid,
                "source_job_id": vqe_job_id,
                "backend": args.backend,
                "budget_usd": 5.0,
                "shots": args.shots,
                "mitigation": "none",
                "mandate_ref": mref,
            },
        )
        raw_jid = (raw_resp.get("job") or raw_resp).get("id") if isinstance(raw_resp, dict) else None
        raw_job = client.poll_job(sid, raw_jid, timeout_s=900) if raw_jid else {}
        raw_res = json.loads(raw_job.get("result_json") or "{}") if isinstance(raw_job.get("result_json"), str) else (raw_job.get("result_json") or raw_job.get("result") or {})
        raw_delta = abs((float(raw_res.get("energy", 0)) - e_fci_ref) * 1000.0) if raw_res.get("energy") is not None else 9999.0
        record(checks, "pillar1_h2_vqe_hw_raw", raw_res.get("success") is True, f"job={raw_jid} |Δ_raw|={raw_delta:.3f} mHa")

        if "vqe_zne" in hw_pillars:
            _, zne_prep = client.request(
                "POST",
                "/api/experiments/vqe/hardware/prepare",
                {
                    "session_id": sid,
                    "source_job_id": vqe_job_id,
                    "backend": args.backend,
                    "budget_usd": 8.0,
                    "shots": args.shots,
                    "mitigation": "ZNE",
                    "mitigation_scales": [1, 3, 5],
                },
            )
            mref_zne = sign_mandate(client, zne_prep["binding"])
            _, zne_resp = client.request(
                "POST",
                "/api/experiments/vqe/hardware",
                {
                    "session_id": sid,
                    "source_job_id": vqe_job_id,
                    "backend": args.backend,
                    "budget_usd": 8.0,
                    "shots": args.shots,
                    "mitigation": "ZNE",
                    "mitigation_scales": [1, 3, 5],
                    "mandate_ref": mref_zne,
                },
            )
            zne_jid = (zne_resp.get("job") or zne_resp).get("id") if isinstance(zne_resp, dict) else None
            zne_job = client.poll_job(sid, zne_jid, timeout_s=900) if zne_jid else {}
            zne_res = json.loads(zne_job.get("result_json") or "{}") if isinstance(zne_job.get("result_json"), str) else (zne_job.get("result_json") or zne_job.get("result") or {})
            zne_delta = abs((float(zne_res.get("energy", 0)) - e_fci_ref) * 1000.0) if zne_res.get("energy") is not None else 9999.0
            record(
                checks,
                "pillar1_h2_vqe_hw_zne_kandala2019",
                zne_res.get("success") is True and bool(zne_res.get("counts_by_term")),
                f"job={zne_jid} |Δ_ZNE|={zne_delta:.3f} mHa vs |Δ_raw|={raw_delta:.3f} mHa (terms={len(zne_res.get('counts_by_term') or {})})",
            )

    code, pes_launch = client.request(
        "POST",
        "/api/experiments",
        {
            "session_id": sid,
            "kind": "pes",
            "budget_usd": 1.0,
            "spec": {
                "kind": "pes",
                "molecule": "H2",
                "r_min": 0.50,
                "r_max": 2.00,
                "steps": 7,
                "layers": 2,
                "shots": 1024,
                "seed": 7,
            },
        },
    )
    pes_job_id = (pes_launch.get("job") or pes_launch).get("id") if isinstance(pes_launch, dict) else None
    pes_job = client.poll_job(sid, pes_job_id) if pes_job_id else {}
    pes_res = json.loads(pes_job.get("result_json") or "{}") if isinstance(pes_job.get("result_json"), str) else (pes_job.get("result_json") or pes_job.get("result") or {})
    pts = pes_res.get("points") or []
    min_pt = min(pts, key=lambda p: p.get("energy", 999.0)) if pts else {}
    record(
        checks,
        "pillar1_h2_pes_7pt_sim",
        pes_res.get("success") is True and len(pts) == 7 and abs(float(min_pt.get("bond_distance", 0)) - 0.75) < 1e-6,
        f"job={pes_job_id} points={len(pts)} min_R={min_pt.get('bond_distance')} A E={min_pt.get('energy')}",
    )

    code, pes_prep = client.request(
        "POST",
        "/api/experiments/pes/hardware/prepare",
        {"session_id": sid, "source_job_id": pes_job_id, "backend": args.backend, "budget_usd": 10.0, "shots": args.shots},
    )
    record(checks, "pillar1_pes_hw_prepare", code == 200 and bool(pes_prep.get("binding")), f"code={code}")

    if args.mode == "hardware" and "pes" in hw_pillars and code == 200:
        mref_pes = sign_mandate(client, pes_prep["binding"])
        _, hpes_resp = client.request(
            "POST",
            "/api/experiments/pes/hardware",
            {
                "session_id": sid,
                "source_job_id": pes_job_id,
                "backend": args.backend,
                "budget_usd": 10.0,
                "shots": args.shots,
                "mandate_ref": mref_pes,
            },
        )
        hpes_jid = (hpes_resp.get("job") or hpes_resp).get("id") if isinstance(hpes_resp, dict) else None
        hpes_job = client.poll_job(sid, hpes_jid, timeout_s=1200) if hpes_jid else {}
        hpes_res = json.loads(hpes_job.get("result_json") or "{}") if isinstance(hpes_job.get("result_json"), str) else (hpes_job.get("result_json") or hpes_job.get("result") or {})
        record(checks, "pillar1_h2_pes_hw", hpes_res.get("success") is True, f"job={hpes_jid} points={len(hpes_res.get('points') or [])}")

    print("\n=== Stage 3: Pillar 2 — MaxCut QAOA (Farhi 2014 / Harrigan 2021) ===")
    ratios = {}
    qaoa_p2_job_id = None
    for p_layers in (1, 2):
        _, ql = client.request(
            "POST",
            "/api/experiments",
            {
                "session_id": sid,
                "kind": "qaoa",
                "budget_usd": 1.0,
                "spec": {"kind": "qaoa", "layers": p_layers, "shots": 1024, "seed": 7},
            },
        )
        qjid = (ql.get("job") or ql).get("id") if isinstance(ql, dict) else None
        qj = client.poll_job(sid, qjid) if qjid else {}
        qres = json.loads(qj.get("result_json") or "{}") if isinstance(qj.get("result_json"), str) else (qj.get("result_json") or qj.get("result") or {})
        ratios[p_layers] = float(qres.get("approximation_ratio") or 0.0)
        if p_layers == 2:
            qaoa_p2_job_id = qjid
        record(
            checks,
            f"pillar2_qaoa_p{p_layers}_sim",
            qres.get("success") is True and ratios[p_layers] >= FARHI_P1_3REG_BOUND and qres.get("ratio_ci95") is not None,
            f"job={qjid} ratio={ratios[p_layers]:.4f} ci95={qres.get('ratio_ci95')}",
        )
    record(checks, "pillar2_qaoa_depth_monotonicity", ratios.get(2, 0.0) >= ratios.get(1, 0.0), f"p1={ratios.get(1):.4f} -> p2={ratios.get(2):.4f}")

    code, qaoa_prep = client.request(
        "POST",
        "/api/experiments/qaoa/hardware/prepare",
        {"session_id": sid, "source_job_id": qaoa_p2_job_id, "backend": args.backend, "budget_usd": 3.0, "shots": args.shots},
    )
    record(checks, "pillar2_qaoa_hw_prepare", code == 200 and bool(qaoa_prep.get("binding")), f"code={code}")

    if args.mode == "hardware" and "qaoa" in hw_pillars and code == 200:
        mref_q = sign_mandate(client, qaoa_prep["binding"])
        _, hq_resp = client.request(
            "POST",
            "/api/experiments/qaoa/hardware",
            {
                "session_id": sid,
                "source_job_id": qaoa_p2_job_id,
                "backend": args.backend,
                "budget_usd": 3.0,
                "shots": args.shots,
                "mandate_ref": mref_q,
            },
        )
        hq_jid = (hq_resp.get("job") or hq_resp).get("id") if isinstance(hq_resp, dict) else None
        hq_job = client.poll_job(sid, hq_jid, timeout_s=900) if hq_jid else {}
        hq_res = json.loads(hq_job.get("result_json") or "{}") if isinstance(hq_job.get("result_json"), str) else (hq_job.get("result_json") or hq_job.get("result") or {})
        record(checks, "pillar2_qaoa_hw", hq_res.get("success") is True, f"job={hq_jid} ratio={hq_res.get('approximation_ratio')}")

    print("\n=== Stage 4: Pillar 3 — 1D Critical TFIM (Pfeuty 1970 / Kandala 2017) ===")
    _, sl = client.request(
        "POST",
        "/api/experiments",
        {
            "session_id": sid,
            "kind": "spin_chain",
            "budget_usd": 1.0,
            "spec": {
                "kind": "spin_chain",
                "model": "tfim",
                "n_sites": 4,
                "coupling": 1.0,
                "field": 1.0,
                "periodic": True,
                "layers": 2,
                "shots": 1024,
                "seed": 7,
            },
        },
    )
    spin_jid = (sl.get("job") or sl).get("id") if isinstance(sl, dict) else None
    spin_job = client.poll_job(sid, spin_jid) if spin_jid else {}
    spin_res = json.loads(spin_job.get("result_json") or "{}") if isinstance(spin_job.get("result_json"), str) else (spin_job.get("result_json") or spin_job.get("result") or {})
    e_exact = spin_res.get("energy_exact")
    e_spin = spin_res.get("energy")
    record(
        checks,
        "pillar3_tfim_4site_critical_sim",
        spin_res.get("success") is True and e_exact is not None,
        f"job={spin_jid} E_VQE={e_spin} vs E_exact={e_exact} (E_exact/4={float(e_exact)/4.0 if e_exact is not None else 0:.5f} vs -4/pi=-1.27324)",
    )

    code, spin_prep = client.request(
        "POST",
        "/api/experiments/spin-chain/hardware/prepare",
        {"session_id": sid, "source_job_id": spin_jid, "backend": args.backend, "budget_usd": 4.0, "shots": args.shots},
    )
    record(checks, "pillar3_tfim_hw_prepare", code == 200 and bool(spin_prep.get("binding")), f"code={code}")

    if args.mode == "hardware" and "tfim" in hw_pillars and code == 200:
        mref_s = sign_mandate(client, spin_prep["binding"])
        _, hs_resp = client.request(
            "POST",
            "/api/experiments/spin-chain/hardware",
            {
                "session_id": sid,
                "source_job_id": spin_jid,
                "backend": args.backend,
                "budget_usd": 4.0,
                "shots": args.shots,
                "mandate_ref": mref_s,
            },
        )
        hs_jid = (hs_resp.get("job") or hs_resp).get("id") if isinstance(hs_resp, dict) else None
        hs_job = client.poll_job(sid, hs_jid, timeout_s=900) if hs_jid else {}
        hs_res = json.loads(hs_job.get("result_json") or "{}") if isinstance(hs_job.get("result_json"), str) else (hs_job.get("result_json") or hs_job.get("result") or {})
        record(checks, "pillar3_tfim_hw", hs_res.get("success") is True, f"job={hs_jid} E_hw={hs_res.get('energy')}")

    print("\n=== Stage 5: Pillar 4 — Fault-Tolerant QEC & Circuit Codesign ===", flush=True)
    code, cs_steane = client.request(
        "POST",
        "/api/codesign/qec-joint",
        {"kind": "steane_cnot"},
    )
    steane_res = (cs_steane.get("result") or cs_steane) if isinstance(cs_steane, dict) else {}
    sfc = steane_res.get("single_fault_check") or {}
    undet = sfc.get("undetectable_logical_faults", sfc.get("undetectable_faults", 0))
    record(
        checks,
        "pillar4_codesign_qec_steane",
        code == 200 and steane_res.get("passed") is True and sfc.get("faults_tested") == 336 and undet == 0,
        f"code={code} passed={steane_res.get('passed')} faults={sfc.get('faults_tested')} undetectable={undet}",
    )

    code, cs_magic = client.request(
        "POST",
        "/api/codesign/qec-joint",
        {"kind": "magic_state_15_to_1"},
    )
    magic_res = (cs_magic.get("result") or cs_magic) if isinstance(cs_magic, dict) else {}
    record(
        checks,
        "pillar4_codesign_qec_magic_15_to_1",
        code == 200 and magic_res.get("ok") is True and (magic_res.get("distance_ge_3") is True or magic_res.get("passed") is True),
        f"code={code} distance_ge_3={magic_res.get('distance_ge_3')} gamma={magic_res.get('gamma')} passed={magic_res.get('passed')}",
    )

    code, cs_packs = client.request("GET", "/api/codesign/packs")
    record(checks, "pillar4_codesign_packs", code == 200, f"code={code}")

    print("\n=== Stage 6: Pillar 5 — Manuscript, Verification-v1, Signoff, Build & Deposit ===", flush=True)
    code, camp = client.request("GET", f"/api/sessions/{sid}/campaign")
    exps = (camp.get("campaign") or camp).get("experiments", []) if isinstance(camp, dict) else []
    record(checks, "campaign_projection", code == 200 and len(exps) >= 3, f"code={code} experiments={len(exps)}")

    code, ms_create = client.request(
        "POST",
        f"/api/sessions/{sid}/manuscripts",
        {"title": f"Peer-Reviewed Cross-Domain Validation ({args.mode.upper()})", "build": False},
    )
    ms = (ms_create.get("manuscript") or ms_create) if isinstance(ms_create, dict) else {}
    mid = ms.get("id")
    chash = ms.get("content_hash")
    artifacts_summary["manuscript_id"] = mid
    record(checks, "manuscript_create", code in (200, 201) and bool(mid), f"manuscript_id={mid}")

    if mid:
        code, rev = client.request("POST", f"/api/sessions/{sid}/manuscripts/{mid}/reviews", {})
        record(checks, "manuscript_review", code == 200, f"code={code}")

        code, ver = client.request("POST", f"/api/sessions/{sid}/manuscripts/{mid}/verify", {})
        vsum = (ver.get("report") or ver).get("summary", {}) if isinstance(ver, dict) else {}
        record(checks, "manuscript_verify_v1", code == 200 and vsum.get("blocking", 0) == 0, f"status={vsum.get('status')} blocking={vsum.get('blocking')}")

        # Populate scholarly metadata before signoff (satisfies C-B gate)
        scholarly = {
            "scholarly": {
                "affiliation": "Kubona Autonomous Science Validation",
                "corresponding_author": "Kubona Operator <operator@kubona.ai>",
                "competing_interests": "None declared.",
                "funding": "Open-source self-funded benchmark.",
                "license": "CC-BY-4.0",
            }
        }
        code, upd = client.request("PUT", f"/api/sessions/{sid}/manuscripts/{mid}", scholarly)
        if code == 200 and isinstance(upd, dict):
            chash = (upd.get("manuscript") or upd).get("content_hash") or chash
        record(checks, "manuscript_scholarly_put", code == 200, f"code={code}")

        client.request("POST", f"/api/sessions/{sid}/manuscripts/{mid}/reviews", {})
        code, so = client.request(
            "POST",
            f"/api/sessions/{sid}/manuscripts/{mid}/signoff",
            {"content_hash": chash, "acknowledge_unverified": True},
        )
        record(checks, "manuscript_signoff", code == 200, f"code={code}")

        code, bld = client.request("POST", f"/api/sessions/{sid}/manuscripts/{mid}/build", {}, timeout=300)
        arts = (bld.get("manuscript") or bld).get("artifacts", []) if isinstance(bld, dict) else []
        art_names = {a.get("name") for a in arts if isinstance(a, dict)}
        record(
            checks,
            "manuscript_build_pdf_and_supplement",
            code == 200 and "main.pdf" in art_names and "supplement/datasheet.json" in art_names,
            f"code={code} artifacts={len(art_names)} has_pdf={'main.pdf' in art_names} has_datasheet={'supplement/datasheet.json' in art_names}",
        )

        code, dep = client.request("POST", f"/api/sessions/{sid}/manuscripts/{mid}/deposit", {"track": "empirical"})
        record(checks, "manuscript_empirical_deposit", code == 200, f"code={code}")

        code, exp = client.request("GET", f"/api/sessions/{sid}/manuscripts/{mid}/export")
        exp_sha = hashlib.sha256(exp if isinstance(exp, bytes) else json.dumps(exp).encode()).hexdigest()[:16] if code == 200 else ""
        record(checks, "manuscript_rocrate_export", code == 200, f"code={code} sha256={exp_sha}...")

    print("\n=== Stage 7: Post-Run Integrity Audit ===")
    code, verify_after = client.request("GET", "/api/ledger/verify")
    record(checks, "ledger_chain_postflight", code == 200 and isinstance(verify_after, dict) and verify_after.get("valid", True) is not False, f"code={code}")

    passed = sum(1 for c in checks if c["passed"])
    total = len(checks)
    artifacts_summary["passed"] = passed
    artifacts_summary["total"] = total
    artifacts_summary["checks"] = checks

    print(f"\n=== SUMMARY: {passed}/{total} checks passed (mode={args.mode}) ===")
    if args.report_out:
        out_path = Path(args.report_out)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(artifacts_summary, indent=2), encoding="utf-8")
        print(f"Report written to {out_path}")
    return 0 if passed == total else 1


if __name__ == "__main__":
    raise SystemExit(main())
