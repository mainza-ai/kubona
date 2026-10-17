"""Conversational Research Studio & Dialogue Engine (Ring 4 & Core).

Translates natural language hypotheses into structured scientific proposals,
queries classical anchors in real time, and drives the interactive research copilot.
"""

from __future__ import annotations

import json
import os
import re
import urllib.request
from typing import Any, Dict, List, Optional
from pydantic import BaseModel
from kubona.anchors.chemistry import MolecularAnchorEngine


# Dialogue cost-estimate volume (named const; the USD figure itself is always
# computed via capability.cost_estimate, never hardcoded).
DIALOGUE_ESTIMATE_SHOTS = 4096

# Explicit molecule table: word-boundary patterns only. "be" must never match
# "because" (the old substring router did exactly that). Unknown molecules
# REFUSE (ValueError) — never a silent H2 default.
KNOWN_MOLECULES: Dict[str, str] = {
    "H2": "H2",
    "LIH": "LiH",
    "BEH2": "BeH2",
}
_MOLECULE_RES: List[tuple[re.Pattern, str]] = [
    (re.compile(r"\bh2\b", re.IGNORECASE), "H2"),
    (re.compile(r"\blih\b|li[\s-]*h\b", re.IGNORECASE), "LiH"),
    (re.compile(r"\bbeh2\b|be[\s-]*h2\b", re.IGNORECASE), "BeH2"),
]

_TIER_RES: List[tuple[re.Pattern, str]] = [
    (re.compile(r"\b(ibm|qpu|quantum hardware|cloud)\b", re.IGNORECASE), "T3_CLOUD_QPU"),
]
_TIER_PROVIDER = {
    "T1_LOCAL_EMU": "local-simulator",
    "T3_CLOUD_QPU": "ibm-qiskit",
}


def parse_molecule(user_prompt: str) -> str:
    """Explicit molecule parser: returns the canonical problem key.

    Matches known molecules on word boundaries. Zero matches -> ValueError
    (unknown refuses). Multiple distinct matches -> ValueError (ambiguous,
    refusing to guess).
    """
    hits: List[str] = []
    for rx, canonical in _MOLECULE_RES:
        if rx.search(user_prompt) and canonical not in hits:
            hits.append(canonical)
    if not hits:
        raise ValueError(
            f"unknown molecule in prompt {user_prompt!r}: known: {sorted(KNOWN_MOLECULES.values())} — refusing"
        )
    if len(hits) > 1:
        raise ValueError(f"ambiguous molecules {hits} in prompt — refusing to guess")
    return hits[0]


def _default_model() -> Optional[str]:
    """Model name via config/discovery only (env KUBONA_MLX_MODEL). No hardcoded default."""
    return os.environ.get("KUBONA_MLX_MODEL")


class ChatMessage(BaseModel):
    role: str  # "user", "assistant", "system"
    content: str
    proposal_card: Optional[Dict[str, Any]] = None


class ExperimentProposal(BaseModel):
    title: str
    problem: str
    hamiltonian: str
    target_accuracy: float
    classical_fci_energy: float
    classical_rhf_energy: float
    recommended_tier: str  # "T1_LOCAL_EMU", "T2_CLOUD_SIM", "T3_CLOUD_QPU"
    estimated_cost_usd: float
    ansatz_type: str
    action_gate_required: bool = True


