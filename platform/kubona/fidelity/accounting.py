"""Fidelity Accounting & Physical Pre-Flight Verification (Ring 3).

A second budget rail alongside the financial spend rail.
Before a job may spend QPU time or submit to cloud queues, the platform must:
1. Predict the physical fidelity F_pred from device noise profiles (T1, T2 <= 2*T1, readout error).
2. Measure calibration freshness (calibration age < device_refresh_interval).
3. Enforce fidelity floors: jobs below threshold are downgraded to simulator or mitigated.
"""

from __future__ import annotations

import json
import math
import os
import time
import dataclasses
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Optional


@dataclass(frozen=True)
class DeviceNoiseProfile:
    device_name: str
    t1_us: float
    t2_us: float  # Enforces physical constraint: t2 <= 2 * t1
    single_qubit_error: float
    two_qubit_error: float
    readout_error: float
    last_calibrated_epoch: float
    qubits: int = 0
    source: str = ""
    version: str = ""
    calibrated_at: str = ""

    def is_physically_sound(self) -> bool:
        return self.t2_us <= (2.0 * self.t1_us + 1e-6)

    def calibration_age_hours(self) -> float:
        return max(0.0, (time.time() - self.last_calibrated_epoch) / 3600.0)


def _contract_path() -> Path:
    # Installed wheels keep contracts at /app/contracts (the image sets
    # KUBONA_FIDELITY_DEVICES); source checkouts resolve the repo tree.
    override = os.environ.get("KUBONA_FIDELITY_DEVICES")
    if override:
        return Path(override)
    return Path(__file__).resolve().parents[2] / "contracts" / "fidelity-devices.json"


_CONTRACT_PATH = _contract_path()


def _parse_calibrated_at(value: str) -> float:
    """Real calibration epoch; missing/unparseable is stale (0), never now."""
    from datetime import datetime, timezone
    try:
        return datetime.fromisoformat(str(value).replace("Z", "+00:00")).timestamp()
    except (TypeError, ValueError):
        return 0.0


def _load_fidelity_contract() -> tuple[Dict[str, DeviceNoiseProfile], Dict[str, str], float, float]:
    raw = json.loads(_CONTRACT_PATH.read_text(encoding="utf-8"))
    catalog: Dict[str, DeviceNoiseProfile] = {}
    for entry in raw.get("devices", []):
        name = str(entry["name"])
        calibrated_epoch = _parse_calibrated_at(str(entry.get("calibrated_at", "")))
        prof = DeviceNoiseProfile(
            device_name=name,
            t1_us=float(entry["t1_us"]),
            t2_us=float(entry["t2_us"]),
            single_qubit_error=float(entry["single_qubit_error"]),
            two_qubit_error=float(entry["two_qubit_error"]),
            readout_error=float(entry.get("readout_error", 0.0)),
            last_calibrated_epoch=calibrated_epoch,
            qubits=int(entry.get("qubits", 0)),
            source=str(entry.get("source", "")),
            version=str(entry.get("version", "")),
            calibrated_at=str(entry.get("calibrated_at", "")),
        )
        if not prof.is_physically_sound():
            raise ValueError(f"device profile {name} violates T2 <= 2*T1")
        if not prof.source or not prof.version or not prof.calibrated_at:
            raise ValueError(f"device profile {name} lacks provenance")
        catalog[name] = prof
        for alias in entry.get("aliases", []):
            alias_str = str(alias)
            catalog[alias_str] = dataclasses.replace(prof, device_name=alias_str)
    prov_map = {str(k): str(v) for k, v in raw.get("provider_device_map", {}).items()}
    default_floor = float(raw.get("default_floor_python", 0.80))
    max_age_h = float(raw.get("max_calibration_age_hours", 24.0))
    return catalog, prov_map, default_floor, max_age_h


# Standard calibrated device profiles loaded from platform/contracts/fidelity-devices.json
DEVICE_CATALOG, PROVIDER_DEVICE_MAP, DEFAULT_FIDELITY_FLOOR, DEFAULT_MAX_CALIBRATION_AGE_HOURS = (
    _load_fidelity_contract()
)


@dataclass
class FidelityAdmissionVerdict:
    admitted: bool
    predicted_fidelity: float
    fidelity_floor: float
    calibration_fresh: bool
    recommended_action: str  # "EXECUTE_DIRECT", "APPLY_ZNE_MITIGATION", "DOWNGRADE_TO_AER_SIMULATOR", "REJECT"
    rejection_reason: Optional[str] = None


