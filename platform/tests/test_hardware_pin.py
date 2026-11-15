"""The vendor id is durable before the bridge waits for a result."""

import json

from kubona.hardware_pin import bind_vendor_pin, persist_vendor_job, pin_path, reset_vendor_pin


def test_pin_is_fsynced_before_return(monkeypatch, tmp_path):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path))
    token = bind_vendor_pin("go-job", "sess")
    try:
        persist_vendor_job("ibm-1")
    finally:
        reset_vendor_pin(token)
    lines = pin_path().read_text(encoding="utf-8").splitlines()
    assert len(lines) == 1
    row = json.loads(lines[0])
    assert row["vendor_job_id"] == "ibm-1"
    assert row["go_job_id"] == "go-job"


def test_unbound_pin_writes_nothing(monkeypatch, tmp_path):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path))
    persist_vendor_job("ibm-2")
    assert not pin_path().exists()
