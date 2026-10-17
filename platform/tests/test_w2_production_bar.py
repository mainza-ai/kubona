"""W2 production-bar regression tests (W2.6, W2.7, W2.8, W2.13–W2.18, W2.21).

Every fix in the W2 wave carries a regression test here. Tests strengthen,
never weaken: where an old test asserted silent/fictional behavior it was
updated to the corrected behavior (noted inline); nothing here asserts less.
"""

import asyncio
import hashlib
import inspect
import json
import logging
import threading
import time

import pytest

# ---------------------------------------------------------------------------
# W2.6: workflow engine — budget ceiling binds, fail-closed render, dynamic
# session discovery, compensations, status lock; library identity + versions.
# ---------------------------------------------------------------------------

from kubona.mcp.gateway import KubonaGateway
from kubona.provenance import Ledger
from kubona.workflows import WorkflowEngine, WorkflowError, parse_workflow
from kubona.workflows.engine import (
    _render_strict,
    discover_session_id_tools,
)
from kubona.workflows.library import WorkflowLibrary


def _engine(gw=None, tmp_path=None, **kw):
    gw = gw or KubonaGateway(require_auth=False)
    lock = str(tmp_path / "w.lock") if tmp_path is not None else None
    return WorkflowEngine(gw, ledger=None, lock_path=lock, **kw)


def test_template_missing_key_refuses_fail_closed():
    with pytest.raises(WorkflowError, match="does not resolve"):
        _render_strict({"x": "{{input.typo}}"}, {"input": {}, "prev": {}, "session_id": ""})
    with pytest.raises(WorkflowError, match="prev.run.job_id"):
        _render_strict({"x": "{{prev.run.job_id}}"}, {"input": {}, "prev": {}, "session_id": ""})
    # present keys still render (dicts as JSON, scalars raw)
    out = _render_strict({"a": "{{input.q}}", "b": "{{prev.s}}"},
                         {"input": {"q": 3}, "prev": {"s": {"k": 1}}, "session_id": ""})
    assert out == {"a": "3", "b": '{"k": 1}'}


def test_workflow_run_refuses_missing_template_key(tmp_path):
    calls = []

    def boom(**kw):
        calls.append(kw)
        return {"ok": True}

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"boom": boom})
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "steps": [
        {"id": "a", "tool": "t/boom", "args": {"x": "{{input.nope}}"}}]})
    out = eng.run(wf, token=None, inputs={})
    assert out["ok"] is False and "does not resolve" in out["error"]
    assert calls == [] and wf.status == "failed"


def test_workflow_dry_run_marks_unresolved_explicitly(tmp_path):
    gw = KubonaGateway(require_auth=False)
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "steps": [
        {"id": "a", "tool": "cost/estimate_cost", "args": {}},
        {"id": "b", "tool": "cost/estimate_cost",
         "args": {"x": "{{prev.a.price}}"}}]})
    out = eng.run(wf, token=None, dry_run=True)
    assert out["ok"] is True
    assert out["unresolved"] == ["prev.a.price"]
    assert out["steps"][1]["args"] == {"x": "{{UNRESOLVED:prev.a.price}}"}
    assert wf.status == "draft"


def test_workflow_budget_ceiling_binds_declared_cost(tmp_path):
    calls = []

    def costly(**kw):
        calls.append(kw)
        return {"ok": True}

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"costly": costly})
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "budget_ceiling_usd": 10.0, "steps": [
        {"id": "a", "tool": "t/costly", "args": {}, "cost_usd": 11.0}]})
    out = eng.run(wf, token=None)
    assert out["ok"] is False and "breach" in out["error"]
    assert calls == []  # refused BEFORE the call, like record_spend


def test_workflow_budget_ceiling_accumulates_reported_spend(tmp_path):
    def spend6(**kw):
        return {"ok": True, "cost_usd": 6.0}

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"spend6": spend6})
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "budget_ceiling_usd": 10.0, "steps": [
        {"id": "a", "tool": "t/spend6", "args": {}},
        {"id": "b", "tool": "t/spend6", "args": {}}]})
    out = eng.run(wf, token=None)
    assert out["ok"] is False and "breach" in out["error"]
    assert out["step"] == "b" and out["spent_usd"] == pytest.approx(12.0)
    assert wf.status == "failed"


def test_workflow_no_ceiling_means_no_binding(tmp_path):
    def spend(**kw):
        return {"ok": True, "cost_usd": 999.0}

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"spend": spend})
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "steps": [{"id": "a", "tool": "t/spend", "args": {}}]})
    out = eng.run(wf, token=None)
    assert out["ok"] is True and out["spent_usd"] == pytest.approx(999.0)


