"""Gateway persistent audit export (G11) — audit survives restart."""

from kubona.mcp.gateway import KubonaGateway


def test_audit_export_persists_across_restart(tmp_path):
    path = str(tmp_path / "audit.jsonl")

    gw1 = KubonaGateway(require_auth=False, audit_path=path)
    from kubona.mcp.servers import compiler
    gw1.mount("compiler", {"transpile_circuit": compiler.transpile_circuit})
    gw1.call("compiler", "transpile_circuit",
             {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];"})
    gw1.call("compiler", "transpile_circuit",
             {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nx q[0];"})

    # a fresh gateway (new instance = restart) reading the same audit path
    gw2 = KubonaGateway(require_auth=False, audit_path=path)
    export = gw2.audit_export()
    assert len(export) == 2
    assert {e["tool"] for e in export} == {"transpile_circuit"}
    assert all("sub" in e for e in export)


def test_audit_export_with_ledger(tmp_path):
    from kubona.provenance import Ledger
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    gw = KubonaGateway(require_auth=False, ledger=ledger, audit_path=str(tmp_path / "audit.jsonl"))
    from kubona.mcp.servers import compiler
    gw.mount("compiler", {"transpile_circuit": compiler.transpile_circuit})
    gw.call("compiler", "transpile_circuit", {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nx q[0];"})
    assert any(r.kind == "audit" for r in ledger)
    assert ledger.validate_chain()[0]
