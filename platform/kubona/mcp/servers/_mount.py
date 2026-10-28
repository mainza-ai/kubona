"""Wire the servers' tools into the gateway registry (shared by tests).

Single mount table (W2.14): `kubona.run.make_gateway` (production auth) and
every test build through this builder, so the tool surface cannot drift
between the dev/test gateway and the production one. `compile_circuit` is
mounted here — it is a real compiler-server tool, not an optional extra.
`world` exposes the Observatory agent-player tools (P5) backed by kubonad.
"""

from __future__ import annotations

from typing import Any

from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.approval import approval_secret_from_env

from . import compiler, control, cost, execution, provenance, world


def build_gateway(*, require_auth: bool = True, auth: Any | None = None,
                  audit_path: str | None = None, ledger: Any | None = None) -> KubonaGateway:
    """Build a gateway with every MCP server mounted.

    `auth=None` keeps the gateway default (dev provider, local/CI smoke
    tests); production passes its OAuth2 issuer via `make_gateway`.
    """
    gw = KubonaGateway(auth=auth, require_auth=require_auth,
                       audit_path=audit_path, ledger=ledger)
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
                          "lower_circuit": compiler.lower_circuit,
                          "compile_circuit": compiler.compile_circuit})
    gw.mount("execution", {
        "submit_job": execution.submit_job,
        "submit_qubo_job": execution.submit_qubo_job,
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
    gw.mount("world", {
        "world_look": world.world_look,
        "world_probe": world.world_probe,
        "world_anchor": world.world_anchor,
        "world_gaze": world.world_gaze,
        "world_summon": world.world_summon,
        "world_handoff": world.world_handoff,
        "world_replay": world.world_replay,
    })
    # Phase-0 money rail: paid dispatches behind human approval. Free-tier
    # simulators auto-allow inside the builders; anything that can spend > 0
    # demands a packet sealed with KUBONA_APPROVAL_SECRET (unconfigured
    # deployments fail closed on paid paths, free paths unaffected).
    gw.require_approval("execution", "submit_job",
                        execution.submit_job_approval_builder,
                        secret=approval_secret_from_env())
    gw.require_approval("execution", "submit_qubo_job",
                        execution.submit_qubo_job_approval_builder,
                        secret=approval_secret_from_env())
    return gw