class ResearchDialogueEngine:
    def __init__(self, mlx_endpoint: str = "http://127.0.0.1:8787/v1/chat/completions",
                 model: str | None = None):
        self.mlx_endpoint = mlx_endpoint
        # Model via config/discovery: explicit arg wins, else env. Never hardcoded.
        self.model = model or _default_model()
        self.anchor_engine = MolecularAnchorEngine()

    def estimate_cost_usd(self, tier: str) -> float:
        """Cost via capability.cost_estimate (real pricing model) or refuse.

        Never a hardcoded figure: the USD number comes from the provider
        schema's pricing model applied to a representative 2-qubit ansatz at
        DIALOGUE_ESTIMATE_SHOTS shots. Estimation failures refuse (raise)
        instead of inventing $0.00 or $14.50.
        """
        from kubona.circuits import parse_qasm2

        provider_name = _TIER_PROVIDER.get(tier)
        if provider_name is None:
            raise ValueError(f"no provider mapping for tier {tier!r} — refusing cost estimate")
        from kubona.schema import load_provider

        cap = load_provider(provider_name)
        circuit = parse_qasm2(
            "OPENQASM 2.0;\nqreg q[2];\nry(0.5) q[0];\nry(0.5) q[1];\ncx q[0],q[1];"
        )
        try:
            est = cap.cost_estimate(DIALOGUE_ESTIMATE_SHOTS, circuit)
        except Exception as e:
            raise ValueError(f"cost estimate refused for tier {tier!r}: {e}") from e
        return float(est["estimated_usd"])

    def generate_proposal(self, user_prompt: str) -> ExperimentProposal:
        # Explicit molecule parse: unknown/ambiguous refuses (no H2 default).
        problem = parse_molecule(user_prompt)

        anchor = self.anchor_engine.compute_anchor(problem)
        hamiltonian = f"{problem}_STO3G"

        tier = "T1_LOCAL_EMU"
        for rx, t in _TIER_RES:
            if rx.search(user_prompt):
                tier = t
                break
        # Cost is computed from the provider schema, or the proposal refuses.
        cost = self.estimate_cost_usd(tier)

        return ExperimentProposal(
            title=f"Autonomous Investigation of {problem} Ground State",
            problem=problem,
            hamiltonian=hamiltonian,
            target_accuracy=1.6e-3,
            classical_fci_energy=anchor.fci_energy_hartree,
            classical_rhf_energy=anchor.rhf_energy_hartree,
            recommended_tier=tier,
            estimated_cost_usd=cost,
            ansatz_type="HardwareEfficient_RYCZ_depth2",
            action_gate_required=(cost > 0.0 or tier == "T3_CLOUD_QPU"),
        )

    def query_local_mlx(self, messages: List[Dict[str, str]], max_tokens: int = 150) -> Dict[str, Any]:
        """Query the local MLX model. Returns {ok, text, offline, ...}.

        Failures return ok=False with offline=True and an honest error — NEVER
        a canned paragraph claiming verified anchors (the old fallback
        presented synthesis as verification).
        """
        model = self.model or _default_model()
        if not model:
            return {"ok": False, "text": "local model unavailable: no model configured "
                    "(pass model= or set KUBONA_MLX_MODEL) — refusing to synthesize",
                    "offline": True, "reason": "no_model_configured"}
        try:
            req = urllib.request.Request(
                self.mlx_endpoint,
                headers={"Content-Type": "application/json"},
                data=json.dumps({
                    "model": model,
                    "messages": messages,
                    "max_tokens": max_tokens,
                    "temperature": 0.2,
                }).encode("utf-8"),
            )
            with urllib.request.urlopen(req, timeout=15) as res:
                data = json.loads(res.read().decode("utf-8"))
                return {"ok": True, "text": data["choices"][0]["message"]["content"],
                        "offline": False, "model": model}
        except Exception as e:
            return {"ok": False,
                    "text": f"local model unavailable ({e}) — offline; no proposal text synthesized",
                    "offline": True, "reason": str(e)}

    def process_turn(self, user_prompt: str, history: List[ChatMessage] | None = None) -> ChatMessage:
        try:
            proposal = self.generate_proposal(user_prompt)
        except ValueError as e:
            return ChatMessage(role="assistant", content=f"Refusing: {e}", proposal_card=None)

        # Build prompt for local MLX
        system_prompt = (
            "You are Kubona's Senior Quantum Research Scientist. "
            "You help the human Principal Investigator design experiments, verify classical anchors, "
            "and navigate quantum execution tiers. Be concise, scientific, and precise."
        )
        prompt_msgs = [{"role": "system", "content": system_prompt}]
        if history:
            for m in history[-4:]:
                prompt_msgs.append({"role": m.role, "content": m.content})

        prompt_msgs.append({
            "role": "user",
            "content": f"{user_prompt}\n[Context: Classical FCI baseline={proposal.classical_fci_energy:.4f} Ha, Tier={proposal.recommended_tier}]"
        })

        mlx_res = self.query_local_mlx(prompt_msgs, max_tokens=120)
        if mlx_res.get("ok"):
            ai_reply = mlx_res["text"]
        else:
            # Offline surfaced honestly: the card carries the numbers, the
            # text says the model could not be reached (never canned science).
            ai_reply = (
                f"{mlx_res['text']} Review the proposal card for the computed "
                "classical anchor and cost estimate."
            )

        return ChatMessage(
            role="assistant",
            content=ai_reply,
            proposal_card=proposal.model_dump(),
        )
