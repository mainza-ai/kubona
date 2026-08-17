"""Wire the four servers' tools into the gateway registry (shared by tests)."""

from __future__ import annotations

from kubona.mcp.gateway import KubonaGateway

from . import compiler, control, cost, execution, provenance


def build_gateway(*, require_auth: bool = True) -> KubonaGateway:
    gw = KubonaGateway(require_auth=require_auth)
    gw.mount("control", {
        "register_loop": control.register_loop,
        "step_loop": control.step_loop,
        "list_loops": control.list_loops,
        "loop_status": control.loop_status,
        "pause_loop": control.pause_loop,
        "resume_loop": control.resume_loop,
        "kill_loop": control.kill_loop,
    })
    gw.mount("compiler", {"transpile_circuit": compiler.transpile_circuit,
                          "lower_circuit": compiler.lower_circuit})
    gw.mount("execution", {
        "submit_job": execution.submit_job,
        "get_job_status": execution.get_job_status,
        "get_result": execution.get_result,
        "run_zne_job": execution.run_zne_job,
        "cancel_job": execution.cancel_job,
        "session_status": execution.session_status,
        "close_session": execution.close_session,
        "list_provider_registry": execution.list_provider_registry,
    })
    gw.mount("cost", {
        "create_session": cost.create_session,
        "estimate_cost": cost.estimate_cost,
        "request_approval": cost.request_approval,
        "hold_approval": cost.hold_approval,
        "approve_approval": cost.approve_approval,
        "reject_approval": cost.reject_approval,
        "session_burn_rate": cost.session_burn_rate,
        "quote_payment": cost.quote_payment,
        "verify_payment": cost.verify_payment,
    })
    gw.mount("provenance", {"write_record": provenance.write_record, "query_records": provenance.query_records})
    return gw
