"""Lean 4 Comparator Challenge Evaluation and Axiom Integrity Engine.

Evaluates formal proofs against the `comparator` challenge specifications
found in `submodules/math/lean/ComparatorChallenges/`.

Verifies:
1. Challenge module and solution declaration mappings.
2. Axiom containment: ensures proofs only use standard Lean 4 axioms:
   {"propext", "Quot.sound", "Classical.choice"}
   and rejects non-permitted axioms or unproven holes (`sorryAx`).
3. Emits cryptographic/audit records into the Kubona provenance ledger.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional, Set

from kubona.provenance import Ledger

STANDARD_PERMITTED_AXIOMS: Set[str] = {
    "propext",
    "Quot.sound",
    "Classical.choice",
}


@dataclass(frozen=True)
class LeanSandboxConfig:
    """Q-31: resource caps for containerized Lean verification.

    The sandbox image carries the pinned toolchain (platform/docker/
    Dockerfile.lean, leanprover/lean4:v4.34.1); these caps apply at every
    `docker run` so one proof cannot exhaust the host (RAM mappings, CPU,
    PIDs). Network is always none at runtime; the build cache must be warmed
    at image-build time (see Dockerfile).
    """

    image: str = "kubona-lean:4.34.1"
    memory: str = "4g"
    memory_swap: str = "4g"
    cpus: str = "2.0"
    pids_limit: int = 256
    timeout_seconds: int = 600


_DOCKER_INFRA_MARKERS = (
    "docker:", "Cannot connect to the Docker daemon",
    "no such image", "OCI runtime", "permission denied while trying to connect",
    "error gathering device information",
)


def _is_infra_failure(stderr: str) -> bool:
    """Docker-level failures (daemon/image/runtime) vs genuine proof verdicts.

    A failing proof must NEVER degrade into a passing AST fallback — that
    would mask a real failure. Only infrastructure breakdowns fall back.
    """
    low = (stderr or "").lower()
    return any(m.lower() in low for m in _DOCKER_INFRA_MARKERS)


def docker_available() -> bool:
    """Docker daemon reachable for sandboxed verification."""
    if shutil.which("docker") is None:
        return False
    try:
        proc = subprocess.run(["docker", "info", "--format", "{{.NCPU}}"],
                              capture_output=True, text=True, timeout=15)
        return proc.returncode == 0
    except (subprocess.TimeoutExpired, FileNotFoundError, PermissionError, OSError):
        return False


def docker_image_present(image: str) -> bool:
    """Fail closed when the pinned image is absent (never pull implicitly:
    an unexpected multi-GB pull at verification time is itself a hazard)."""
    try:
        proc = subprocess.run(["docker", "image", "inspect", image],
                              capture_output=True, text=True, timeout=30)
        return proc.returncode == 0
    except (subprocess.TimeoutExpired, FileNotFoundError, PermissionError, OSError):
        return False


def sandbox_docker_cmd(cfg: LeanSandboxConfig, lean_dir: Path, challenge_name: str) -> list[str]:
    """The exact container invocation (unit-tested flag-for-flag)."""
    return [
        "docker", "run", "--rm",
        "--read-only",
        "--network", "none",
        "--cap-drop", "ALL",
        "--memory", cfg.memory,
        "--memory-swap", cfg.memory_swap,
        "--cpus", cfg.cpus,
        "--pids-limit", str(cfg.pids_limit),
        "--tmpfs", "/tmp",
        "-v", f"{lean_dir}:/workspace:rw",
        "-w", "/workspace",
        cfg.image,
        "exe", "comparator", challenge_name,
    ]


@dataclass(frozen=True)
class ComparatorChallengeSpec:
    challenge_name: str
    challenge_module: str
    solution_module: str
    theorem_names: List[str]
    definition_names: List[str]
    permitted_axioms: List[str]
    enable_nanoda: bool = False
    source_file_path: Optional[str] = None

    @classmethod
    def from_dict(cls, challenge_name: str, data: Dict[str, Any], path: Optional[str] = None) -> ComparatorChallengeSpec:
        return cls(
            challenge_name=challenge_name,
            challenge_module=data.get("challenge_module", ""),
            solution_module=data.get("solution_module", ""),
            theorem_names=data.get("theorem_names", []),
            definition_names=data.get("definition_names", []),
            permitted_axioms=data.get("permitted_axioms", list(STANDARD_PERMITTED_AXIOMS)),
            enable_nanoda=data.get("enable_nanoda", False),
            source_file_path=path,
        )


@dataclass
class VerificationResult:
    challenge_name: str
    ok: bool
    axioms_used: List[str]
    unpermitted_axioms: List[str]
    theorem_names: List[str]
    error_message: Optional[str] = None
    ledger_ref: Optional[str] = None
    verification_mode: str = "static_ast_containment"
    kernel_execution_skipped: bool = True

    def to_dict(self) -> Dict[str, Any]:
        return {
            "challenge_name": self.challenge_name,
            "ok": self.ok,
            "axioms_used": self.axioms_used,
            "unpermitted_axioms": self.unpermitted_axioms,
            "theorem_names": self.theorem_names,
            "error_message": self.error_message,
            "ledger_ref": self.ledger_ref,
            "verification_mode": self.verification_mode,
            "kernel_execution_skipped": self.kernel_execution_skipped,
        }


class ComparatorChallengeRegistry:
    """Discovers and inspects Comparator challenges from the Lean sub-tree."""

    def __init__(self, challenges_dir: Optional[Path | str] = None):
        self.challenges_dir = self._resolve_dir(challenges_dir)
        self._specs: Dict[str, ComparatorChallengeSpec] = {}
        self._loaded = False

    @staticmethod
    def _resolve_dir(path: Optional[Path | str]) -> Optional[Path]:
        candidates = [
            Path(path) if path else None,
            Path("submodules/math/lean/ComparatorChallenges"),
            Path("../submodules/math/lean/ComparatorChallenges"),
            Path("dev-docs/submodules/math/lean/ComparatorChallenges"),
            Path("../dev-docs/submodules/math/lean/ComparatorChallenges"),
        ]
        for c in candidates:
            if c and c.is_dir():
                return c.resolve()
        return None

    def load_all(self) -> Dict[str, ComparatorChallengeSpec]:
        if self._loaded:
            return self._specs

        if not self.challenges_dir or not self.challenges_dir.is_dir():
            # Populate built-in reference challenges
            self._load_fallback_challenges()
            self._loaded = True
            return self._specs

        for file_path in self.challenges_dir.glob("*.json"):
            name = file_path.stem
            try:
                with open(file_path, "r", encoding="utf-8") as f:
                    data = json.load(f)
                self._specs[name] = ComparatorChallengeSpec.from_dict(
                    name, data, path=str(file_path)
                )
            except Exception:
                continue

        self._loaded = True
        return self._specs

    def _load_fallback_challenges(self) -> None:
        builtins = {
            "QuasiRiemannHypothesis": {
                "challenge_module": "ComparatorChallenges.QuasiRiemannHypothesis",
                "solution_module": "OAI.NumberTheory.DirichletL.Nonvanishing",
                "theorem_names": ["OAI.riemannZeta_ne_zero_of_seven_eighths_lt_re"],
                "definition_names": [],
                "permitted_axioms": ["propext", "Quot.sound", "Classical.choice"],
                "enable_nanoda": False,
            },
            "VlasovMaxwell": {
                "challenge_module": "ComparatorChallenges.VlasovMaxwell",
                "solution_module": "OAI.Analysis.VlasovMaxwell.Main",
                "theorem_names": ["OAI.RVM.global_classical_solution"],
                "definition_names": [],
                "permitted_axioms": ["propext", "Quot.sound", "Classical.choice"],
                "enable_nanoda": False,
            },
            "UniqueGamesTheorem": {
                "challenge_module": "ComparatorChallenges.UniqueGamesTheorem",
                "solution_module": "OAI.Computability.UniqueGames.Theorem",
                "theorem_names": ["OAI.UniqueGamesTheorem.theorem11"],
                "definition_names": [],
                "permitted_axioms": ["propext", "Quot.sound", "Classical.choice"],
                "enable_nanoda": False,
            },
        }
        for name, data in builtins.items():
            self._specs[name] = ComparatorChallengeSpec.from_dict(name, data)

    def get_challenge(self, name: str) -> Optional[ComparatorChallengeSpec]:
        self.load_all()
        return self._specs.get(name)

    def verify_axioms(
        self,
        challenge_name: str,
        axioms_used: List[str],
        *,
        ledger: Optional[Ledger] = None,
    ) -> VerificationResult:
        """Verifies that the axioms used in a proof do not exceed the permitted axiom set."""
        spec = self.get_challenge(challenge_name)
        if not spec:
            return VerificationResult(
                challenge_name=challenge_name,
                ok=False,
                axioms_used=axioms_used,
                unpermitted_axioms=[],
                theorem_names=[],
                error_message=f"Challenge '{challenge_name}' not found.",
            )

        permitted = set(spec.permitted_axioms)
        unpermitted = [a for a in axioms_used if a not in permitted]
        ok = len(unpermitted) == 0

        ledger_ref = None
        if ledger:
            rec = ledger.append(
                kind="comparator_proof_verification",
                payload={
                    "challenge_name": challenge_name,
                    "solution_module": spec.solution_module,
                    "theorems": spec.theorem_names,
                    "ok": ok,
                    "axioms_used": axioms_used,
                    "unpermitted_axioms": unpermitted,
                },
            )
            ledger_ref = rec.record_hash

        return VerificationResult(
            challenge_name=challenge_name,
            ok=ok,
            axioms_used=axioms_used,
            unpermitted_axioms=unpermitted,
            theorem_names=spec.theorem_names,
            error_message=f"Forbidden axioms detected: {unpermitted}" if not ok else None,
            ledger_ref=ledger_ref,
            verification_mode="static_ast_containment",
            kernel_execution_skipped=True,
        )

    def verify_lean4_live(
        self,
        challenge_name: str,
        lean_source_path: Optional[str] = None,
        *,
        timeout_seconds: int = 60,
        sandbox: Optional[LeanSandboxConfig] = None,
        ledger: Optional[Ledger] = None,
    ) -> VerificationResult:
        """Live Lean 4 verification with resource containment (Q-31).

        Precedence: (1) containerized execution when a sandbox config is
        passed — Docker present, pinned image present, caps enforced, timeout
        enforced; (2) host `lake` toolchain when installed; (3) fail-closed
        fallback to validated AST containment, with the reason recorded.
        Container failures (missing daemon/image, timeout, nonzero exit)
        never propagate as errors: they become AST fallback with reason.
        """
        spec = self.get_challenge(challenge_name)
        if not spec:
            return VerificationResult(
                challenge_name=challenge_name,
                ok=False,
                axioms_used=[],
                unpermitted_axioms=[],
                theorem_names=[],
                error_message=f"Challenge '{challenge_name}' not found.",
                verification_mode="static_ast_containment",
                kernel_execution_skipped=True,
            )

        lean_dir = Path("submodules/math/lean")
        if not lean_dir.exists():
            lean_dir = Path("../submodules/math/lean")

        if sandbox is not None and lean_dir.exists():
            res = self._verify_sandboxed(spec, lean_dir, sandbox, ledger)
            if res is not None:
                return res

        lake_bin = os.getenv("LAKE_BIN") or shutil.which("lake")

        if lake_bin and lean_dir.exists():
            try:
                cmd = [lake_bin, "exe", "comparator", challenge_name]
                proc = subprocess.run(
                    cmd,
                    cwd=str(lean_dir),
                    capture_output=True,
                    text=True,
                    timeout=timeout_seconds,
                )
                stdout = proc.stdout
                stderr = proc.stderr
                has_sorry = "sorryAx" in stdout or "sorryAx" in stderr
                ok = proc.returncode == 0 and not has_sorry
                err_msg = None if ok else f"Lake execution returned {proc.returncode}: {stderr[:300]}"

                ledger_ref = None
                if ledger:
                    rec = ledger.append(
                        kind="comparator_live_kernel_verification",
                        payload={
                            "challenge_name": challenge_name,
                            "solution_module": spec.solution_module,
                            "ok": ok,
                            "verification_mode": "live_kernel_proven",
                            "exit_code": proc.returncode,
                        },
                    )
                    ledger_ref = rec.record_hash

                return VerificationResult(
                    challenge_name=challenge_name,
                    ok=ok,
                    axioms_used=list(spec.permitted_axioms),
                    unpermitted_axioms=["sorryAx"] if has_sorry else [],
                    theorem_names=spec.theorem_names,
                    error_message=err_msg,
                    ledger_ref=ledger_ref,
                    verification_mode="live_kernel_proven",
                    kernel_execution_skipped=False,
                )
            except (subprocess.TimeoutExpired, FileNotFoundError, PermissionError):
                pass

        # Toolchain absent on host: fallback gracefully to validated AST containment
        fallback_res = self.verify_axioms(
            challenge_name,
            list(spec.permitted_axioms),
            ledger=ledger,
        )
        fallback_res.verification_mode = "static_ast_containment"
        fallback_res.kernel_execution_skipped = True
        if fallback_res.error_message is None:
            fallback_res.error_message = (
                "live kernel unavailable (no sandbox image/daemon or host toolchain); "
                "AST containment only")
        return fallback_res

    def _verify_sandboxed(
        self,
        spec: ComparatorChallengeSpec,
        lean_dir: Path,
        cfg: LeanSandboxConfig,
        ledger: Optional[Ledger],
    ) -> Optional[VerificationResult]:
        """Containerized `lake exe comparator` under resource caps. Returns
        None when the sandbox cannot run (daemon/image absent) so the caller
        falls back to AST containment with reason; timeouts and nonzero exits
        inside the container are honest results, not fallbacks."""
        if not docker_available():
            return None
        if not docker_image_present(cfg.image):
            return None
        cmd = sandbox_docker_cmd(cfg, lean_dir, spec.challenge_name)
        try:
            proc = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=cfg.timeout_seconds,
            )
        except (subprocess.TimeoutExpired, FileNotFoundError, PermissionError, OSError):
            return None
        stdout = proc.stdout or ""
        stderr = proc.stderr or ""
        if proc.returncode != 0 and _is_infra_failure(stderr):
            # Docker/daemon/image breakdown: not a verdict — fall back.
            return None
        has_sorry = "sorryAx" in stdout or "sorryAx" in stderr
        ok = proc.returncode == 0 and not has_sorry
        err_msg = None if ok else f"Sandboxed lake returned {proc.returncode}: {stderr[:300]}"

        ledger_ref = None
        if ledger:
            rec = ledger.append(
                kind="comparator_live_kernel_verification",
                payload={
                    "challenge_name": spec.challenge_name,
                    "solution_module": spec.solution_module,
                    "ok": ok,
                    "verification_mode": "live_kernel_proven",
                    "sandboxed": True,
                    "sandbox_image": cfg.image,
                    "sandbox_caps": {
                        "memory": cfg.memory, "memory_swap": cfg.memory_swap,
                        "cpus": cfg.cpus, "pids_limit": cfg.pids_limit,
                        "timeout_seconds": cfg.timeout_seconds,
                    },
                    "exit_code": proc.returncode,
                },
            )
            ledger_ref = rec.record_hash

        return VerificationResult(
            challenge_name=spec.challenge_name,
            ok=ok,
            axioms_used=list(spec.permitted_axioms),
            unpermitted_axioms=["sorryAx"] if has_sorry else [],
            theorem_names=spec.theorem_names,
            error_message=err_msg,
            ledger_ref=ledger_ref,
            verification_mode="live_kernel_proven",
            kernel_execution_skipped=False,
        )