def test_session_id_discovery_is_dynamic(tmp_path):
    seen = {}

    def session_ping(session_id=None, **kw):
        seen["ping"] = session_id
        return {"ok": True}

    def needs_session(session_id=None, **kw):
        seen["needs"] = session_id
        return {"ok": True}

    def plain(**kw):
        seen["plain"] = kw
        return {"ok": True}

    gw = KubonaGateway(require_auth=False)
    gw.mount("misc", {"session_ping": session_ping,
                       "needs_session": needs_session, "plain": plain})
    # dynamic discovery picks up session_* names AND session_id params with
    # no allowlist edit
    found = discover_session_id_tools(gw)
    assert "misc/session_ping" in found and "misc/needs_session" in found
    assert "misc/plain" not in found
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "steps": [
        {"id": "a", "tool": "misc/session_ping", "args": {}},
        {"id": "b", "tool": "misc/needs_session", "args": {}},
        {"id": "c", "tool": "misc/plain", "args": {}}]})
    out = eng.run(wf, token=None, session_id="sess-1")
    assert out["ok"] is True
    assert seen["ping"] == "sess-1" and seen["needs"] == "sess-1"
    assert "session_id" not in seen["plain"]
    # explicit registry override replaces discovery
    eng2 = _engine(gw, tmp_path, session_id_tools={"misc/plain"})
    seen.clear()
    out = eng2.run(parse_workflow({"name": "w2", "steps": [
        {"id": "c", "tool": "misc/plain", "args": {}}]}), token=None, session_id="s")
    assert seen["plain"].get("session_id") == "s"


def test_compensations_run_and_record_on_failure(tmp_path):
    ledger = Ledger(path=str(tmp_path / "led.jsonl"))

    def fail(**kw):
        raise RuntimeError("main step blew up")

    def compensate(reason=None, **kw):
        return {"compensated": True, "reason": reason}

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"fail": fail, "compensate": compensate})
    eng = WorkflowEngine(gw, ledger=ledger, lock_path=str(tmp_path / "w.lock"))
    wf = parse_workflow({"name": "w",
                         "steps": [{"id": "a", "tool": "t/fail", "args": {}}],
                         "on_failure": [{"id": "c1", "tool": "t/compensate",
                                         "args": {"reason": "cleanup"}}]})
    out = eng.run(wf, token=None)
    assert out["ok"] is False and out["error"] == "main step blew up"
    assert out["compensations"] == [{"id": "c1", "ok": True,
                                     "result": {"compensated": True, "reason": "cleanup"}}]
    kinds = [r.kind for r in ledger.query()]
    assert "workflow_compensation" in kinds


def test_failing_compensation_never_masks_original_error(tmp_path):
    def fail(**kw):
        raise RuntimeError("original")

    def badcomp(**kw):
        raise RuntimeError("compensation also failed")

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"fail": fail, "badcomp": badcomp})
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w",
                         "steps": [{"id": "a", "tool": "t/fail", "args": {}}],
                         "on_failure": [{"id": "c1", "tool": "t/badcomp", "args": {}}]})
    out = eng.run(wf, token=None)
    assert out["ok"] is False and out["error"] == "original"
    assert out["compensations"][0]["ok"] is False


def test_status_guard_refuses_concurrent_run(tmp_path):
    entered = threading.Event()
    release = threading.Event()

    def blocking(**kw):
        entered.set()
        assert release.wait(timeout=10)
        return {"ok": True}

    gw = KubonaGateway(require_auth=False)
    gw.mount("t", {"blocking": blocking})
    eng = _engine(gw, tmp_path)
    wf = parse_workflow({"name": "w", "steps": [{"id": "a", "tool": "t/blocking", "args": {}}]})
    t = threading.Thread(target=eng.run, args=(wf,), kwargs={"token": None})
    t.start()
    assert entered.wait(timeout=10)
    with pytest.raises(WorkflowError, match="already running"):
        eng.run(wf, token=None)
    release.set()
    t.join(timeout=10)
    assert wf.status == "completed"


def test_library_preserves_identity_and_bumps_version(tmp_path):
    lib = WorkflowLibrary(path=str(tmp_path / "wf.json"))
    src = {"name": "w", "steps": [{"id": "a", "tool": "cost/estimate_cost", "args": {}}]}
    item = lib.save(src, created_by="researcher")
    assert item.version == 1 and item.supersedes is None
    # to_workflow keeps the library id (no forked uuid)
    wf = item.to_workflow()
    assert wf.workflow_id == item.workflow_id
    # re-saving the same id bumps version in place (no twin record)
    item2 = lib.save({**src, "workflow_id": item.workflow_id}, created_by="researcher")
    assert item2.workflow_id == item.workflow_id and item2.version == 2
    assert len(lib.all()) == 1
    # from_dict round-trips the id
    assert parse_workflow({**src, "workflow_id": "abc123"}).workflow_id == "abc123"
    # atomic write: the file is always whole JSON
    assert json.loads((tmp_path / "wf.json").read_text())[0]["version"] == 2
    # reload preserves versions
    assert WorkflowLibrary(path=str(tmp_path / "wf.json")).get(item.workflow_id).version == 2


# ---------------------------------------------------------------------------
# W2.7: secrets slug parity, gateway mount-merge + loud audit, empty revoke.
# ---------------------------------------------------------------------------

from kubona.secrets import OpenBaoSecretSource, provider_slug
from kubona.session import tenant_slug


def test_provider_slug_unified_with_tenant_rule():
    assert provider_slug("ibm-qiskit") == "ibm-qiskit"
    assert provider_slug("IBM-Qiskit") == "ibm-qiskit"
    assert provider_slug("quantinuum") == tenant_slug("quantinuum").lower()
    with pytest.raises(ValueError):
        provider_slug("...")
    with pytest.raises(ValueError):
        provider_slug("x" * 65)


def test_gateway_mount_merge_and_conflict(tmp_path):
    def fn1(**kw):
        return {"v": 1}

    def fn2(**kw):
        return {"v": 2}

    gw = KubonaGateway(require_auth=False)
    gw.mount("s", {"t": fn1})
    gw.mount("s", {"t": fn1})  # identical re-mount: idempotent
    gw.mount("s", {"u": fn2})  # merge into existing server
    assert gw.call("s", "u", {}, token=None) == {"v": 2}
    with pytest.raises(ValueError, match="mount conflict"):
        gw.mount("s", {"t": fn2})  # silent overwrite refused
    assert gw.call("s", "t", {}, token=None) == {"v": 1}  # original intact


