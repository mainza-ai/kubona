import json
from pathlib import Path

import pytest
from datetime import datetime, timezone

from kubona.fidelity.refresh import CalibrationError, apply_snapshot


def _doc(tmp_path):
    p = tmp_path / "devices.json"
    p.write_text(json.dumps({"devices": [{
        "name": "ibm-heron", "qubits": 156, "t1_us": 180.0, "t2_us": 140.0,
        "single_qubit_error": 0.0003, "two_qubit_error": 0.0035,
        "readout_error": 0.012, "calibrated_at": "2026-10-01",
        "source": "old", "version": "v1",
    }]}))
    return p


def _snap():
    return {"t1_us": 200.0, "t2_us": 150.0, "single_qubit_error": 0.0002,
            "two_qubit_error": 0.003, "readout_error": 0.01,
            "source": "backend-properties"}


def test_dry_run_does_not_write(tmp_path):
    p = _doc(tmp_path)
    apply_snapshot("ibm-heron", _snap(), contract=p, now=datetime(2026, 10, 27, tzinfo=timezone.utc))
    assert json.loads(p.read_text())["devices"][0]["calibrated_at"] == "2026-10-01"


def test_write_stamps_real_calibration(tmp_path):
    p = _doc(tmp_path)
    target = apply_snapshot("ibm-heron", _snap(), contract=p, write=True,
                            now=datetime(2026, 10, 27, 12, 0, tzinfo=timezone.utc))
    written = json.loads(p.read_text())["devices"][0]
    assert written["calibrated_at"] == "2026-10-27T12:00:00+00:00"
    assert written["t1_us"] == 200.0
    assert written["version"].startswith("live-")
    assert target["source"] == "backend-properties"
    assert not (tmp_path / "devices.json.tmp").exists()


@pytest.mark.parametrize("snap", [
    None,
    {},
    {**_snap(), "t1_us": 0},
    {**_snap(), "t2_us": 500.0},
    {k: v for k, v in _snap().items() if k != "readout_error"},
])
def test_refuses_unmeasured_or_unsound(tmp_path, snap):
    with pytest.raises(CalibrationError):
        apply_snapshot("ibm-heron", snap, contract=_doc(tmp_path))


def test_unknown_device_refuses(tmp_path):
    with pytest.raises(CalibrationError):
        apply_snapshot("nope", _snap(), contract=_doc(tmp_path))


def test_contract_path_honors_env_override(tmp_path, monkeypatch):
    src = Path(__file__).resolve().parents[2] / "contracts" / "fidelity-devices.json"
    target = tmp_path / "fidelity-devices.json"
    target.write_text(src.read_text())
    monkeypatch.setenv("KUBONA_FIDELITY_DEVICES", str(target))
    from kubona.fidelity import refresh
    assert refresh.contract_path() == target
    snap = {"t1_us": 100.0, "t2_us": 90.0, "single_qubit_error": 1e-4,
            "two_qubit_error": 2e-3, "readout_error": 1e-2,
            "source": "test", "calibrated_at": "2026-11-08T00:00:00+00:00"}
    refresh.apply_snapshot("ibm-heron", snap, write=True)
    assert "test" in target.read_text()
