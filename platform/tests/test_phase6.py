"""Phase 6: workflows, agent authoring, notebooks, scholarly IO, discovery + the P6 gate."""

import pytest

from kubona.agents import AgentCreationError, AgentRegistry
from kubona.discovery import run_campaign
from kubona.library import AlgorithmLibrary
from kubona.mcp.auth import AuthError
from kubona.mcp.servers._mount import build_gateway
from kubona.provenance import Ledger
from kubona.workflows import WorkflowEngine, WorkflowError, parse_workflow
from kubona.workflows.library import WorkflowLibrary
from kubona.integrations import ground_literature, DATA_SOURCES

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"

VQE_WORKFLOW = {
    "name": "vqe-sweep",
    "steps": [
        {"id": "cost", "tool": "cost/estimate_cost",
         "args": {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 4096}},
        {"id": "run", "tool": "execution/submit_job",
         "args": {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 4096, "seed": 7}},
        {"id": "record", "tool": "provenance/write_record",
         "args": {"kind": "workflow_result", "payload": {"job_id": "{{prev.run.job_id}}"}}},
    ],
}


def test_parse_and_execute_user_workflow(tmp_path):
    gw = build_gateway()
    ledger = Ledger(path=str(tmp_path / "led.jsonl"))
    engine = WorkflowEngine(gw, ledger=ledger)
    wf = parse_workflow(VQE_WORKFLOW)
    assert not wf.requires_approval()  # user-created
    # create a real session (the governed chain needs one)
    sid = gw.call("cost", "create_session",
                  {"name": "wf", "owner": "researcher", "budget_ceiling_usd": 10.0},
                  token="dev-token")["session_id"]
    out = engine.run(wf, token="dev-token", session_id=sid)
    assert out["ok"] and out["steps"] == 3
    assert len(out["step_records"]) == 3
    assert ledger.validate_chain()[0]


def test_agent_created_workflow_requires_approval(tmp_path):
    gw = build_gateway()
    ledger = Ledger(path=str(tmp_path / "led.jsonl"))
    engine = WorkflowEngine(gw, ledger=ledger)
    wf = parse_workflow({**VQE_WORKFLOW, "created_by_agent": True})
    assert wf.requires_approval()
    with pytest.raises(WorkflowError):
        engine.run(wf, token="dev-token", session_id="s1")


def test_workflow_validation_unknown_tool(tmp_path):
    gw = build_gateway()
    bad = parse_workflow({**VQE_WORKFLOW, "steps": [{"id": "x", "tool": "nope/does_not_exist"}]})
    assert WorkflowEngine(gw).validate(bad) == ["nope/does_not_exist"]


def test_workflow_library_saves_and_reuses(tmp_path):
    lib = WorkflowLibrary(path=str(tmp_path / "wf.json"))
    item = lib.save(VQE_WORKFLOW, created_by="researcher")
    assert lib.get(item.workflow_id) is not None
    assert len(lib.all()) == 1


def test_agent_registry_no_escalation(tmp_path):
    reg = AgentRegistry(path=str(tmp_path / "agents.json"))
    reg.register("plain-agent", "researcher", scopes=("execution:submit", "cost:read"))
    # subset ok
    reg.create_as_agent("plain-agent", "sub-agent", "reviewer", scopes=("execution:submit",))
    # escalation rejected (trying to gain cost:approve)
    with pytest.raises(AgentCreationError):
        reg.create_as_agent("plain-agent", "evil-agent", "escalator",
                            scopes=("execution:submit", "cost:approve"))


def test_experiment_report_structured_and_traceable(tmp_path):
    from kubona.reports import build_report

    r = build_report(title="t", objectives="o", results="res")
    r.add_ref("results", "ledger:abc")
    md = r.to_markdown()
    assert "## Objectives" in md and "## Results" in md
    assert r.refs == [("results", "ledger:abc")]


def test_scholarly_grounding_writes_provenance(tmp_path):
    ledger = Ledger(path=str(tmp_path / "led.jsonl"))
    out = ground_literature("vqe", source="mock", ledger=ledger)
    assert len(out["items"]) == 2 and out["license"] and out["freshness"] == 1.0
    lit = [r for r in ledger if r.kind == "literature"]
    assert len(lit) == 1 and lit[0].payload["license"] and lit[0].payload["freshness"] == 1.0


def test_arxiv_offline_never_crashes(tmp_path):
    ledger = Ledger(path=str(tmp_path / "led.jsonl"))
    out = ground_literature("vqe", source="arxiv", ledger=ledger)
    # offline or online, it must return a provenance record, never raise
    assert "items" in out and "ledger_ref" in out
    assert out["ledger_ref"] is not None


# ---- THE PHASE 6 GATE : a lab runs the full discovery loop ----
def test_phase6_gate_full_discovery_loop(tmp_path):
    ledger = Ledger(path=str(tmp_path / "led.jsonl"), tenant="lab-alpha")
    library = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    camp = run_campaign(problem="h2-energetics", hamiltonian="Z0Z1 + 0.5 X0X1",
                        ground_state=-1.118, budget_steps=40, tenant="lab-alpha",
                        **{"ledger": ledger, "library": library, "source": "mock"})

    # every stage ran
    for stage in ("ground", "design", "experiment", "analyze", "report", "archive"):
        assert stage in camp["stages"], stage

    # the report is structured + every claim traces to a ledger record / citation
    report = camp["report"]
    for section in ("objectives", "methods", "protocol", "results", "analysis", "conclusions"):
        assert report.get(section), section
    assert camp["all_claims_traceable"] is True

    # the best result was archived; ledger chain intact; lab-tenant scoped
    assert camp["library_entry_id"] is not None
    assert library.best("vqe_ansatz") is not None
    ok, msg = ledger.validate_chain()
    assert ok, msg
    # the ledger is tenant-scoped to lab-alpha
    assert "lab-alpha" in str(ledger.path)