def test_gateway_audit_failure_loud_not_silent(tmp_path, caplog):
    def ok(**kw):
        return {"ok": True}

    gw = KubonaGateway(require_auth=False, audit_path=str(tmp_path))  # a dir: unwritable
    gw.mount("s", {"ok": ok})
    with caplog.at_level(logging.WARNING, logger="kubona.mcp.gateway"):
        out = gw.call("s", "ok", {}, token=None)
    assert out == {"ok": True}  # call path unaffected
    assert gw.audit_failures == 1
    assert "audit export failed" in caplog.text


def test_gateway_ledger_failure_loud_not_silent(tmp_path):
    class BoomLedger:
        def append(self, *a, **k):
            raise IOError("disk on fire")

    def ok(**kw):
        return {"ok": True}

    gw = KubonaGateway(require_auth=False, ledger=BoomLedger())
    gw.mount("s", {"ok": ok})
    assert gw.call("s", "ok", {}, token=None) == {"ok": True}
    assert gw.audit_failures == 1


def test_openbao_empty_revoke_refuses_without_network():
    src = OpenBaoSecretSource(addr="http://127.0.0.1:1", token="x")
    with pytest.raises(ValueError, match="lease id"):
        src.revoke_lease("")


# ---------------------------------------------------------------------------
# W2.8: tiered storage — read-only reads, substring honesty, bounded dedup.
# ---------------------------------------------------------------------------

from kubona.memory.tiered_storage import MAX_STRATEGIES, TieredStorageManager
from kubona.science.memory import KnowledgeEntity, KnowledgeRelation


def test_tier0_reads_are_read_only_with_explicit_touch(tmp_path):
    mgr = TieredStorageManager(tmp_dir=tmp_path / "t0")
    mgr.put_tier0("k", {"v": 1})
    assert mgr.get_tier0("k") == {"v": 1}
    rec = mgr._tier0_cache["k"]
    assert rec.access_count == 0 and rec.last_accessed_at == 0.0  # read touched nothing
    assert mgr.get_tier0("k") == {"v": 1}
    assert rec.access_count == 0
    assert mgr.touch_tier0("k") is True
    assert rec.access_count == 1 and rec.last_accessed_at > 0.0
    assert mgr.touch_tier0("missing") is False
    assert mgr.get_tier0("missing") is None


def test_refutation_search_is_honest_substring(tmp_path):
    mgr = TieredStorageManager(tmp_dir=tmp_path / "t0")
    mgr.index_entity(KnowledgeEntity(entity_id="n1", entity_type="negative_certificate",
                                     name="Barren Plateau in deep VQE",
                                     properties={"harness": "vqe"}))
    mgr.index_entity(KnowledgeEntity(entity_id="m1", entity_type="molecule",
                                     name="Barren Plateau"))
    hits = mgr.search_refutations_substring("barren plateau")
    assert [e.entity_id for e in hits] == ["n1"]  # substring, certs only
    assert mgr.check_refutation("barren plateau") == hits  # alias agrees
    assert mgr.graph.search_substring("vqe") == hits
    assert mgr.graph.find_negative_certificates("vqe") == hits  # alias agrees


def test_strategy_bank_dedups_and_bounds(tmp_path):
    mgr = TieredStorageManager(tmp_dir=tmp_path / "t0")
    kw = dict(domain="spin", heuristic_name="h", reasoning_trace_summary="try x",
              success_rate=0.5)
    mgr.record_strategy("s1", **kw)
    mgr.record_strategy("s2", **kw)  # same content, new id -> dedup, no twin
    assert len(mgr._tier3_strategy_bank) == 1
    entry = mgr._tier3_strategy_bank["s1"]
    assert entry["update_count"] == 1 and entry["success_rate"] == 0.5
    with pytest.raises(ValueError):
        mgr.record_strategy("bad", domain="d", heuristic_name="h",
                            reasoning_trace_summary="s", success_rate=2.0)
    for i in range(MAX_STRATEGIES + 10):
        mgr.record_strategy(f"s{i}", domain="spin", heuristic_name=f"h{i}",
                            reasoning_trace_summary=f"trace {i}",
                            success_rate=(i % 100) / 100.0)
    assert len(mgr._tier3_strategy_bank) == MAX_STRATEGIES  # bounded
    assert mgr.query_strategies("spin")  # query still works over the bound


# ---------------------------------------------------------------------------
# W2.13: dead code verified gone; serve/serve-gateway boot-proof.
# ---------------------------------------------------------------------------

def test_collective_dead_types_deleted_skeptic_documented():
    import kubona.agents.collective as collective

    assert not hasattr(collective, "RoleType")  # deleted: zero callers
    assert not hasattr(collective, "AgentRoleManifest")  # deleted: zero callers
    # the skeptic itself is retained as a TESTED reference with no production
    # caller — behavior pinned here, wiring explicitly pending
    verdict = collective.DisconfirmationOfficer().challenge_finding(
        experiment_energy=-1.0, classical_anchor_energy=-1.1, qubits=2)
    assert verdict.is_falsified is True and verdict.classical_shortcut_found is True
    calm = collective.DisconfirmationOfficer().challenge_finding(
        experiment_energy=-1.137, classical_anchor_energy=-1.1375, qubits=8)
    assert calm.is_falsified is False


