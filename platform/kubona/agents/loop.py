"""An LLM-driven agent loop — the missing 'brain' for workflows and search.

ReAct-style: the LLM chooses a gateway tool + args (as JSON), the loop executes it
under the cost/capability gates, observes, and continues until 'done' or a step
cap / budget limit. Works with any AbstractLLM (local Ollama/LM Studio/MLX, or the
deterministic ScriptedLLM when no local model is present).
"""

from __future__ import annotations

import json
import re
import uuid
from typing import Any

from kubona.llm import AbstractLLM, ScriptedLLM
from kubona.mcp.gateway import KubonaGateway
from kubona.provenance import Ledger

SYSTEM = ("You are Kubona's research agent. Choose ONE next action as JSON: "
          '{"tool": "<server>/<tool>", "args": {...}} — or {"done": "<final summary>"}. '
          "Respect budgets; prefer the simulator tier; one action per turn.")


def _signature_params(gateway: KubonaGateway, tool: str) -> list[str]:
    """Ordered param names for a tool (for arg normalization)."""
    import inspect

    server, _, name = tool.partition("/")
    fn = (gateway._registry.get(server) or {}).get(name)
    if fn is None:
        return []
    try:
        return [pn for pn, param in inspect.signature(fn).parameters.items()
                if param.kind in (param.POSITIONAL_OR_KEYWORD, param.KEYWORD_ONLY)]
    except Exception:  # noqa: BLE001
        return []


def normalize_args(gateway: KubonaGateway, tool: str, args: dict[str, Any]) -> dict[str, Any]:
    """Best-effort map the model's arg keys onto the tool's real signature.

    Handles the common failure where a model uses a synonym for the canonical
    param name (e.g. `circuit` for `circuit_qasm`): if the tool has exactly one
    unfilled required param and the model supplied exactly one unrecognized key,
    remap it; drop other unknown keys. This makes tool-calling robust across
    local models' naming habits.
    """
    params = _signature_params(gateway, tool)
    if not params:
        return args
    out = {k: v for k, v in args.items() if k in params}
    unknown = [k for k in args if k not in params]
    missing = [p for p in params if p not in out]
    # if there is exactly one unmatched required/positional param and exactly one
    # unknown key, treat the unknown key as that param
    if len(unknown) == 1 and len(missing) == 1:
        out[missing[0]] = args[unknown[0]]
    return out


def tool_signature_map(gateway: KubonaGateway | None) -> dict[str, str]:
    """Derive exact arg signatures for every mounted tool (from the gateway functions)."""
    import inspect

    sigs: dict[str, str] = {}
    if gateway is None:
        return sigs
    for server, tools in gateway._registry.items():
        for name, fn in tools.items():
            try:
                params = []
                for pname, param in inspect.signature(fn).parameters.items():
                    default = "" if param.default is inspect.Parameter.empty else f"={param.default!r}"
                    ann = param.annotation.__name__ if param.annotation is not inspect.Parameter.empty else "?"
                    params.append(f"{pname}: {ann}{default}")
                sigs[f"{server}/{name}"] = f"{server}/{name}({', '.join(params)})"
            except Exception:  # noqa: BLE001
                sigs[f"{server}/{name}"] = f"{server}/{name}"
    return sigs


class LLMAgentLoop:
    def __init__(self, llm: AbstractLLM | None = None, gateway: KubonaGateway | None = None,
                 ledger: Ledger | None = None, max_steps: int = 8, token: str = "dev-token"):
        self.llm = llm or ScriptedLLM()
        self.gateway = gateway
        self.ledger = ledger
        self.max_steps = max_steps
        self.token = token

    def run(self, task: str, available_tools: list[str] | None = None) -> dict[str, Any]:
        transcript: list[dict[str, Any]] = []
        tool_specs = tool_signature_map(self.gateway)
        for step in range(self.max_steps):
            prompt = self._prompt(task, transcript, available_tools, tool_specs)
            decision = self.llm.complete(SYSTEM, prompt, temperature=0.2, json_mode=True)
            parsed = self._parse(decision)
            if parsed is None:
                transcript.append({"step": step, "action": "parse-error", "decision": decision})
                continue
            if "done" in parsed:
                transcript.append({"step": step, "action": "done", "summary": parsed["done"]})
                break
            tool = parsed.get("tool", "")
            server, _, name = tool.partition("/")
            args = parsed.get("args", {})
            result: dict[str, Any]
            if self.gateway is None:
                result = {"ok": True, "note": "no gateway mounted (dry run)"}
            else:
                try:
                    if self.gateway._registry.get(server) and name in self.gateway._registry[server]:
                        args = normalize_args(self.gateway, tool, args)
                    result = self.gateway.call(server, name, args, token=self.token)
                except Exception as e:  # noqa: BLE001
                    result = {"ok": False, "error": str(e)}
            transcript.append({"step": step, "action": tool, "args": args, "result_summary": str(result)[:300]})
            if self.ledger is not None:
                self.ledger.append("agent_step", {"task": task, "step": step, "tool": tool,
                                                  "ok": result.get("ok"),
                                                  "result_summary": str(result)[:300]})
        return {"task": task, "steps": len(transcript), "transcript": transcript,
                "done": any(t.get("action") == "done" for t in transcript)}

    # -- prompt / parse ----------------------------------------------------
    def _prompt(self, task: str, transcript: list[dict[str, Any]], tools: list[str] | None,
                tool_specs: dict[str, str]) -> str:
        lines = [f"Task: {task}"]
        if tool_specs:
            lines.append("Tool signatures (use EXACT arg names):")
            for spec in sorted(tool_specs.values()):
                if tools is None or spec.split("(")[0].strip() in tools:
                    lines.append("  " + spec)
        else:
            lines.append("Available tools: " + (", ".join(tools or []) or "any gateway tool"))
        lines.append("Output ONLY a JSON object: {\"tool\": \"<server>/<tool>\", \"args\": {...}} "
                     "or {\"done\": \"<summary>\"}.")
        for t in transcript:
            lines.append(f"step {t['step']}: {t.get('action')} -> {t.get('result_summary', '')[:120]}")
        return "\n".join(lines) + "\nNext action JSON:"

    @staticmethod
    def _parse(decision: str) -> dict[str, Any] | None:
        # strip markdown fences / surrounding prose defensively
        text = decision.strip()
        if text.startswith("```"):
            text = text.strip("`\n")
            if text.startswith("json"):
                text = text[4:].lstrip()
        start, end = text.find("{"), text.rfind("}")
        if start < 0 or end <= start:
            return None
        try:
            obj = json.loads(text[start:end + 1])  # outermost object (args may nest)
            return obj if isinstance(obj, dict) else None
        except Exception:  # noqa: BLE001
            return None
