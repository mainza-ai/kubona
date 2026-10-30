"""Ideation families + elite sharing + widen-nudge (paper's anti-caution fix).

N qualitatively-distinct families mature separately, then share elites
(cross-pollination). Stagnation is measured, never vibes: offers since
last cell-improvement per family. LLM proposals are an explicit callable;
the deterministic mutator (no network) is the test path. The Strata local
engine is the development default; NVIDIA stays explicitly selectable
(parked, not removed). oMLX/OpenCode and other local shims stay parked:
refused outright, never silently routed.
"""

from __future__ import annotations

import json
import os
from typing import Any, Callable

import httpx

from .frontier import MapElitesFrontier

PARKED_PROVIDERS = frozenset({"omlx", "opencode", "ollama", "lm-studio", "mlx", "llama.cpp"})


class FamilySearch:
    """N frontiers + lineage + stagnation. Deterministic, ledger-friendly."""

    def __init__(self, families: list[str], kappa: int = 1):
        if not families or len(set(families)) != len(families) or any(
                not isinstance(f, str) or not f.strip() for f in families):
            raise ValueError("families must be distinct non-blank names: refusing")
        self.families = list(families)
        self.frontiers = {f: MapElitesFrontier(kappa=kappa) for f in families}
        self.lineage: list[dict[str, Any]] = []
        self._stagnant: dict[str, int] = {f: 0 for f in families}

    def offer(self, family: str, artifact: Any, toffoli: int, qubits: int,
              parent: str | None = None) -> bool:
        if family not in self.frontiers:
            raise ValueError(f"unknown family {family!r}: refusing")
        improved = self.frontiers[family].offer(artifact, toffoli, qubits,
                                                {"family": family, "parent": parent})
        entry = {"family": family, "toffoli": toffoli, "qubits": qubits,
                 "improved": improved, "parent": parent}
        self.lineage.append(entry)
        self._stagnant[family] = 0 if improved else self._stagnant[family] + 1
        return improved

    def elites(self) -> dict[str, Any]:
        """Best entry per family (cross-pollination pool)."""
        out: dict[str, Any] = {}
        for f, fr in self.frontiers.items():
            b = fr.best()
            if b is not None:
                out[f] = {"toffoli": b.toffoli, "qubits": b.qubits,
                          "tq": b.tq, "artifact": b.artifact}
        return out

    def share_elites(self) -> dict[str, Any]:
        """One cross-pollination round (paper Fig-overview(a)): offer every
        family's best artifact into every OTHER family's frontier.

        Deterministic (sorted family order); each cross-offer records lineage
        with parent="x-poll:<src-family>" so the idea's origin is auditable.
        Returns {offers, improvements, details}. Fail-closed on empty elites
        (nothing to share -> zero offers, never a fake improvement).
        """
        elites = self.elites()
        if not elites:
            return {"offers": 0, "improvements": 0, "details": []}
        details: list[dict[str, Any]] = []
        improvements = 0
        offers = 0
        for dst in sorted(self.frontiers):
            for src in sorted(elites):
                if src == dst:
                    continue
                e = elites[src]
                offers += 1
                improved = self.offer(dst, e["artifact"], e["toffoli"],
                                      e["qubits"], parent=f"x-poll:{src}")
                details.append({"from": src, "to": dst, "improved": improved,
                                "toffoli": e["toffoli"], "qubits": e["qubits"]})
                if improved:
                    improvements += 1
        return {"offers": offers, "improvements": improvements, "details": details}

    def widen_nudge(self) -> dict[str, Any]:
        """Which family most needs a widen nudge (max offers-since-improvement)."""
        fam = max(self._stagnant, key=lambda f: self._stagnant[f])
        return {"family": fam, "offers_since_improvement": self._stagnant[fam],
                "nudge": f"widen {fam}: propose a qualitatively-distinct rewrite, not a parameter tweak"}

    def to_dict(self) -> dict[str, Any]:
        return {"families": self.families, "stagnation": dict(self._stagnant),
                "elites": self.elites(), "lineage_len": len(self.lineage)}


def deterministic_mutator(artifact: list, step: int) -> list:
    """No-network proposer for tests: toggles a trailing marker op deterministically."""
    if not isinstance(artifact, list):
        raise ValueError("artifact must be a list: refusing")
    out = [tuple(t) for t in artifact]
    # deterministic, kickmix-legal tweak: append x(0) on even steps, drop it on odd
    if step % 2 == 0:
        out.append(("x", (0,), ()))
    elif out and out[-1][0] == "x":
        out.pop()
    return out


