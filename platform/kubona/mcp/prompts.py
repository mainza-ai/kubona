"""Reusable MCP Prompts for Kubona guided workflows (Phase 1).

Provides deterministic, cache-friendly multi-step workflow templates so any
MCP client can trigger canonical Kubona protocols (classical anchor first,
cost-governed submission, hardware readiness, provenance audit, failure triage).
"""

from __future__ import annotations

from mcp.server.mcpserver import MCPServer


def register_prompts(server: MCPServer) -> None:
    """Register Kubona's canonical workflow prompts on `server`."""

    @server.prompt(
        name="vqe-with-anchor",
        title="Run Anchor-First VQE Campaign",
        description="Establish a PySCF RHF/FCI classical anchor first, compile and estimate cost, then run VQE on the local simulator and report discrepancy in mHa.",
    )
    def vqe_with_anchor(
        molecule: str = "H2",
        bond_distance_angstrom: str = "0.74",
        basis: str = "sto-3g",
        session_id: str = "",
    ) -> str:
        sid = session_id or "<session_id>"
        return (
            f"Run an anchor-first VQE workflow for {molecule} at R={bond_distance_angstrom} A ({basis}) "
            f"in session {sid}:\n"
            "1. Call `get_classical_anchor` first and record both RHF and FCI energies in Ha. "
            "If the anchor call fails, stop and report the claim as unverified.\n"
            "2. Check `get_session_autonomy` and `session_burn_rate` to verify budget and agentic mode.\n"
            "3. Run `run_experiment` with `kind='vqe'` (or `transpile_circuit` -> `estimate_cost` -> `submit_job` "
            "on `qiskit-aer-simulator` for raw OpenQASM 2.0).\n"
            "4. Compare the measured VQE energy against the FCI anchor in mHa. Under the Rayleigh-Ritz "
            "variational principle, never claim quantum advantage from sub-FCI shot noise."
        )

    @server.prompt(
        name="hardware-readiness-check",
        title="QPU Hardware Readiness Check",
        description="Verify credentials, provider capabilities, budget headroom, calibration, and classical anchor before proposing paid QPU execution.",
    )
    def hardware_readiness_check(session_id: str, provider: str = "ibm_fez") -> str:
        return (
            f"Perform a pre-flight QPU readiness check for session {session_id} targeting `{provider}`:\n"
            "1. Call `list_provider_registry` to confirm the provider is live (not a fail-closed stub).\n"
            "2. Call `session_status` and `session_burn_rate` to confirm remaining ceiling > estimated spend.\n"
            "3. Call `get_session_autonomy` to verify `agentic_locked` is false and check pending approvals.\n"
            "4. Call `estimate_cost` with the exact QASM, provider, and shots (note `heuristic: true`: "
            "estimates inform, ceilings bind).\n"
            "5. Report readiness clearly. Never submit paid QPU jobs without an explicit human approval / "
            "Ed25519 mandate receipt."
        )

    @server.prompt(
        name="cost-governed-submit",
        title="Cost-Governed Circuit Submission",
        description="Compile, estimate cost, check session ceiling, submit with idempotency, and poll until terminal settlement.",
    )
    def cost_governed_submit(
        session_id: str,
        provider: str = "qiskit-aer-simulator",
        shots: str = "1024",
    ) -> str:
        return (
            f"Submit a circuit safely to `{provider}` ({shots} shots) in session {session_id}:\n"
            "1. Validate and canonicalize OpenQASM 2.0 with `transpile_circuit` (or `compile_circuit`).\n"
            "2. Call `estimate_cost` with the canonical QASM, provider, and shots.\n"
            "3. Verify `session_burn_rate` has headroom.\n"
            "4. Call `submit_job`. If a paid backend returns an MRTR approval packet (`request_state`), "
            "pause and present the packet to the operator for approval before re-calling with `approval_response`.\n"
            "5. Poll `get_job_status` until terminal, then fetch `get_result`. Never re-drive a settled job."
        )

    @server.prompt(
        name="audit-session-provenance",
        title="Audit Session Provenance & Receipts",
        description="Inspect hash-chained provenance records, research graph, and spend settlement for a session.",
    )
    def audit_session_provenance(session_id: str) -> str:
        return (
            f"Audit provenance and spend for session {session_id}:\n"
            "1. Call `session_status` and `session_burn_rate` to verify spend vs ceiling.\n"
            "2. Call `query_records` to inspect immutable provenance ledger entries (never invent missing rows).\n"
            "3. Call `get_research_graph` (starting at `since_seq=0`) to inspect claims, hypotheses, and plans.\n"
            "4. Mark every reported fact as **seen** (citing record/job/event IDs) or **unknown**."
        )

    @server.prompt(
        name="debug-failed-job",
        title="Triage Failed or Blocked Job",
        description="Diagnose a failed job, 409 conflict, or blocked delete using Kubona's closed error catalog.",
    )
    def debug_failed_job(job_id: str, session_id: str = "") -> str:
        sid_hint = f" in session {session_id}" if session_id else ""
        return (
            f"Diagnose job `{job_id}`{sid_hint}:\n"
            "1. Read `kubona://contracts/error-catalog` to map the exact `error_code` to its remediation and retryability.\n"
            "2. Call `get_job_status` for the persisted terminal/error state.\n"
            "3. Call `list_loops` to check for stale or `sleeping` loops blocking session deletion (`delete_blocked`).\n"
            "4. Never retry a `409 request_conflict` with the same `request_id` and different payload; mint a fresh id."
        )
