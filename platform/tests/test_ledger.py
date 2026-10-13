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


def test_label_defaults_to_file_tenant(tmp_path):
    from kubona.provenance import Ledger
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"), tenant="lab-a")
    r = ledger.append("experiment", {"circuit_hash": "abc"})
    assert r.label == "tenant/lab-a", r
    ok, msg = ledger.validate_chain()
    assert ok, msg


def test_label_tampering_breaks_chain(tmp_path):
    from kubona.provenance import Ledger
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"), tenant="lab-a")
    ledger.append("experiment", {"circuit_hash": "abc"})
    lines = open(ledger.path).read().splitlines()
    assert '"tenant/lab-a"' in lines[0]
    lines[0] = lines[0].replace('"tenant/lab-a"', '"public"')
    open(ledger.path, "w").write("\n".join(lines) + "\n")
    ok, msg = ledger.validate_chain()
    assert not ok
    assert "content hash mismatch" in msg


def test_legacy_rows_still_validate(tmp_path):
    from kubona.provenance import Ledger
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    open(ledger.path, "w").write(
        '{"record_id": "old1", "prev_hash": "' + "0" * 64 + '", '
        '"content_hash": "PLACEHOLDER", "record_hash": "X", "ts": 1.0, '
        '"kind": "experiment", "payload": {"a": 1}}\n')
    import hashlib, json
    payload = {"a": 1}
    content = hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()
    prev = "0" * 64
    record = hashlib.sha256(f"{prev}:{content}".encode()).hexdigest()
    open(ledger.path, "w").write(
        json.dumps({"record_id": "old1", "prev_hash": prev, "content_hash": content,
                    "record_hash": record, "ts": 1.0, "kind": "experiment",
                    "payload": payload}) + "\n")
    ok, msg = ledger.validate_chain()
    assert ok, msg


def test_readable_and_derive_label():
    from kubona.provenance import derive_label, readable
    from kubona.provenance.ledger import LedgerRecord
    mk = lambda label: LedgerRecord("i", "p", "c", "r", 0.0, "k", {}, label)
    assert readable(mk("public"), None) is True
    assert readable(mk("tenant/lab-a"), "lab-a") is True
    assert readable(mk("tenant/lab-a"), "lab-b") is False
    assert readable(mk("tenant/lab-a"), None) is False
    assert readable(mk(""), "lab-b") is True  # legacy rows: file-shard scoping
    assert derive_label(["public", "public"], "lab-a") == "public"
    assert derive_label(["public", "tenant/lab-a"], "lab-a") == "tenant/lab-a"


def _append_n(path, n, tag):
    # Module-level import inside the child: multiprocessing spawn-safe.
    from kubona.provenance import Ledger
    ledger = Ledger(path=path)
    for i in range(n):
        ledger.append("experiment", {"circuit_hash": f"{tag}-{i}", "shots": 8})


def test_concurrent_process_appends_keep_chain(tmp_path):
    """Two processes appending at once must not fork the chain (2026-10-12:
    concurrent suites produced identical-prev_hash twins in the shared
    ledger). flock serializes tail-read + append across processes."""
    import multiprocessing as mp
    path = str(tmp_path / "ledger.jsonl")
    procs = [mp.Process(target=_append_n, args=(path, 25, f"p{k}")) for k in range(2)]
    for p in procs:
        p.start()
    for p in procs:
        p.join(60)
    assert all(p.exitcode == 0 for p in procs)
    from kubona.provenance import Ledger
    ok, msg = Ledger(path=path).validate_chain()
    assert ok, msg
