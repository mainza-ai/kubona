"""Simulator-backend parity contract (Phase-2 credibility hygiene).

Three simulator-class lists can rot apart unnoticed:

1. Bridge free-vs-paid gate: ``_FREE_SIMULATOR_BACKENDS`` in
   ``platform/bridge/server.py`` (decides whether QuantumSubmit demands a
   verified spend mandate).
2. Go admission gate: ``simulatorBackends`` in
   ``platform/core/pkg/jobs/admission.go`` (decides whether SubmitJob
   demands a spend-mandate receipt).
3. Python jurisdiction gate: ``_LOCAL_ONLY_PROVIDERS`` in
   ``platform/kubona/mcp/servers/execution.py`` (the ONLY provider names
   allowed to claim jurisdiction ``"none"`` and skip the export gate).

Lists 1 and 2 are the SAME policy (zero-cost local execution) on two
rails, so they must name the same backends after normalization. List 3 is
deliberately narrower (see ``EXECUTION_ALLOWLIST`` reason below), so it is
pinned exactly AND required to be a subset of the free-vs-paid set: a name
that may skip export control must never be a name the money gate bills.

The test parses the three source files as text (stdlib only, no product
imports — Go source cannot be imported at all, and importing the bridge
server would drag gRPC/proto/chemistry). String literals are extracted
from the exact set/map literal, so moving or renaming the list breaks
this test loudly instead of silently dropping coverage.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
BRIDGE = REPO / "platform" / "bridge" / "server.py"
GO_ADMISSION = REPO / "platform" / "core" / "pkg" / "jobs" / "admission.go"
EXECUTION = REPO / "platform" / "kubona" / "mcp" / "servers" / "execution.py"


def _extract_frozenset(source: str, name: str) -> set[str]:
    """Extract string literals from ``NAME = frozenset({...})``."""
    m = re.search(
        re.escape(name) + r"\s*=\s*frozenset\(\{(?P<body>.*?)\}\)",
        source,
        re.DOTALL,
    )
    assert m, f"{name} frozenset literal not found: moved or renamed?"
    return set(re.findall(r'"([^"]*)"', m.group("body")))


def _extract_go_bool_map(source: str, name: str) -> set[str]:
    """Extract keys from ``NAME = map[string]bool{...}``."""
    m = re.search(
        re.escape(name) + r"\s*=\s*map\[string\]bool\{(?P<body>.*?)\}",
        source,
        re.DOTALL,
    )
    assert m, f"{name} map literal not found: moved or renamed?"
    # Keys are the quoted strings before each colon; values are `true`.
    return set(re.findall(r'"([^"]*)"\s*:', m.group("body")))


def normalize(backend: str) -> str:
    """Canonical simulator-backend spelling for cross-list comparison.

    Mirrors what the product gates actually do (both strip whitespace and
    lowercase: bridge ``_is_paid_backend``, Go ``isSimulatorBackend``) and
    additionally folds ``_`` to ``-`` so alias spellings
    (``local_simulator`` vs ``local-simulator``) compare equal. The product
    gates list both alias spellings explicitly because they do NOT fold
    separators at runtime — this folding exists only for comparison.
    """
    return backend.strip().lower().replace("_", "-")


# INTENTIONAL DIFFERENCE, allowlisted with reason: the jurisdiction gate
# matches exact provider-catalog names (``load_provider(provider)`` keys),
# so it lists only canonical simulator provider IDs. Backend aliases
# (``aer``, ``statevector``, ``gpu-simulator``, ``_``-variants) and the
# empty-string default are admission-layer spellings handled by the
# free-vs-paid lists — they must never claim jurisdiction "none" under an
# alias the provider catalog does not know. If a new canonical simulator
# provider is added, list it here AND in the free-vs-paid sets.
EXECUTION_ALLOWLIST = frozenset({"local-simulator", "qiskit-aer-simulator"})


def test_bridge_and_go_name_same_simulators():
    bridge = _extract_frozenset(BRIDGE.read_text(), "_FREE_SIMULATOR_BACKENDS")
    assert len(bridge) >= 2, "bridge set parse looks empty: literal shape changed?"
    go = _extract_go_bool_map(GO_ADMISSION.read_text(), "simulatorBackends")
    assert len(go) >= 2, "Go map parse looks empty: literal shape changed?"
    bridge_n = {normalize(b) for b in bridge}
    go_n = {normalize(b) for b in go}
    assert bridge_n == go_n, (
        "bridge free-vs-paid set and Go admission set diverged:\n"
        f"  bridge-only: {sorted(bridge_n - go_n)}\n"
        f"  go-only:     {sorted(go_n - bridge_n)}\n"
        "One rail would bill (or comp) a backend the other treats as free."
    )


def test_execution_local_only_is_pinned_allowlist_subset():
    raw = _extract_frozenset(EXECUTION.read_text(), "_LOCAL_ONLY_PROVIDERS")
    assert raw == EXECUTION_ALLOWLIST, (
        f"_LOCAL_ONLY_PROVIDERS changed to {sorted(raw)}: intentional "
        "narrowing is allowlisted, anything else must update "
        "EXECUTION_ALLOWLIST in this test with a written reason."
    )
    bridge = _extract_frozenset(BRIDGE.read_text(), "_FREE_SIMULATOR_BACKENDS")
    bridge_n = {normalize(b) for b in bridge}
    exec_n = {normalize(b) for b in raw}
    assert exec_n <= bridge_n, (
        f"jurisdiction-bypass names not in the free set: {sorted(exec_n - bridge_n)} — "
        "a backend that skips export control must never be billable."
    )
