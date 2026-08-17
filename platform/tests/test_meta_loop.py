"""The §13 meta-loop (Phase-4 stretch): propose-only, shadow-tested, human-gated."""

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
