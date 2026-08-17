"""Wire the four servers' tools into the gateway registry (shared by tests)."""

from __future__ import annotations

from kubona.mcp.gateway import KubonaGateway

from . import compiler, cost, execution, provenance


def build_gateway(*, require_auth: bool = True) -> KubonaGateway:
    gw = KubonaGateway(require_auth=require_auth)
    gw.mount("compiler", {"transpile_circuit": compiler.transpile_circuit})
    gw.mount("execution", {
        "submit_job": execution.submit_job,
        "get_job_status": execution.get_job_status,
        "get_result": execution.get_result,
        "list_provider_registry": execution.list_provider_registry,
    })
    gw.mount("cost", {"estimate_cost": cost.estimate_cost})
    gw.mount("provenance", {"write_record": provenance.write_record, "query_records": provenance.query_records})
    return gw
