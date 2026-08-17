"""Ledger hash-chain tests."""

from kubona.provenance import Ledger


def test_chain_integrity(tmp_path):
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    r1 = ledger.append("experiment", {"circuit_hash": "abc", "shots": 100})
    r2 = ledger.append("experiment", {"circuit_hash": "def", "shots": 200})
    r3 = ledger.append("job", {"job_id": "j1"})
    ok, msg = ledger.validate_chain()
    assert ok, msg
    assert ledger.count() == 3
    assert r2.prev_hash == r1.record_hash
    assert r3.prev_hash == r2.record_hash


def test_tamper_detected(tmp_path):
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    ledger.append("experiment", {"circuit_hash": "abc"})
    ledger.append("experiment", {"circuit_hash": "def"})
    # tamper: rewrite the file changing the first record's payload
    lines = open(ledger.path).read().splitlines()
    lines[0] = lines[0].replace('"circuit_hash": "abc"', '"circuit_hash": "hacked"')
    open(ledger.path, "w").write("\n".join(lines) + "\n")
    ok, msg = ledger.validate_chain()
    assert not ok
    assert "content hash mismatch" in msg