class NvidiaProposer:
    """Production proposer hook: Strata local by default, NVIDIA parked.

    Strata (`strata`, OpenAI-compatible localhost) is the development
    default; NVIDIA NIM (`nvidia`) stays explicitly selectable for
    parked-but-available use and still requires its key. Parked shims
    (oMLX/OpenCode/...) refuse outright: never silently routed.
    propose() is a bounded OpenAI-compatible call (timeout + one retry on
    transport/5xx; 4xx is a refusal). Tests use a local stub server.
    """

    # NOTE: the class name is historical (it was NVIDIA-only); the
    # contract below is provider-agnostic across the two supported
    # backends. Renaming would churn imports for zero behavior change.
    def __init__(self, provider: str = "strata", model: str | None = None,
                 base_url: str | None = None, timeout_sec: float = 30.0,
                 max_retries: int = 1, max_tokens: int = 512):
        if provider in PARKED_PROVIDERS:
            raise ValueError(
                f"provider {provider!r} is parked: refusing (use strata for local dev)")
        if provider not in ("strata", "nvidia"):
            raise ValueError(
                f"proposer supports strata (dev default) or nvidia (parked fallback), "
                f"got {provider!r}: refusing")
        self.provider = provider
        if provider == "strata":
            self.model = model or os.environ.get("STRATA_MODEL", "")
            # Empty env counts as unset (fall back to the Serving-panel URL,
            # never to an empty string that would silently misroute calls).
            self.base_url = base_url or os.environ.get("STRATA_BASE_URL") or "http://127.0.0.1:47831/v1"
            self.api_key = os.environ.get("STRATA_API_KEY", "")
        else:
            self.model = model or os.environ.get("NVIDIA_MODEL", "")
            self.base_url = base_url or "https://integrate.api.nvidia.com/v1"
            self.api_key = os.environ.get("NVIDIA_API_KEY", "")
            if not self.api_key:
                raise ValueError("NVIDIA_API_KEY absent: refusing LLM proposal (deterministic path only)")
        if not self.model:
            env_var = "STRATA_MODEL" if provider == "strata" else "NVIDIA_MODEL"
            raise ValueError(f"no model configured: set {env_var} (or pass model= explicitly)")
        if timeout_sec <= 0 or max_retries < 0 or max_tokens <= 0:
            raise ValueError("timeout_sec/max_tokens must be positive and max_retries non-negative: refusing")
        self.timeout_sec = float(timeout_sec)
        self.max_retries = int(max_retries)
        self.max_tokens = int(max_tokens)

    def propose(self, prompt: str) -> str:
        """Call the configured OpenAI-compatible endpoint with a bound.

        One retry on transport/5xx only; 4xx is a refusal, not a retry. The
        deterministic mutator is the explicit test path, never a silent
        fallback for a broken provider.
        """
        if not isinstance(prompt, str) or not prompt.strip():
            raise ValueError("prompt must be non-blank: refusing")
        endpoint = self.base_url.rstrip("/") + "/chat/completions"
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        payload = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": "You propose reversible Kickmix circuits. Reply with JSON only."},
                {"role": "user", "content": prompt},
            ],
            "temperature": 0.2,
            "max_tokens": self.max_tokens,
            "stream": False,
        }
        last: Exception | None = None
        for attempt in range(self.max_retries + 1):
            try:
                with httpx.Client(timeout=self.timeout_sec) as client:
                    resp = client.post(endpoint, json=payload, headers=headers)
                if resp.status_code >= 500:
                    last = RuntimeError(f"proposer HTTP {resp.status_code}: {resp.text[:160]}")
                    continue
                if resp.status_code >= 400:
                    raise ValueError(f"proposer refused (HTTP {resp.status_code}): {resp.text[:160]}")
                data = resp.json()
                content = data.get("choices", [{}])[0].get("message", {}).get("content")
                if not isinstance(content, str) or not content.strip():
                    raise ValueError("proposer returned no message content: refusing")
                return content
            except (httpx.TransportError, httpx.TimeoutException) as exc:
                last = exc
                continue
        raise RuntimeError(f"proposer unreachable after {self.max_retries + 1} attempt(s): {last}")


