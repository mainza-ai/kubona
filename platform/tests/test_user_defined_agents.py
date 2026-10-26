"""Unit tests for UserDefinedAgentManifest and CustomAgentRegistry (Phase 3)."""

import pytest
from pathlib import Path
from kubona.agents.user_defined import (
    CustomAgentRegistry,
    UserDefinedAgentManifest,
    STANDARD_SYSTEM_AGENTS,
)


def test_standard_system_agents_loaded():
    reg = CustomAgentRegistry()
    agents = reg.list_all(include_system=True)
    assert len(agents) >= len(STANDARD_SYSTEM_AGENTS)
    roles = {a.role for a in agents}
    assert "lead_theorist" in roles
    assert "lean4_proof_engineer" in roles
    assert "classical_anchor_solver" in roles
    assert "quantum_circuit_compiler" in roles
    assert "adversarial_falsification_critic" in roles
    assert "sentinel_spend_governor" in roles
    assert "multimodal_spectroscopist" in roles


def test_register_custom_agent(tmp_path: Path):
    storage = tmp_path / "custom_agents.json"
    reg = CustomAgentRegistry(storage_path=storage)

    custom = UserDefinedAgentManifest(
        id="agent_photovoltaic_scout",
        name="Perovskite Photovoltaic Scout",
        role="photovoltaic_crystallographer",
        description="Scouts halide perovskite band gaps using hybrid DFT and tensor networks.",
        system_prompt="Analyze halide perovskite crystallographic CIF files.",
        capabilities=["cif_analysis", "band_gap_estimation"],
        modalities=["text", "molecule_3d", "spectroscopy"],
        preferred_model="local/qwen2.5-coder:32b",
        spend_budget_usd=5.0,
    )

    registered = reg.register_custom_agent(custom)
    assert registered.id == "agent_photovoltaic_scout"
    assert registered.is_system_agent is False

    # Verify persistence
    reg2 = CustomAgentRegistry(storage_path=storage)
    found = reg2.get("agent_photovoltaic_scout")
    assert found is not None
    assert found.name == "Perovskite Photovoltaic Scout"
    assert "cif_analysis" in found.capabilities

    # Test delete
    deleted = reg2.delete_custom_agent("agent_photovoltaic_scout")
    assert deleted is True
    assert reg2.get("agent_photovoltaic_scout") is None


def test_validation_rejects_overwriting_system_agent():
    reg = CustomAgentRegistry()
    malicious = UserDefinedAgentManifest(
        id="sys_lead_theorist",
        name="Fake Theorist",
        role="lead_theorist",
        description="Overwriting system agent",
        system_prompt="Fake prompt",
    )
    with pytest.raises(ValueError, match="Cannot overwrite reserved system agent"):
        reg.register_custom_agent(malicious)


def _manifest(**kw):
    base = dict(id="agent_x", name="X", role="r", description="d",
                system_prompt="prompt")
    base.update(kw)
    return UserDefinedAgentManifest(**base)


def test_validation_contract_edges():
    # P1-4 step 1 (freeze): every remaining refusal edge of
    # register/delete, pinned before any consolidation migration.
    reg = CustomAgentRegistry()
    with pytest.raises(ValueError, match="ID cannot be empty"):
        reg.register_custom_agent(_manifest(id="  "))
    with pytest.raises(ValueError, match="name cannot be empty"):
        reg.register_custom_agent(_manifest(name=" "))
    with pytest.raises(ValueError, match="system prompt cannot be empty"):
        reg.register_custom_agent(_manifest(system_prompt=""))
    # is_system_agent forgery is forced False, never stored True.
    got = reg.register_custom_agent(_manifest(is_system_agent=True))
    assert got.is_system_agent is False
    # System agents refuse deletion; unknown deletes report False.
    with pytest.raises(ValueError, match="Cannot delete system agent"):
        reg.delete_custom_agent("sys_lead_theorist")
    assert reg.delete_custom_agent("agent_no_such") is False


def test_sqlite_persistence_and_parity(tmp_path: Path):
    db_file = tmp_path / "kubona_test.db"
    reg = CustomAgentRegistry(db_path=db_file)

    manifest = UserDefinedAgentManifest(
        id="agent_sqlite_scout",
        name="SQLite Scout",
        role="data_scout",
        description="Tests SQLite backing store.",
        system_prompt="Probe database state.",
        mode="pipeline",
        steps=[{"id": "s1", "tool": "provenance/query_records"}],
        scopes=["provenance:query"],
    )
    reg.register_custom_agent(manifest)

    # Reopen in new instance from SQLite db
    reg2 = CustomAgentRegistry(db_path=db_file)
    loaded = reg2.get("agent_sqlite_scout")
    assert loaded is not None
    assert loaded.name == "SQLite Scout"
    assert loaded.mode == "pipeline"
    assert len(loaded.steps) == 1
    assert loaded.steps[0]["tool"] == "provenance/query_records"
    assert loaded.scopes == ["provenance:query"]

    # Delete persists
    assert reg2.delete_custom_agent("agent_sqlite_scout") is True
    reg3 = CustomAgentRegistry(db_path=db_file)
    assert reg3.get("agent_sqlite_scout") is None


def test_pipeline_mode_and_scope_validation():
    reg = CustomAgentRegistry()
    # Invalid mode refuses
    with pytest.raises(ValueError, match="mode must be 'prompt' or 'pipeline'"):
        reg.register_custom_agent(_manifest(mode="invalid"))

    # Pipeline mode with no steps refuses
    with pytest.raises(ValueError, match="pipeline mode requires at least one step"):
        reg.register_custom_agent(_manifest(mode="pipeline", steps=[]))

    # Pipeline mode with malformed step refuses
    with pytest.raises(ValueError, match="requires non-empty 'id' and 'tool'"):
        reg.register_custom_agent(_manifest(mode="pipeline", steps=[{"id": "step1"}]))

    # Empty scope string refuses
    with pytest.raises(ValueError, match="scope cannot be empty string"):
        reg.register_custom_agent(_manifest(scopes=["   "]))

