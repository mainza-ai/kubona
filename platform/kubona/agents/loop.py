"""An LLM-driven agent loop — the missing 'brain' for workflows and search.

ReAct-style: the LLM chooses a gateway tool + args (as JSON), the loop executes it
under the cost/capability gates, observes, and continues until 'done' or a step
cap / budget limit. Works with any AbstractLLM (local Ollama/LM Studio/MLX, or the
deterministic ScriptedLLM when no local model is present).
"""

from __future__ import annotations

import hashlib
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
    remap it — and SAY SO (W0.15): the remap is returned visibly in
    `_remapped_from` instead of silently rebinding (a `shots` typo silently
    becoming the wrong slot used to be invisible). Multiple unknown keys
    refuse outright: guessing across several slots is fabrication.
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
        out["_remapped_from"] = {missing[0]: unknown[0]}
    elif unknown:
        raise ValueError(f"tool {tool}: unknown args {unknown} (missing {missing}): refusing to guess")
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
                 ledger: Ledger | None = None, max_steps: int = 8, token: str | None = None):
        # W0.15: no default credential. A loop driving a gateway without a
        # token used to authenticate as dev-token silently.
        if gateway is not None and not token:
            raise ValueError("LLMAgentLoop with a gateway requires an explicit token: refusing dev default")
        if max_steps <= 0:
            raise ValueError(f"max_steps must be positive, got {max_steps}")
        self.llm = llm or ScriptedLLM()
        self.gateway = gateway
        self.ledger = ledger
        self.max_steps = max_steps
        self.token = token

    @staticmethod
    def _result_hash(result: dict[str, Any]) -> str:
        return hashlib.sha256(json.dumps(result, sort_keys=True, default=str).encode()).hexdigest()

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
            # W0.15: the allowlist is enforced server-side here, not merely
            # rendered into the prompt (the model used to escape to any tool).
            if available_tools is not None and tool not in available_tools:
                result = {"ok": False, "error": f"tool {tool!r} outside the loop allowlist: refusing"}
                transcript.append({"step": step, "action": tool, "args": parsed.get("args", {}),
                                   "result_summary": str(result)[:300],
                                   "result_hash": self._result_hash(result)})
                continue
            server, _, name = tool.partition("/")
            args = parsed.get("args", {})
            remapped: dict[str, str] = {}
            result: dict[str, Any]
            if self.gateway is None:
                # Dry run executes nothing: ok False with the reason stated
                # (the old ok True presented non-execution as success).
                result = {"ok": False, "dry_run": True,
                          "note": "no gateway mounted: nothing executed"}
            else:
                try:
                    if self.gateway._registry.get(server) and name in self.gateway._registry[server]:
                        args = normalize_args(self.gateway, tool, args)
                        remapped = args.pop("_remapped_from", {})
                    result = self.gateway.call(server, name, args, token=self.token)
                except Exception as e:  # noqa: BLE001
                    result = {"ok": False, "error": str(e)}
            summary = str(result)[:300]
            entry: dict[str, Any] = {"step": step, "action": tool, "args": args,
                                     "result_summary": summary,
                                     "result_hash": self._result_hash(result)}
            if remapped:
                entry["args_remapped_from"] = remapped
            transcript.append(entry)
            if self.ledger is not None:
                self.ledger.append("agent_step", {"task": task, "step": step, "tool": tool,
                                                  "ok": result.get("ok"),
                                                  "result_summary": summary,
                                                  "result_hash": self._result_hash(result),
                                                  "args_remapped_from": remapped or None})
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
        # Strict parse (W0.15): strip markdown fences, decode the FIRST
        # complete object, and refuse trailing prose. The old find/rfind
        # outermost slice let a prompt-injected second object win via rfind.
        text = decision.strip()
        if text.startswith("```"):
            text = text.strip("`\n")
            if text.startswith("json"):
                text = text[4:].lstrip()
        try:
            obj, end = json.JSONDecoder().raw_decode(text)
        except Exception:  # noqa: BLE001
            return None
        if not isinstance(obj, dict):
            return None
        if text[end:].strip():
            return None  # trailing prose after the object: refusing
        return obj
