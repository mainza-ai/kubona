"""The §13 meta-loop (Phase-4 stretch): propose-only, shadow-tested, human-gated."""

import pytest

from kubona.autoresearch.meta import MetaLoop, reflect_on_orchestrator


def test_meta_loop_proposes_but_never_auto_applies(tmp_path):
    meta = MetaLoop(store_path=str(tmp_path / "meta.json"))
    # a history where the second half (candidate strategy) converges better
    history = [0.1, 0.2, 0.3, 0.2, 0.5, 0.6, 0.7, 0.8]
    proposal = reflect_on_orchestrator(history, incumbent_radius=0.1,
                                       candidate_radius=1.0, meta=meta)
    assert proposal is not None
    assert proposal.status == "PROPOSED"  # NOT auto-applied
    assert proposal.kind == "mutation_radius"
    assert proposal.shadow_evidence["proposed_score"] > proposal.shadow_evidence["incumbent_score"]
    # versioned + human-gated
    assert meta.pending() == [proposal]
    meta.approve(proposal.proposal_id, reviewer="pi")
    assert meta.pending() == []
    assert meta._get(proposal.proposal_id).status == "APPROVED"


def test_meta_loop_no_proposal_when_not_better(tmp_path):
    meta = MetaLoop(store_path=str(tmp_path / "meta.json"))
    history = [0.5, 0.4, 0.5, 0.4, 0.5, 0.4, 0.5, 0.4]  # no improvement
    assert reflect_on_orchestrator(history, 0.1, 1.0, meta) is None
    assert meta.pending() == []


def test_meta_loop_write_scope(tmp_path):
    """§13.4: the meta-loop's write access stops at search strategy — it has no
    platform-code surface at all (no compiler/MCP/security handles exist on it)."""
    meta = MetaLoop(store_path=str(tmp_path / "meta.json"))
    attrs = [a for a in dir(meta) if not a.startswith("_")]
    assert not any("compiler" in a or "server" in a or "ledger" in a or "policy" in a for a in attrs)


def test_meta_scope_gate(tmp_path):
    """Q-09: kinds outside the write scope refuse at propose time."""
    from kubona.autoresearch.meta import MetaLoop
    from kubona.autoresearch.meta_scope import MetaScope, ScopeError
    meta = MetaLoop(store_path=str(tmp_path / "meta.json"), scope=MetaScope.default())
    with pytest.raises(ScopeError):
        meta.propose_strategy_change("compiler", None, None, "self-rewrite", {})
    with pytest.raises(ScopeError):
        meta.propose_strategy_change(" entrainment-field", None, None, "novelty", {})
    p = meta.propose_strategy_change("mutation_radius", 0.1, 0.2, "shadow won", {})
    assert p.status == "PROPOSED"


def test_meta_scope_widening(tmp_path):
    """Q-09: widening needs a trusted admin signature; unwidenable refuses all."""
    from nacl.signing import SigningKey
    from kubona.autoresearch.meta import MetaLoop
    from kubona.autoresearch.meta_scope import MetaScope, ScopeError
    import time
    sk = SigningKey.generate()
    admin = sk.verify_key.encode().hex()
    scope = MetaScope.default(trust_roots=(admin,))
    assert scope.allows("mutation_radius")
    assert not scope.allows("retrieval_policy")

    def sign(kind, exp):
        payload = MetaScope.canonical_payload(scope.version, kind, exp)
        return sk.sign(payload).signature.hex()

    exp = time.time() + 60.0
    scope2 = scope.widen("retrieval_policy", admin, sign("retrieval_policy", exp), exp)
    assert scope2.version == 2 and scope2.allows("retrieval_policy")
    assert scope.history == [] and len(scope2.history) == 1

    # Untrusted key refuses.
    from nacl.signing import SigningKey as SK2
    evil = SK2.generate()
    evil_pub = evil.verify_key.encode().hex()
    evil_sig = evil.sign(MetaScope.canonical_payload(scope.version, "x", exp)).signature.hex()
    try:
        scope.widen("x", evil_pub, evil_sig, exp)
        raise AssertionError("untrusted widen must refuse")
    except ScopeError:
        pass

    # Unwidenable refuses even the trusted admin.
    good_sig = sign("compiler", exp)
    try:
        scope.widen("compiler", admin, good_sig, exp)
        raise AssertionError("compiler widen must refuse")
    except ScopeError:
        pass

    # Expired envelope refuses.
    past = time.time() - 10.0
    stale = sk.sign(MetaScope.canonical_payload(scope.version, "y", past)).signature.hex()
    try:
        scope.widen("y", admin, stale, past)
        raise AssertionError("expired widen must refuse")
    except ScopeError:
        pass

    # Widened scope admits new kinds at propose time; digest changes.
    assert scope.digest() != scope2.digest()
    meta = MetaLoop(store_path=str(tmp_path / "meta.json"), scope=scope2)
    p = meta.propose_strategy_change("retrieval_policy", 0.1, 0.2, "shadow won", {})
    assert p.status == "PROPOSED"

    # Persistence round-trips the registry.
    path = scope2.save(tmp_path / "scope.json")
    loaded = MetaScope.load(path)
    assert loaded.version == 2 and loaded.allows("retrieval_policy")
    assert loaded.digest() == scope2.digest()
