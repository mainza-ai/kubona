"""P1-4 step 1 (freeze, no behavior change): the AgentRegistry
(no-escalation creation path) validation contract.

test_phase6.py pins the happy path + escalation refusal. This file pins
every remaining refusal edge so the consolidation program (Go vs Python
identity home) cannot silently weaken one: empty id/persona/scopes, bad
ceilings, duplicates, unknown creators, caller-stated creator scopes,
child-ceiling exceedance, operator bootstrap without scopes, ghost
disable/link/usable, corrupt-store boot, drifted-row skip.
"""

import json
import math

import pytest

from kubona.agents.registry import Agent, AgentCreationError, AgentRegistry


def _reg(tmp_path, name="agents.json"):
    return AgentRegistry(path=str(tmp_path / name))


def _op(reg, agent_id="a1", scopes=("execution:submit",)):
    return reg.create_agent("operator:root", scopes, agent_id, "researcher",
                            scopes=scopes)


def test_empty_fields_refuse(tmp_path):
    reg = _reg(tmp_path)
    op = ("execution:submit",)
    with pytest.raises(AgentCreationError):
        reg.create_agent("operator:root", op, "", "p", scopes=op)
    with pytest.raises(AgentCreationError):
        reg.create_agent("operator:root", op, "a", "", scopes=op)
    with pytest.raises(AgentCreationError):
        reg.create_agent("operator:root", op, "a", "p", scopes=())
    with pytest.raises(AgentCreationError):
        reg.create_agent("operator:root", op, "a", "p", scopes=("  ",))
    with pytest.raises(AgentCreationError):
        reg.create_agent("operator:root", op, "a", "p", scopes=(123,))


def test_bad_ceilings_refuse(tmp_path):
    reg = _reg(tmp_path)
    op = ("execution:submit",)
    for bad in (0, -1.5, math.nan, math.inf, "10"):
        with pytest.raises(AgentCreationError):
            reg.create_agent("operator:root", op, f"c-{bad}", "p",
                             scopes=op, budget_ceiling_usd=bad)


def test_duplicate_refuses(tmp_path):
    reg = _reg(tmp_path)
    _op(reg)
    with pytest.raises(AgentCreationError, match="already exists"):
        _op(reg)


def test_creator_rules(tmp_path):
    reg = _reg(tmp_path)
    op = ("execution:submit", "cost:read")
    _op(reg, scopes=op)
    # Unknown creator agent refuses.
    with pytest.raises(AgentCreationError, match="unknown"):
        reg.create_agent("ghost-agent", (), "x", "p", scopes=("execution:submit",))
    # Caller-stated scopes for an agent creator refuse (record is the source).
    with pytest.raises(AgentCreationError, match="never the caller|from the creator"):
        reg.create_agent("a1", ("execution:submit",), "y", "p",
                         scopes=("execution:submit",))
    # Child ceiling above the creator's recorded ceiling refuses.
    reg.create_agent("operator:root", op, "capped", "p", scopes=op,
                     budget_ceiling_usd=5.0)
    with pytest.raises(AgentCreationError, match="exceeds creator ceiling"):
        reg.create_agent("capped", (), "big", "p", scopes=op,
                         budget_ceiling_usd=50.0)
    # Operator bootstrap without explicit scopes refuses.
    with pytest.raises(AgentCreationError, match="explicit creator_scopes"):
        reg.create_agent("operator:root", (), "z", "p", scopes=op)


def test_ghost_operations_refuse(tmp_path):
    reg = _reg(tmp_path)
    with pytest.raises(AgentCreationError, match="unknown"):
        reg.disable("ghost")
    with pytest.raises(AgentCreationError, match="unknown"):
        reg.link_workflow("ghost", "wf-1")
    with pytest.raises(AgentCreationError, match="unknown"):
        reg.usable("ghost")
    _op(reg)
    reg.disable("a1")
    with pytest.raises(AgentCreationError, match="disabled"):
        reg.usable("a1")
    with pytest.raises(AgentCreationError, match="unknown|workflow_id"):
        reg.link_workflow("a1", "")


def test_corrupt_store_boots_empty_and_drift_skips(tmp_path):
    p = tmp_path / "agents.json"
    p.write_text("{not json")
    reg = AgentRegistry(path=str(p))
    assert reg.all() == []
    # Dict-shaped (non-list) store also boots empty, never crashes.
    p.write_text(json.dumps({"agent_id": "x"}))
    assert AgentRegistry(path=str(p)).all() == []
    # Drifted rows skip; valid rows load (scopes tuple-ified).
    p.write_text(json.dumps([
        {"agent_id": "good", "persona": "p", "scopes": ["execution:submit"]},
        {"agent_id": "drifted"},
    ]))
    reg2 = AgentRegistry(path=str(p))
    assert [a.agent_id for a in reg2.all()] == ["good"]
    assert isinstance(reg2.get("good").scopes, tuple)
    # Non-dict rows (string/number/null) skip, never crash boot — the
    # pre-fix code raised ValueError out of _load on a string row.
    p.write_text(json.dumps(["not-a-dict", 42, None,
                             {"agent_id": "ok", "persona": "p"}]))
    assert [a.agent_id for a in AgentRegistry(path=str(p)).all()] == ["ok"]


def test_to_dict_roundtrip(tmp_path):
    reg = _reg(tmp_path)
    a = _op(reg, scopes=("execution:submit", "cost:read"))
    d = a.to_dict()
    assert d["scopes"] == ["execution:submit", "cost:read"]
    assert Agent(**{k: v for k, v in d.items()
                    if k in Agent.__dataclass_fields__}).agent_id == "a1"