def test_model_router_has_no_dead_audio_branch():
    from kubona.mesh.model_router import HybridModelRouter, MultimodalItem, MultimodalPayload, Modality

    assert "needs_audio =" not in inspect.getsource(HybridModelRouter.route)
    assert "needs_audio=" not in inspect.getsource(HybridModelRouter.route)
    router = HybridModelRouter()
    decision = router.route(MultimodalPayload(
        prompt="p", items=[MultimodalItem(modality=Modality.AUDIO, data="x")]))
    assert decision.is_local is True  # audio rides the default local pick


def test_llm_registry_has_no_stale_cache():
    from kubona.llm.registry import LocalModelRegistry

    reg = LocalModelRegistry(ports=[])
    assert not hasattr(reg, "_cache")
    assert reg.discover()["summary"] == {}
    assert not hasattr(reg, "_cache")


def test_pyquil_qcs_seam_has_no_dead_imports():
    import kubona.providers.pyquil_provider as pq
    from kubona.providers.base import ProviderError

    src = inspect.getsource(pq.PyQuilProvider._submit_via_qcs)
    imports = [line for line in src.splitlines()
               if line.strip().startswith(("import ", "from "))]
    assert imports == []  # the seam imports nothing: no dead probes
    with pytest.raises(ProviderError, match="QCS endpoint"):
        pq.PyQuilProvider.__new__(pq.PyQuilProvider)._submit_via_qcs(None, 1, "tok")


def test_cutting_and_fidelity_dead_imports_gone():
    import pathlib
    root = pathlib.Path(__file__).resolve().parent.parent / "kubona"
    wire_src = (root / "cutting" / "wire.py").read_text()
    assert "\nimport math\n" not in wire_src
    import kubona.cutting.wire as wire

    assert wire.plan_cut  # module still imports and works
    fidelity_src = (root / "fidelity" / "accounting.py").read_text()
    typing_lines = [line for line in fidelity_src.splitlines()
                    if line.startswith("from typing import")]
    assert typing_lines and all("List" not in line for line in typing_lines)


def test_run_serve_threads_host_port(monkeypatch):
    import kubona.run as run
    import kubona.console_api as console_api

    seen = {}
    monkeypatch.setattr(console_api, "main", lambda host="x", port=None: seen.update(host=host, port=port))
    run.serve(host="0.0.0.0", port=9999)
    assert seen == {"host": "0.0.0.0", "port": 9999}
    run.serve()
    assert seen == {"host": "127.0.0.1", "port": None}


def test_run_serve_gateway_threads_host_port(monkeypatch, tmp_path):
    import os
    import kubona.run as run
    import uvicorn

    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    monkeypatch.delenv("KUBONA_GATEWAY_PORT", raising=False)
    captured = {}

    class FakeServer:
        def __init__(self, config):
            captured["host"] = config.host
            captured["port"] = config.port

        def run(self):
            captured["ran"] = True

    monkeypatch.setattr(uvicorn, "Server", FakeServer)
    run.serve_gateway(host="1.2.3.4", port=8129)
    assert captured == {"host": "1.2.3.4", "port": 8129, "ran": True}
    monkeypatch.setenv("KUBONA_GATEWAY_PORT", "8131")
    run.serve_gateway()
    assert (captured["host"], captured["port"]) == ("127.0.0.1", 8131)


def test_run_single_gateway_builder(monkeypatch, tmp_path):
    import kubona.run as run
    from kubona.mcp.servers._mount import build_gateway

    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    prod = run.make_gateway(require_auth=False)
    dev = build_gateway(require_auth=False)
    prod_tools = {server: sorted(tools) for server, tools in prod._registry.items()}
    dev_tools = {server: sorted(tools) for server, tools in dev._registry.items()}
    assert prod_tools == dev_tools  # one table: production cannot drift
    assert "compile_circuit" in prod_tools["compiler"]  # mounted, not forgotten
    assert "gw.mount" not in inspect.getsource(run)  # no second table in run.py


# ---------------------------------------------------------------------------
# W2.15: sandbox — every bypass refused, env scrubbed, output capped.
# ---------------------------------------------------------------------------

from kubona.sandbox import SecurityPolicyViolation, WorkloadSandbox
from kubona.sandbox.execution import MAX_OUTPUT_BYTES, scrubbed_env


@pytest.mark.parametrize("code", [
    "import os",
    "import sys",
    "import subprocess",
    "import kubona",
    "import kubona.session",
    "from kubona import session",
    "from os import system",
    "import importlib",
    "__import__('os')",
    "importlib.import_module('os')",
    "getattr(__builtins__, 'eval')",
    "x = lambda m: getattr(m, 'system')",
    "y = obj.__class__",
    "y = obj.__mro__",
    "y = obj.__subclasses__()",
    "y = obj.__dict__",
    "y = func.__globals__",
    "eval('1')",
    "exec('x=1')",
    "open('/etc/passwd')",
    "compile('1', 's', 'eval')",
    "g = globals()",
    "d = obj.__class__.__mro__",
    "from math import __dict__",
])
def test_sandbox_refuses_every_bypass(code):
    with pytest.raises(SecurityPolicyViolation):
        WorkloadSandbox().validate_ast(code)


