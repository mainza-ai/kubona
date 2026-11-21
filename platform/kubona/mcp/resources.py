"""Read-only MCP Resources for Kubona self-onboarding (Phase 1).

Exposes live, contract-backed reference data directly over MCP so any
connected AI agent can inspect Kubona's skills, error catalog, rate cards,
experiment kinds, and tool catalog without cloning the repository.
"""

from __future__ import annotations

import json
import os
import re
from pathlib import Path

from mcp.server.mcpserver import MCPServer

_SKILL_NAME_RE = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$")


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[3]


def _contracts_dir() -> Path:
    env_dir = os.environ.get("KUBONA_CONTRACTS_DIR")
    if env_dir:
        return Path(env_dir)
    pkg_rel = Path(__file__).resolve().parents[1] / "contracts"
    if pkg_rel.exists():
        return pkg_rel
    return _repo_root() / "platform" / "contracts"


def _skills_dir() -> Path:
    env_dir = os.environ.get("KUBONA_EXTERNAL_SKILLS_DIR")
    if env_dir:
        return Path(env_dir)
    return _repo_root() / "skills"


def list_external_skills() -> list[dict]:
    """Enumerate available repo-level external agent skills (name + description)."""
    root = _skills_dir()
    if not root.exists():
        return []
    out: list[dict] = []
    for entry in sorted(root.iterdir()):
        if not entry.is_dir() or entry.is_symlink():
            continue
        skill_md = entry / "SKILL.md"
        if not skill_md.exists() or skill_md.is_symlink():
            continue
        text = skill_md.read_text(encoding="utf-8")
        desc = ""
        if text.startswith("---\n"):
            end = text.find("\n---\n", 4)
            if end > 0:
                for line in text[4:end].splitlines():
                    if line.startswith("description:"):
                        desc = line.split(":", 1)[1].strip()
                        break
        out.append({
            "name": entry.name,
            "uri": f"kubona://skills/{entry.name}",
            "description": desc,
            "bytes": skill_md.stat().st_size,
        })
    return out


def read_external_skill(name: str) -> str:
    """Read one external SKILL.md by validated directory name (fail-closed)."""
    if not _SKILL_NAME_RE.match(name):
        raise ValueError(f"invalid skill name: {name!r}")
    root = _skills_dir()
    skill_dir = root / name
    if skill_dir.is_symlink() or not skill_dir.is_dir():
        raise FileNotFoundError(f"unknown skill: {name!r}")
    skill_md = skill_dir / "SKILL.md"
    if skill_md.is_symlink() or not skill_md.is_file():
        raise FileNotFoundError(f"SKILL.md missing for {name!r}")
    return skill_md.read_text(encoding="utf-8")


def register_resources(server: MCPServer) -> None:
    """Attach all read-only Kubona onboarding and contract resources to `server`."""

    @server.resource(
        "kubona://skills",
        name="kubona_skills_index",
        title="Kubona Agent Skills Index",
        description="Index of external AI agent SKILL.md guides with kubona://skills/{name} URIs.",
        mime_type="application/json",
    )
    def skills_index() -> str:
        return json.dumps({"skills": list_external_skills()}, indent=2)

    @server.resource(
        "kubona://skills/{name}",
        name="kubona_skill_body",
        title="Kubona Agent Skill Document",
        description="Full markdown instructions for one Kubona external agent skill.",
        mime_type="text/markdown",
    )
    def skill_body(name: str) -> str:
        return read_external_skill(name)

    @server.resource(
        "kubona://contracts/error-catalog",
        name="kubona_error_catalog",
        title="Kubona Error Catalog (CSV)",
        description="Closed error code vocabulary with HTTP status, retryability, and remediation.",
        mime_type="text/csv",
    )
    def error_catalog() -> str:
        p = _contracts_dir() / "error-catalog.csv"
        if not p.exists():
            raise FileNotFoundError("error-catalog.csv not found")
        return p.read_text(encoding="utf-8")

    @server.resource(
        "kubona://contracts/mcp-catalog",
        name="kubona_mcp_catalog",
        title="Kubona Reviewed MCP Tool Catalog",
        description="Digest-pinned MCP tool catalog with trust_property and error_codes per tool.",
        mime_type="application/json",
    )
    def mcp_catalog() -> str:
        p = _contracts_dir() / "mcp-catalog.json"
        if not p.exists():
            raise FileNotFoundError("mcp-catalog.json not found")
        return p.read_text(encoding="utf-8")

    @server.resource(
        "kubona://contracts/experiment-kinds",
        name="kubona_experiment_kinds",
        title="Kubona Experiment Result Schema (v1)",
        description="JSON Schema for experiment-result-v1 covering all supported experiment kinds.",
        mime_type="application/json",
    )
    def experiment_kinds() -> str:
        p = _contracts_dir() / "schemas" / "experiment-result-v1.schema.json"
        if not p.exists():
            raise FileNotFoundError("experiment-result-v1.schema.json not found")
        return p.read_text(encoding="utf-8")

    @server.resource(
        "kubona://contracts/rate-cards",
        name="kubona_rate_cards",
        title="Kubona Provider Rate Cards",
        description="Reviewed list-price rate card used for pre-flight cost estimation.",
        mime_type="application/json",
    )
    def rate_cards() -> str:
        p = _contracts_dir() / "rate-cards" / "list-price-2026-10.json"
        if not p.exists():
            # Fallback: return any first JSON under rate-cards/
            rc_dir = _contracts_dir() / "rate-cards"
            if rc_dir.exists():
                candidates = sorted(rc_dir.glob("*.json"))
                if candidates:
                    return candidates[0].read_text(encoding="utf-8")
            raise FileNotFoundError("rate card contract not found")
        return p.read_text(encoding="utf-8")
