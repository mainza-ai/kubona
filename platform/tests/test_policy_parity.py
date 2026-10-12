"""Cross-rail authorization parity (Python side).

Drives the SAME table as the Go rail (platform/core/pkg/api/
policy_parity_test.go) against kubona.security.policy.PolicyEngine:
same abstract case, same expected verdict. Abstract principal/action
mapping (mirrors the documented scopes):
  operator            -> ("*",) operator
  researcher          -> AGENT_RESEARCHER_SCOPES
  researcher-approved -> researcher + cost:approve (the approval artifact)
  observer            -> read-only subset (no submit/approve/override/write)
  nobody              -> () (no scopes at all)
Tool mapping: tool:teleport -> execution/teleport (unmapped);
tool:read -> execution/get_job_status; tool:submit ->
execution/submit_job; metered -> cost/hold_approval (approve-gated).
"""

import json
from pathlib import Path

from kubona.core.auth import Principal
from kubona.security.policy import AGENT_RESEARCHER_SCOPES, PolicyEngine

TABLE = Path(__file__).resolve().parent / "policy_parity.json"

OBSERVER_SCOPES = (
    "compiler:read", "execution:read", "cost:read",
    "providers:read", "provenance:read", "control:read",
)

PRINCIPALS = {
    "operator": Principal(sub="op", role="operator", scopes=("*",)),
    "researcher": Principal(sub="agent-r", role="agent", scopes=AGENT_RESEARCHER_SCOPES),
    "researcher-approved": Principal(
        sub="agent-r", role="agent",
        scopes=tuple(sorted(set(AGENT_RESEARCHER_SCOPES) | {"cost:approve"})),
    ),
    "observer": Principal(sub="agent-o", role="agent", scopes=OBSERVER_SCOPES),
    "nobody": Principal(sub="anon", role="agent", scopes=()),
}

TOOLS = {
    "tool:teleport": "execution/teleport",
    "tool:read": "execution/get_job_status",
    "tool:submit": "execution/submit_job",
    "metered": "cost/hold_approval",
}


def _verdict(decision):
    return "allow" if decision.allowed else "deny"


def test_policy_parity_table():
    table = json.loads(TABLE.read_text())
    assert table["version"] == 1, "table version bump needs a driver update on both rails"
    engine = PolicyEngine()
    ran = 0
    for case in table["cases"]:
        if "py" not in case["rails"]:
            continue
        principal = PRINCIPALS[case["principal"]]
        tool = TOOLS[case["action"]]
        got = _verdict(engine.decide(principal, tool))
        assert got == case["expect"], (
            f"case {case['id']} ({case['principal']}/{case['action']}): "
            f"Python rail says {got}, table demands {case['expect']} — {case['why']}"
        )
        ran += 1
    assert ran >= 7, f"parity table shrank unexpectedly ({ran} py cases)"