def test_sandbox_allows_plain_science_code():
    sb = WorkloadSandbox()
    sb.validate_ast("import math\nimport numpy as np\nx = math.sin(0.5) + np.pi")
    sb.validate_ast("from collections import Counter\nc = Counter([1, 1, 2])")


def test_sandbox_env_scrub_drops_secrets(monkeypatch):
    monkeypatch.setenv("KUBONA_W2_SENTINEL", "topsecret")
    monkeypatch.setenv("BAO_TOKEN", "topsecret")
    monkeypatch.setenv("PATH", "/usr/bin:/bin")
    env = scrubbed_env()
    assert "KUBONA_W2_SENTINEL" not in env and "BAO_TOKEN" not in env
    assert env.get("PATH") == "/usr/bin:/bin"
    src = inspect.getsource(WorkloadSandbox.execute_script)
    assert "env=scrubbed_env()" in src  # the child really runs scrubbed


def test_sandbox_caps_output():
    sb = WorkloadSandbox()
    res = sb.execute_script("print('x' * 1000000)")
    assert res.success is True and res.truncated is True
    assert len(res.stdout.encode()) <= MAX_OUTPUT_BYTES + 64


def test_sandbox_uses_owned_tmpdir_not_delete_false():
    import pathlib
    root = pathlib.Path(__file__).resolve().parent.parent / "kubona"
    src = (root / "sandbox" / "execution.py").read_text()
    assert "TemporaryDirectory" in src and "NamedTemporaryFile(" not in src
    assert "kubona" not in WorkloadSandbox().allowed_modules


# ---------------------------------------------------------------------------
# W2.16: hub downloader — sanitize, hash pin, byte gate, typed refusals.
# ---------------------------------------------------------------------------

import httpx as _httpx  # noqa: F401  (patched per-test below)

from kubona.mesh.hf_hub import (
    HFDownloadManager,
    HFHubError,
    HuggingFaceHubClient,
    _infer_quant_type,
    sanitize_filename,
    sanitize_repo_id,
)


def test_quant_unknown_and_safetensors_labeled():
    assert _infer_quant_type("llama-q4_k_m.gguf") == "Q4_K_M"
    assert _infer_quant_type("model-fp16.safetensors") == "FP16"
    assert _infer_quant_type("model.safetensors") == "SAFETENSORS"
    assert _infer_quant_type("model.bin") == "UNKNOWN"
    assert _infer_quant_type("weights.gguf") == "UNKNOWN"  # no confident default


def test_filename_and_repo_sanitized(tmp_path):
    dest = tmp_path / "store"
    dest.mkdir()
    assert sanitize_filename("model-q4_k_m.gguf", dest) == "model-q4_k_m.gguf"
    for bad in ("../evil.gguf", "a/b.gguf", "a\\b.gguf", "/abs.gguf", "", ".", ".."):
        with pytest.raises(HFHubError):
            sanitize_filename(bad, dest)
    assert sanitize_repo_id("owner/name") == "owner/name"
    assert sanitize_repo_id("my-org/model-1") == "my-org/model-1"  # dots/dashes legal
    for bad in ("../x", "a/b/c", "", "owner/", "/etc/passwd", "noslash", "a..b"):
        with pytest.raises(HFHubError):
            sanitize_repo_id(bad)


class _FakeStreamResp:
    def __init__(self, status=200, headers=None, chunks=(), text=""):
        self.status_code = status
        self.headers = dict(headers or {})
        self._chunks = list(chunks)
        self.text = text

    async def __aenter__(self):
        return self

    async def __aexit__(self, *a):
        return False

    async def aiter_bytes(self, chunk_size=1):
        for c in self._chunks:
            yield c


class _FakeClient:
    handler = None  # set per test: fn(kind, url, **kw) -> resp

    def __init__(self, *a, **k):
        pass

    async def __aenter__(self):
        return self

    async def __aexit__(self, *a):
        return False

    def stream(self, method, url):
        return self.handler("stream", url)

    async def get(self, url, params=None):
        return self.handler("get", url)


@pytest.fixture()
def fake_hub(monkeypatch):
    def install(handler):
        _FakeClient.handler = staticmethod(handler)
        monkeypatch.setattr("kubona.mesh.hf_hub.httpx.AsyncClient", _FakeClient)
    return install


@pytest.mark.asyncio
async def test_hub_non200_raises_typed_refusal(fake_hub):
    def handler(kind, url):
        return _FakeStreamResp(status=500, text="boom")
    fake_hub(handler)
    with pytest.raises(HFHubError, match="HTTP 500"):
        await HuggingFaceHubClient().search_models("x")
    with pytest.raises(HFHubError, match="HTTP 500"):
        await HuggingFaceHubClient().get_model_details("a/b")


@pytest.mark.asyncio
async def test_download_refuses_unknown_quant_and_missing_pin(tmp_path, fake_hub):
    fake_hub(lambda kind, url: _FakeStreamResp())
    mgr = HFDownloadManager(models_dir=tmp_path)
    with pytest.raises(HFHubError, match="UNKNOWN"):
        await mgr.start_download("owner/repo", "model.bin", expected_sha256="0" * 64)
    with pytest.raises(HFHubError, match="expected_sha256"):
        await mgr.start_download("o/r", "m-q4_k_m.gguf", expected_sha256="")
    with pytest.raises(HFHubError, match="traversal|separators|repo id"):
        await mgr.start_download("../../evil", "m-q4_k_m.gguf", expected_sha256="0" * 64)