# The exact gate vocabulary and arity the Kickmix evaluator accepts. `cond`
# is only meaningful for diagonal gates, but the parser does not invent
# records; it passes what the model returned to the harness, which re-validates.
_GATE_ARITY = {
    "x": 1, "cx": 2, "ccx": 3, "swap": 2,
    "z": 1, "cz": 2, "ccz": 3, "global_flip": 0, "hmr": 1,
    "alloc_marker": 1, "free_marker": 1,
}


def parse_gate_proposal(text: str, num_qubits: int, max_ops: int = 64) -> list[tuple]:
    """Parse a strict JSON gate proposal; raise ValueError on any doubt.

    Accepted shapes: {"ops": [[name, [qubits], [cond]], ...]} or a bare
    list of the same. Code fences are tolerated because models emit them;
    everything else is validated against the closed gate/arity set and the
    circuit width. The returned tuples are what `CircuitCodesignHarness`
    consumes.
    """
    if not isinstance(text, str) or not text.strip():
        raise ValueError("empty proposal: refusing")
    raw = text.strip()
    if raw.startswith("```"):
        raw = raw.strip("`")
        if raw.startswith("json"):
            raw = raw[4:]
        raw = raw.strip()
    start = min([i for i in (raw.find("{"), raw.find("[")) if i >= 0], default=-1)
    if start < 0:
        raise ValueError("proposal contains no JSON object/array: refusing")
    try:
        doc = json.loads(raw[start:])
    except json.JSONDecodeError as exc:
        raise ValueError(f"proposal is not valid JSON: {exc}") from exc
    ops = doc.get("ops") if isinstance(doc, dict) else doc
    if not isinstance(ops, list) or not ops:
        raise ValueError("proposal must contain a non-empty ops list: refusing")
    if len(ops) > max_ops:
        raise ValueError(f"proposal has {len(ops)} ops, cap is {max_ops}: refusing")
    out: list[tuple] = []
    for idx, op in enumerate(ops):
        if isinstance(op, dict):
            name = op.get("name")
            qubits = op.get("qubits", [])
            cond = op.get("cond", [])
        elif isinstance(op, (list, tuple)) and len(op) == 3:
            name, qubits, cond = op
        else:
            raise ValueError(f"op {idx} must be {{name,qubits,cond}} or [name,qubits,cond]: refusing")
        if not isinstance(name, str) or name not in _GATE_ARITY:
            raise ValueError(f"op {idx} has unknown gate {name!r}: refusing")
        if not isinstance(qubits, (list, tuple)) or not isinstance(cond, (list, tuple)):
            raise ValueError(f"op {idx} qubits/cond must be lists: refusing")
        if len(qubits) != _GATE_ARITY[name]:
            raise ValueError(f"op {idx} gate {name!r} needs {_GATE_ARITY[name]} qubit(s), got {len(qubits)}: refusing")
        qs = []
        for q in qubits:
            if isinstance(q, bool) or not isinstance(q, int) or q < 0 or q >= num_qubits:
                raise ValueError(f"op {idx} qubit {q!r} is outside [0,{num_qubits}): refusing")
            qs.append(q)
        cs = []
        for c in cond:
            if isinstance(c, bool) or not isinstance(c, int) or c < 0:
                raise ValueError(f"op {idx} cond record {c!r} is invalid: refusing")
            cs.append(c)
        out.append((name, tuple(qs), tuple(cs)))
    return out


def make_gate_proposer(proposer: Any, num_qubits: int, max_ops: int = 64) -> Callable[[list, int], list[tuple]]:
    """Adapt a text proposer to `IndependentRerun`'s (current, step)->ops.

    The LLM sees the current artifact and the closed vocabulary; the parser
    refuses anything else. A proposer/network failure raises, and the rerun
    loop records that trial as invalid rather than aborting the run.
    """
    if num_qubits < 1 or num_qubits > 64:
        raise ValueError("num_qubits must be in [1,64]: refusing")
    gates = ", ".join(sorted(_GATE_ARITY))
    def propose(current: list, step: int) -> list[tuple]:
        prompt = (
            "Complete the reversible Kickmix circuit. "
            f"Allowed gates and qubit arity: {gates}. "
            "Only diagonal gates may carry a cond list of measurement-record ids. "
            f"num_qubits={num_qubits}; step={step}; max_ops={max_ops}. "
            f"Current ops: {json.dumps(current)}. "
            'Reply with JSON only: {"ops": [[name, [qubits], [cond]], ...]}.'
        )
        return parse_gate_proposal(proposer.propose(prompt), num_qubits, max_ops)
    return propose
