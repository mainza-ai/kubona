"""Operator-run signed comparator attestation (A3).

The Lean comparator runs in the pinned Docker sandbox (Q-31). This CLI is the
only supported producer of a `formal_verification` record: it refuses unless
the kernel actually ran (`live_kernel_proven`), signs the canonical payload
with the comparator key, and prints the attestation for the API to verify.

It deliberately does not fall back to static AST containment: a static check
must never become a kernel-proof claim.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from pathlib import Path
from typing import Any, Dict, Optional

from kubona.eval.comparator_eval import (
    ComparatorChallengeRegistry,
    LeanSandboxConfig,
    VerificationResult,
    docker_available,
    docker_image_present,
)


class AttestationError(Exception):
    """A refusal that must not be converted into a weaker record."""


def canonical_json(payload: Dict[str, Any]) -> bytes:
    return json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()


def build_attestation(
    result: VerificationResult,
    *,
    session_id: str,
    challenge: str,
    lean_toolchain: str = "4.34.1",
    challenge_digest: str = "",
) -> Dict[str, Any]:
    """Build the signed payload for a genuine kernel proof.

    Refuses anything weaker than `live_kernel_proven` with a clean axiom set.
    The proof hash is deterministic over the challenge, theorem names, axioms,
    and toolchain, so re-running the same proof yields the same identifier.
    """
    if result.verification_mode != "live_kernel_proven":
        raise AttestationError(
            f"comparator mode {result.verification_mode!r} is not a kernel proof; "
            "static containment cannot be attested as live_kernel_proven")
    if not result.ok:
        raise AttestationError(f"comparator rejected the proof: {result.error_message or 'unknown reason'}")
    if result.unpermitted_axioms:
        raise AttestationError(f"unpermitted axioms: {result.unpermitted_axioms}")
    allowed = {"propext", "Quot.sound", "Classical.choice"}
    outside = sorted(a for a in result.axioms_used if a not in allowed)
    if outside:
        raise AttestationError(f"axioms outside the standard whitelist: {outside}")
    if not session_id.strip():
        raise AttestationError("session_id is required")
    if not challenge.strip():
        raise AttestationError("challenge is required")

    material = "|".join([
        challenge,
        ",".join(sorted(result.theorem_names)),
        ",".join(sorted(result.axioms_used)),
        lean_toolchain,
    ])
    payload = {
        "kind": "formal_verification",
        "session_id": session_id,
        "challenge": challenge,
        "proof_hash": hashlib.sha256(material.encode("utf-8")).hexdigest(),
        "verdict": "live_kernel_proven",
        "sorry_ax_count": 0,
        "axioms_clean": True,
        "axioms_used": sorted(result.axioms_used),
        "theorem_names": sorted(result.theorem_names),
        "lean_toolchain": lean_toolchain,
        "challenge_digest": challenge_digest,
    }
    return payload


def sign_attestation(payload: Dict[str, Any], privkey_hex: str) -> Dict[str, Any]:
    from nacl.signing import SigningKey

    try:
        key = SigningKey(bytes.fromhex(privkey_hex))
    except Exception as exc:  # noqa: BLE001 - refuse malformed key material
        raise AttestationError(f"invalid comparator private key: {exc}") from exc
    signature = key.sign(canonical_json(payload)).signature
    return {
        "payload": payload,
        "signature_hex": signature.hex(),
        "pubkey_hex": key.verify_key.encode().hex(),
    }


def _challenge_digest(lean_dir: Optional[Path], challenge: str) -> str:
    if not lean_dir:
        return ""
    for name in (f"{challenge}.lean", f"{challenge}.json"):
        path = lean_dir / "ComparatorChallenges" / name
        if path.is_file():
            return hashlib.sha256(path.read_bytes()).hexdigest()
    return ""


def main(argv: Optional[list[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--challenge", required=True, help="Comparator challenge name")
    parser.add_argument("--session", required=True, help="Kubona session id the proof is for")
    parser.add_argument("--out", help="write the attestation JSON here (default: stdout)")
    parser.add_argument("--lean-dir", default="submodules/math/lean",
                        help="Lean project root (default: submodules/math/lean)")
    parser.add_argument("--image", default="kubona-lean:4.34.1", help="pinned sandbox image")
    args = parser.parse_args(argv)

    privkey = os.environ.get("KUBONA_COMPARATOR_PRIVKEY_HEX", "").strip()
    if not privkey:
        print("comparator-cli: KUBONA_COMPARATOR_PRIVKEY_HEX is not set", file=sys.stderr)
        return 2
    if not docker_available():
        print("comparator-cli: Docker is not available; refusing to attest a kernel proof", file=sys.stderr)
        return 2
    if not docker_image_present(args.image):
        print(f"comparator-cli: sandbox image {args.image} is absent; build it first "
              "(platform/docker/Dockerfile.lean)", file=sys.stderr)
        return 2

    lean_dir = Path(args.lean_dir)
    registry = ComparatorChallengeRegistry(
        lean_dir / "ComparatorChallenges" if (lean_dir / "ComparatorChallenges").is_dir() else None
    )
    if registry.get_challenge(args.challenge) is None:
        print(f"comparator-cli: unknown challenge {args.challenge!r}", file=sys.stderr)
        return 2

    result = registry.verify_lean4_live(
        args.challenge,
        lean_source_path=str(lean_dir),
        timeout_seconds=LeanSandboxConfig().timeout_seconds,
        sandbox=LeanSandboxConfig(image=args.image),
    )
    try:
        payload = build_attestation(
            result,
            session_id=args.session,
            challenge=args.challenge,
            challenge_digest=_challenge_digest(lean_dir, args.challenge),
        )
        attestation = sign_attestation(payload, privkey)
    except AttestationError as exc:
        print(f"comparator-cli: {exc}", file=sys.stderr)
        return 2

    raw = json.dumps(attestation, indent=2, sort_keys=True)
    if args.out:
        Path(args.out).write_text(raw + "\n", encoding="utf-8")
        print(f"comparator-cli: wrote {args.out}")
    else:
        print(raw)
    return 0


if __name__ == "__main__":  # pragma: no cover - CLI entry point
    raise SystemExit(main())