@pytest.mark.asyncio
async def test_download_success_verifies_hash_before_rename(tmp_path, fake_hub):
    body = b"hello-weights"
    digest = hashlib.sha256(body).hexdigest()

    def handler(kind, url):
        return _FakeStreamResp(status=200,
                               headers={"Content-Length": str(len(body))},
                               chunks=[body])
    fake_hub(handler)
    mgr = HFDownloadManager(models_dir=tmp_path)
    task = await mgr.start_download("owner/repo", "model-q4_k_m.gguf",
                                    expected_sha256=digest)
    done = await mgr.wait_for(task.task_id, timeout=10)
    assert done.status == "completed" and done.sha256 == digest
    target = tmp_path / "owner__repo" / "model-q4_k_m.gguf"
    assert target.read_bytes() == body  # promoted only after verification


@pytest.mark.asyncio
async def test_download_hash_mismatch_refuses_and_cleans(tmp_path, fake_hub):
    body = b"tampered-bytes!!"

    def handler(kind, url):
        return _FakeStreamResp(status=200,
                               headers={"Content-Length": str(len(body))},
                               chunks=[body])
    fake_hub(handler)
    mgr = HFDownloadManager(models_dir=tmp_path)
    task = await mgr.start_download("owner/repo", "model-q8_0.gguf",
                                    expected_sha256="0" * 64)
    done = await mgr.wait_for(task.task_id, timeout=10)
    assert done.status == "failed" and "mismatch" in done.error
    target = tmp_path / "owner__repo" / "model-q8_0.gguf"
    assert not target.exists()  # never promoted
    assert not any((tmp_path / "owner__repo").glob("*.part"))


@pytest.mark.asyncio
async def test_download_oversize_refused_by_gate(tmp_path, fake_hub):
    def handler(kind, url):
        return _FakeStreamResp(status=200,
                               headers={"Content-Length": "999999999"},
                               chunks=[b"x" * 10])
    fake_hub(handler)
    mgr = HFDownloadManager(models_dir=tmp_path, max_bytes=100)
    task = await mgr.start_download("owner/repo", "big-q4_k_m.gguf",
                                    expected_sha256="1" * 64)
    done = await mgr.wait_for(task.task_id, timeout=10)
    assert done.status == "failed" and "exceeds cap" in done.error


# ---------------------------------------------------------------------------
# W2.17: settings allowlist, edge validation, quarantine, atomic writes.
# ---------------------------------------------------------------------------

from kubona.settings import LLM_EDGE_KEYS, PUBLIC_SETTINGS, SECRET_KEYS, SettingsStore


def test_settings_public_is_allowlist_and_masks(tmp_path):
    store = SettingsStore(path=str(tmp_path / "s.json"))
    store.update({"llm_provider": "ollama", "llm_model": "m",
                  "llm_api_key": "sekret", "evil": "x"})
    public = store.public()
    assert public["llm_provider"] == "ollama" and public["llm_api_key"] == "********"
    assert "evil" not in public  # allowlist: unlisted keys never leave
    assert SECRET_KEYS == frozenset({"llm_api_key"})  # single shared const


def test_settings_blank_secret_never_blanks_and_strips(tmp_path):
    store = SettingsStore(path=str(tmp_path / "s.json"))
    store.set("llm_api_key", "keepme")
    store.update({"llm_api_key": "   "})
    assert store.get("llm_api_key") == "keepme"
    store.update({"llm_model": "  qwen  "})
    assert store.get("llm_model") == "qwen"  # stripped
    with pytest.raises(ValueError):
        store.update({"llm_provider": "   "})  # whitespace-only refuses


def test_settings_edge_validates_schema(tmp_path):
    store = SettingsStore(path=str(tmp_path / "s.json"))
    with pytest.raises(ValueError, match="unknown key"):
        store.apply_llm_edge({"nonsense": 1})
    with pytest.raises(ValueError, match="unknown provider"):
        store.apply_llm_edge({"llm_provider": "gpt-99"})
    with pytest.raises(ValueError, match="base_url"):
        store.apply_llm_edge({"llm_provider": "ollama", "llm_base_url": "ftp://x"})
    out = store.apply_llm_edge({"llm_provider": "ollama",
                                "llm_base_url": "http://127.0.0.1:11434",
                                "llm_model": "m"})
    assert out["llm_provider"] == "ollama"
    assert set(out) >= {"llm_provider", "host_available"}  # allowlist + snapshot


def test_settings_corrupt_quarantined_loudly(tmp_path):
    bad = tmp_path / "s.json"
    bad.write_text("{not json")
    with pytest.raises(ValueError, match="quarantined"):
        SettingsStore(path=str(bad))
    assert not bad.exists()  # moved aside, never silently reset
    assert list(tmp_path.glob("s.json.corrupt-*"))
    bad2 = tmp_path / "s2.json"
    bad2.write_text("[1, 2]")
    with pytest.raises(ValueError, match="quarantined"):
        SettingsStore(path=str(bad2))


# ---------------------------------------------------------------------------
# W2.18: providers — downgrade refusal, no default device, fresh secrets,
# vectorized sampling, labeled heuristics, split messaging.
# ---------------------------------------------------------------------------

from kubona.circuits import Circuit, Gate, parse_qasm2
from kubona.providers.base import ProviderError
from kubona.schema import load_provider

