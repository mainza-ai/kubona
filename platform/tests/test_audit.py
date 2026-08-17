"""Immutable audit: gateway audit entries land in the hash-chained ledger."""

from kubona.mcp.servers._mount import build_gateway
from kubona.provenance import Ledger


def test_audit_written_to_ledger(tmp_path):
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    gw = build_gateway()
    gw._ledger = ledger
    gw.call("compiler", "transpile_circuit",
            {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];"}, token="dev-token")
    gw.call("provenance", "write_record",
            {"kind": "experiment", "payload": {"circuit_hash": "abc"}}, token="dev-token")

    audits = [r for r in ledger if r.kind == "audit"]
    assert len(audits) == 2
    assert audits[0].payload["tool"] == "transpile_circuit"
    assert audits[0].payload["sub"] == "researcher@kubona.local"

    ok, msg = ledger.validate_chain()
    assert ok, msg
    # in-memory mirror still works for the console/gateway consumers
    assert len(gw.audit_log()) == 2
