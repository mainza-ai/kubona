"""Signed comparator attestation (A3 boundary)."""

from __future__ import annotations

import json

import pytest
from nacl.signing import SigningKey, VerifyKey

from kubona.eval.comparator_cli import (
    AttestationError,
    build_attestation,
    canonical_json,
    main,
    sign_attestation,
)
from kubona.eval.comparator_eval import VerificationResult


def _result(mode="live_kernel_proven", ok=True, axioms=None):
    return VerificationResult(
        challenge_name="Heisenberg",
        ok=ok,
        axioms_used=axioms if axioms is not None else ["propext", "Quot.sound"],
        unpermitted_axioms=[],
        theorem_names=["Demo.theorem"],
        verification_mode=mode,
        kernel_execution_skipped=mode != "live_kernel_proven",
    )


def test_live_kernel_proof_builds_deterministic_payload():
    a = build_attestation(_result(), session_id="s1", challenge="Heisenberg", challenge_digest="d")
    b = build_attestation(_result(), session_id="s1", challenge="Heisenberg", challenge_digest="d")
    assert a == b
    assert a["verdict"] == "live_kernel_proven"
    assert a["sorry_ax_count"] == 0
    assert a["axioms_clean"] is True
    assert len(a["proof_hash"]) == 64
    assert "Heisenberg" in json.dumps(a)


@pytest.mark.parametrize("mode,ok", [
    ("static_ast_containment", True),
    ("verification_unavailable", True),
    ("live_kernel_proven", False),
])
def test_non_kernel_or_failed_results_never_attest(mode, ok):
    with pytest.raises(AttestationError):
        build_attestation(_result(mode=mode, ok=ok), session_id="s1", challenge="Heisenberg")


def test_unpermitted_axioms_refuse():
    r = _result(axioms=["propext", "sorryAx"])
    with pytest.raises(AttestationError, match="whitelist"):
        build_attestation(r, session_id="s1", challenge="Heisenberg")


def test_signature_verifies_over_canonical_payload():
    key = SigningKey.generate()
    payload = build_attestation(_result(), session_id="s1", challenge="Heisenberg")
    doc = sign_attestation(payload, key.encode().hex())
    assert doc["pubkey_hex"] == key.verify_key.encode().hex()
    VerifyKey(bytes.fromhex(doc["pubkey_hex"])).verify(
        canonical_json(payload), bytes.fromhex(doc["signature_hex"])
    )


def test_cli_refuses_without_key(monkeypatch):
    monkeypatch.delenv("KUBONA_COMPARATOR_PRIVKEY_HEX", raising=False)
    assert main(["--challenge", "Heisenberg", "--session", "s1"]) == 2


def test_cli_refuses_without_sandbox(monkeypatch):
    monkeypatch.setenv("KUBONA_COMPARATOR_PRIVKEY_HEX", SigningKey.generate().encode().hex())
    monkeypatch.setattr("kubona.eval.comparator_cli.docker_available", lambda: False)
    assert main(["--challenge", "Heisenberg", "--session", "s1"]) == 2