class FidelityAccountingEngine:
    def __init__(self, default_floor: float = 0.80, max_calibration_age_hours: float = 24.0):
        self.default_floor = default_floor
        self.max_calibration_age_hours = max_calibration_age_hours

    def predict_fidelity(
        self,
        device: DeviceNoiseProfile,
        n_qubits: int,
        n_single_qubit_gates: int,
        n_two_qubit_gates: int,
        circuit_duration_us: float = 1.0,
    ) -> float:
        """Predicts circuit fidelity from gate error propagation and decoherence."""
        # Decoherence factor: exp(-t / T1) * exp(-t / T2)
        decoherence_factor = math.exp(-circuit_duration_us / device.t1_us) * math.exp(-circuit_duration_us / device.t2_us)
        # Gate error propagation: (1 - e_1q)^N1 * (1 - e_2q)^N2
        gate_survival = ((1.0 - device.single_qubit_error) ** n_single_qubit_gates) * (
            (1.0 - device.two_qubit_error) ** n_two_qubit_gates
        )
        # Readout survival: (1 - e_ro)^n_qubits
        readout_survival = (1.0 - device.readout_error) ** n_qubits

        f_pred = decoherence_factor * gate_survival * readout_survival
        return max(0.0, min(1.0, f_pred))

    def fidelity_budget(
        self,
        device: DeviceNoiseProfile,
        n_qubits: int,
        n_single_qubit_gates: int,
        n_two_qubit_gates: int,
        circuit_duration_us: float = 1.0,
    ) -> dict[str, Any]:
        """Per-mechanism infidelity budget via leave-one-out ablation.

        Each mechanism is toggled to ideal in turn; the fidelity gain names
        what that mechanism costs. Shares are normalized gains (first-order
        attribution, NOT an additive ledger — the model is multiplicative,
        so shares need not sum to the total infidelity; the absolute gains
        are the honest numbers, shares are for ranking).
        Returns predicted_fidelity, per-mechanism entries
        {name, fidelity_without, absolute_gain, share}, and the dominant
        mechanism (None when the device is already ideal).
        """
        base = self.predict_fidelity(device, n_qubits, n_single_qubit_gates,
                                     n_two_qubit_gates, circuit_duration_us)
        ablations = [
            ("decoherence", {"t1_us": 1e12, "t2_us": 1e12}),
            ("single_qubit_gates", {"single_qubit_error": 0.0}),
            ("two_qubit_gates", {"two_qubit_error": 0.0}),
            ("readout", {"readout_error": 0.0}),
        ]
        entries = []
        for name, override in ablations:
            improved = self.predict_fidelity(
                dataclasses.replace(device, **override),
                n_qubits, n_single_qubit_gates, n_two_qubit_gates,
                circuit_duration_us)
            entries.append({"name": name, "fidelity_without": round(improved, 6),
                            "absolute_gain": round(max(0.0, improved - base), 6)})
        total_gain = sum(e["absolute_gain"] for e in entries)
        for e in entries:
            e["share"] = round(e["absolute_gain"] / total_gain, 4) if total_gain > 0 else 0.0
        dominant = max(entries, key=lambda e: e["absolute_gain"])["name"] if total_gain > 0 else None
        return {"predicted_fidelity": round(base, 6), "mechanisms": entries,
                "dominant": dominant}

    def evaluate_admission(
        self,
        device_name: str,
        n_qubits: int,
        n_single_qubit_gates: int,
        n_two_qubit_gates: int,
        fidelity_floor: float | None = None,
    ) -> FidelityAdmissionVerdict:
        floor = fidelity_floor or self.default_floor
        device = (
            DEVICE_CATALOG.get(device_name)
            or DEVICE_CATALOG.get(device_name.replace("_", "-"))
        )
        if device is None:
            return FidelityAdmissionVerdict(
                admitted=False,
                predicted_fidelity=0.0,
                fidelity_floor=floor,
                calibration_fresh=False,
                recommended_action="REJECT",
                rejection_reason=(
                    f"unknown fidelity device {device_name!r}: refusing to inherit the "
                    "simulator profile (no silent simulation)"),
            )
        if n_qubits < 0 or n_single_qubit_gates < 0 or n_two_qubit_gates < 0:
            return FidelityAdmissionVerdict(
                admitted=False, predicted_fidelity=0.0, fidelity_floor=floor,
                calibration_fresh=False, recommended_action="REJECT",
                rejection_reason="negative circuit counts: refusing admission",
            )
        if device.qubits and n_qubits > device.qubits:
            return FidelityAdmissionVerdict(
                admitted=False, predicted_fidelity=0.0, fidelity_floor=floor,
                calibration_fresh=False, recommended_action="REJECT",
                rejection_reason=(
                    f"circuit uses {n_qubits} qubits but {device.device_name} has "
                    f"{device.qubits}: refusing admission"),
            )

        # Check calibration freshness
        age = device.calibration_age_hours()
        fresh = age <= self.max_calibration_age_hours

        f_pred = self.predict_fidelity(device, n_qubits, n_single_qubit_gates, n_two_qubit_gates)

        if not fresh:
            return FidelityAdmissionVerdict(
                admitted=False,
                predicted_fidelity=f_pred,
                fidelity_floor=floor,
                calibration_fresh=False,
                recommended_action="DOWNGRADE_TO_AER_SIMULATOR",
                rejection_reason=f"Stale calibration data ({age:.1f} hours old > {self.max_calibration_age_hours}h limit).",
            )

        if f_pred >= floor:
            return FidelityAdmissionVerdict(
                admitted=True,
                predicted_fidelity=f_pred,
                fidelity_floor=floor,
                calibration_fresh=True,
                recommended_action="EXECUTE_DIRECT",
                rejection_reason=None,
            )
        elif f_pred >= (floor - 0.20):
            # Redeemable via ZNE or Zero-Noise Extrapolation
            return FidelityAdmissionVerdict(
                admitted=True,
                predicted_fidelity=f_pred,
                fidelity_floor=floor,
                calibration_fresh=True,
                recommended_action="APPLY_ZNE_MITIGATION",
                rejection_reason=f"Predicted fidelity ({f_pred:.3f}) below floor ({floor:.3f}) but redeemable via ZNE error mitigation.",
            )
        else:
            return FidelityAdmissionVerdict(
                admitted=False,
                predicted_fidelity=f_pred,
                fidelity_floor=floor,
                calibration_fresh=True,
                recommended_action="DOWNGRADE_TO_AER_SIMULATOR",
                rejection_reason=f"Predicted fidelity ({f_pred:.3f}) too degraded to produce meaningful scientific signal. Downscaling to simulator.",
            )