BELL_QASM = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def test_qiskit_hardware_refuses_silent_aer_downgrade(monkeypatch):
    pytest.importorskip("qiskit")
    from kubona.providers.qiskit_provider import QiskitProvider

    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    monkeypatch.delenv("BAO_ADDR", raising=False)
    p = QiskitProvider(load_provider("ibm-qiskit"))
    out_of_set = parse_qasm2(BELL_QASM)  # h/cx convert explicitly: conversion passes
    with pytest.raises(ProviderError, match="allow_sim_fallback"):
        p.submit(out_of_set, shots=100)
    out = p.submit(out_of_set, shots=100, allow_sim_fallback=True)
    assert out["hardware"] is False and out["sim_fallback"] is True
    assert sum(out["counts"].values()) == 100


def test_qiskit_gate_allowlist_and_arity():
    pytest.importorskip("qiskit")
    from kubona.providers.qiskit_provider import QiskitProvider

    p = QiskitProvider(load_provider("qiskit-aer-simulator"))
    bad_arity = Circuit(name="t", num_qubits=1,
                        gates=[Gate("rx", (0,), (0.1, 0.2))])
    with pytest.raises(ProviderError, match="takes 1 parameter"):
        p.submit(bad_arity, shots=10)
    unknown = Circuit(name="t", num_qubits=1, gates=[Gate("zzz", (0,))])
    with pytest.raises(ProviderError, match="no explicit Qiskit conversion"):
        p.submit(unknown, shots=10)
    zeroq = type("C", (), {"num_qubits": 1, "gates": [
        type("G", (), {"name": "h", "qubits": (), "params": ()})()]})()
    with pytest.raises(ProviderError, match="zero-qubit"):
        p.submit(zeroq, shots=10)
    with pytest.raises(ProviderError, match="positive int"):
        p.submit(parse_qasm2(BELL_QASM), shots=0)


def test_tket_refuses_unset_backend_and_bad_shots():
    from kubona.providers.tket_provider import TketProvider

    cap = load_provider("quantinuum").model_copy(update={"backend": None})
    p = TketProvider(cap)
    p._ready, p._err = True, None
    assert p.device is None
    with pytest.raises(ProviderError, match="no backend"):
        p.submit(parse_qasm2(BELL_QASM), shots=10)
    with pytest.raises(ProviderError, match="positive int"):
        p.submit(parse_qasm2(BELL_QASM), shots=0)


def test_tket_credentials_fresh_per_call(monkeypatch):
    from kubona.providers.tket_provider import TketProvider

    p = TketProvider(load_provider("quantinuum"))
    assert p._secrets is None  # never cached on the instance
    monkeypatch.setenv("KUBONA_QUANTINUUM_API_KEY", "k1")
    monkeypatch.delenv("BAO_ADDR", raising=False)
    assert p._credential() == "k1"
    monkeypatch.setenv("KUBONA_QUANTINUUM_API_KEY", "k2")
    assert p._credential() == "k2"  # rotation visible, no restart
    # BAO_ADDR is honored per call (fail-closed), never silently env-only
    monkeypatch.setenv("BAO_ADDR", "http://127.0.0.1:1")
    monkeypatch.setenv("BAO_TOKEN", "tok")
    with pytest.raises(Exception):
        p._credential()


