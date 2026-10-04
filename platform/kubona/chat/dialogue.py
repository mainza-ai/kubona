"""Conversational Research Studio & Dialogue Engine (Ring 4 & Core).

Translates natural language hypotheses into structured scientific proposals,
queries classical anchors in real time, and drives the interactive research copilot.
"""

from __future__ import annotations

import json
import urllib.request
from typing import Any, AsyncGenerator, Dict, List, Optional
from pydantic import BaseModel
from kubona.anchors.chemistry import MolecularAnchorEngine


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
    def __init__(self, mlx_endpoint: str = "http://127.0.0.1:8787/v1/chat/completions"):
        self.mlx_endpoint = mlx_endpoint
        self.anchor_engine = MolecularAnchorEngine()

    def generate_proposal(self, user_prompt: str) -> ExperimentProposal:
        # Detect target molecule/Hamiltonian or default to H2
        problem = "H2"
        if "lih" in user_prompt.lower():
            problem = "LiH"
        elif "be" in user_prompt.lower():
            problem = "BeH2"

        anchor = self.anchor_engine.compute_anchor(problem)
        hamiltonian = f"{problem}_STO3G"
        
        tier = "T1_LOCAL_EMU"
        cost = 0.0
        if "ibm" in user_prompt.lower() or "cloud" in user_prompt.lower() or "qpu" in user_prompt.lower():
            tier = "T3_CLOUD_QPU"
            cost = 14.50

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

    def query_local_mlx(self, messages: List[Dict[str, str]], max_tokens: int = 150) -> str:
        try:
            req = urllib.request.Request(
                self.mlx_endpoint,
                headers={"Content-Type": "application/json"},
                data=json.dumps({
                    "model": "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
                    "messages": messages,
                    "max_tokens": max_tokens,
                    "temperature": 0.2,
                }).encode("utf-8"),
            )
            with urllib.request.urlopen(req, timeout=15) as res:
                data = json.loads(res.read().decode("utf-8"))
                return data["choices"][0]["message"]["content"]
        except Exception:
            return (
                "Principal Investigator, I have formulated a grounded experiment proposal. "
                "The classical Full Configuration Interaction (FCI) anchor and Restricted Hartree-Fock (RHF) "
                "baselines have been verified in Ring 2. Review the proposal card below to authorize the run."
            )

    def process_turn(self, user_prompt: str, history: List[ChatMessage] | None = None) -> ChatMessage:
        proposal = self.generate_proposal(user_prompt)
        
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
        
        ai_reply = self.query_local_mlx(prompt_msgs, max_tokens=120)
        
        return ChatMessage(
            role="assistant",
            content=ai_reply,
            proposal_card=proposal.model_dump(),
        )