def test_pyquil_seeded_path_refuses_wavesim(monkeypatch):
    import sys
    import types

    from kubona.providers.pyquil_provider import PyQuilProvider

    class FakeProg:
        def declare(self, nm, ty, n):
            return self

        def __getitem__(self, i):
            return ("ro", i)

        def measure(self, q, ro):
            return self

        def __iadd__(self, other):
            return self

        def __add__(self, other):
            return self

    class G:
        def __call__(self, *a):
            return ("g", a)

    mod = types.ModuleType("pyquil")
    mod.Program = FakeProg
    monkeypatch.setitem(sys.modules, "pyquil", mod)
    gates_mod = types.ModuleType("pyquil.gates")
    for n in ["H", "X", "Y", "Z", "T", "CNOT", "CZ", "SWAP", "RX", "RY", "RZ"]:
        setattr(gates_mod, n, G())
    monkeypatch.setitem(sys.modules, "pyquil.gates", gates_mod)

    class FakeWF:
        def run_and_measure(self, prog, trials=None):
            return [[0, 0]] * (trials // 2) + [[1, 1]] * (trials // 2)

    api_mod = types.ModuleType("pyquil.api")
    api_mod.WavefunctionSimulator = FakeWF
    monkeypatch.setitem(sys.modules, "pyquil.api", api_mod)
    monkeypatch.delenv("KUBONA_RIGETTI_QCS_TOKEN", raising=False)
    monkeypatch.delenv("BAO_ADDR", raising=False)

    p = PyQuilProvider(load_provider("rigetti"))
    with pytest.raises(ProviderError, match="nondeterministic"):
        p.submit(parse_qasm2(BELL_QASM), shots=64, seed=7)
    out = p.submit(parse_qasm2(BELL_QASM), shots=64, seed=None)
    assert out["deterministic"] is False and out["seed"] is None
    assert out["counts"] == {"00": 32, "11": 32}  # Counter path, exact


def test_local_sim_vectorized_seeded_and_perf():
    from kubona.providers import get_provider

    sim = get_provider("local-simulator")
    bell = parse_qasm2(BELL_QASM)
    a = sim.submit(bell, shots=2000, seed=42)
    b = sim.submit(bell, shots=2000, seed=42)
    assert a["counts"] == b["counts"]  # seeded determinism preserved
    import numpy as _np

    plus = Circuit(name="plus", num_qubits=10,
                   gates=[Gate("h", (i,)) for i in range(10)])
    start = time.time()
    out = sim.submit(plus, shots=100_000, seed=1)
    elapsed = time.time() - start
    assert sum(out["counts"].values()) == 100_000
    assert elapsed < 10.0  # vectorized: old O(shots*2^n) scan needed minutes
    zeroq = type("C", (), {"num_qubits": 1, "gates": [
        type("G", (), {"name": "h", "qubits": (), "params": ()})()]})()
    with pytest.raises(ProviderError, match="zero-qubit"):
        sim.submit(zeroq, shots=10)


def test_annealing_labels_and_validates():
    import numpy as np

    from kubona.providers import get_provider

    prov = get_provider("dwave-annealing")
    Q = np.array([[1.0, -1.0], [-1.0, 1.0]])
    exact = prov.solve_qubo(Q, seed=1)
    assert exact["heuristic"] is False and exact["optimality_gap"] == 0
    assert exact["energy"] == pytest.approx(0.0)
    big = -np.eye(17)
    h = prov.solve_qubo(big, n_samples=200, seed=1)
    assert h["heuristic"] is True and h["optimality_gap"] == "unknown"
    assert len(h["solution"]) == 17 and h["energy"] <= 0
    with pytest.raises(ProviderError):
        prov.solve_qubo(np.zeros((2, 3)), seed=1)  # non-square
    with pytest.raises(ProviderError):
        prov.solve_qubo(np.full((2, 2), np.nan), seed=1)  # non-finite
    tri = np.array([[0.0, 2.0], [0.0, 0.0]])  # upper-triangular QUBO convention
    r = prov.solve_qubo(tri, seed=1)
    assert r["asymmetric_input"] is True and r["energy"] == pytest.approx(0.0)


def test_neutral_photonic_split_sdk_vs_wiring():
    from kubona.providers.pulser_provider import PulserProvider
    from kubona.providers.strawberry_fields_provider import StrawberryFieldsProvider

    for cls, name in ((PulserProvider, "neutral-atom"),
                      (StrawberryFieldsProvider, "photonic")):
        p = cls.__new__(cls)
        p._ready, p._err = False, "nope"
        with pytest.raises(ProviderError, match="SDK missing"):
            p.submit(parse_qasm2(BELL_QASM), shots=4)
        p._ready = True
        with pytest.raises(ProviderError, match="not yet wired"):
            p.submit(parse_qasm2(BELL_QASM), shots=4)


# ---------------------------------------------------------------------------
# W2.21: shared consts — identity with the canonical home, single copies.
# ---------------------------------------------------------------------------

from kubona.schema.provider import MODALITIES_GATE, MODALITIES_SIMULATOR, TWO_Q_GATES


def test_modality_and_gate_consts_are_single_sourced():
    import kubona.canary.canary as canary
    import kubona.mcp.servers.execution as execution
    import kubona.sdk.conformance as conformance

    assert canary._GATE_MODALITIES is MODALITIES_GATE
    assert conformance._GATE_MODALITIES is MODALITIES_GATE
    assert conformance._SIM_MODALITIES is MODALITIES_SIMULATOR
    assert execution._SIMULATOR_MODALITIES is MODALITIES_SIMULATOR
    assert execution._TWO_QUBIT_GATES is TWO_Q_GATES
    # no second full copy of the 2q literal anywhere in the package
    import pathlib
    pkg = pathlib.Path(__file__).resolve().parent.parent / "kubona"
    hits = [p for p in pkg.rglob("*.py")
            if '"cx", "cz", "swap", "cp", "ecr", "rzz"' in p.read_text()]
    assert hits == [pkg / "schema" / "provider.py"], hits
    # the approval price threshold has exactly one definition
    price_hits = [p for p in pkg.rglob("*.py")
                  if "APPROVAL_THRESHOLD_USD =" in p.read_text()]
    assert price_hits == [pkg / "cost" / "metering.py"], price_hits


def test_payload_modality_is_not_hardware_modality():
    from kubona.mesh.model_router import Modality as PayloadModality
    from kubona.schema.provider import Modality as HardwareModality

    payload_values = {m.value for m in PayloadModality}
    hardware_values = {m.value for m in HardwareModality}
    assert payload_values.isdisjoint(hardware_values)  # distinct domains, documented


def test_llm_port_map_is_single_sourced(monkeypatch):
    from kubona.llm import registry

    assert not hasattr(registry, "_PORT_PROVIDER")
    assert registry.PORT_PROVIDERS[8081] == "llama.cpp"
    # openai-compat on 8081 resolves via the single map (was "local" fallback)
    monkeypatch.setattr(registry, "_probe",
                        lambda base, timeout=2.0: {"provider": "openai-compat",
                                                   "models": ["m"]} if base.endswith(":8081") else None)
    reg = registry.LocalModelRegistry(ports=[8081])
    models = reg.available_models()
    assert models and models[0].provider == "llama.cpp"
    assert reg.pick_default()["provider"] == "llama.cpp"


def test_watchdog_threshold_has_no_docstring_second_copy():
    import pathlib
    src = (pathlib.Path(__file__).resolve().parent.parent / "kubona"
           / "control" / "watchdog.py").read_text()
    assert "1e-7" not in src  # the drifted docstring number is gone
    assert "wbp_variance_floor" in src  # the single const is referenced
