"""Hybrid PQC transport tests: X25519 + ML-KEM768 handshake, tamper detection."""

import pytest

from kubona.security import HYBRID_SUITE, HybridKeyPair, handshake_initiator, handshake_responder
from kubona.security.hybrid_transport import TransportError


def test_hybrid_handshake_and_roundtrip():
    alice, bob = HybridKeyPair.generate(), HybridKeyPair.generate()
    session_a, ct = handshake_initiator(alice, bob.public_bytes())
    session_b = handshake_responder(bob, alice.public_bytes(), ct)

    assert session_a.suite == HYBRID_SUITE == session_b.suite
    msg = b"harvest-now-decrypt-later: PQC protects this"
    enc = session_a.encrypt(msg, aad=b"tool-call")
    assert session_b.decrypt(enc, aad=b"tool-call") == msg
    # both directions share the key
    enc2 = session_b.encrypt(b"reply")
    assert session_a.decrypt(enc2) == b"reply"


def test_wrong_peer_fails():
    alice, bob = HybridKeyPair.generate(), HybridKeyPair.generate()
    mallory = HybridKeyPair.generate()
    session_a, ct = handshake_initiator(alice, bob.public_bytes())
    # bob's twin with a different key cannot derive the same session
    session_wrong = handshake_responder(mallory, alice.public_bytes(), ct)
    enc = session_a.encrypt(b"secret")
    with pytest.raises(TransportError):
        session_wrong.decrypt(enc)


def test_tamper_detected():
    alice, bob = HybridKeyPair.generate(), HybridKeyPair.generate()
    session_a, ct = handshake_initiator(alice, bob.public_bytes())
    session_b = handshake_responder(bob, alice.public_bytes(), ct)
    enc = session_a.encrypt(b"data")
    tampered = enc[:-1] + bytes([enc[-1] ^ 0xFF])
    with pytest.raises(TransportError):
        session_b.decrypt(tampered)


def test_ciphertext_carries_nonce():
    alice, bob = HybridKeyPair.generate(), HybridKeyPair.generate()
    session_a, ct = handshake_initiator(alice, bob.public_bytes())
    e1, e2 = session_a.encrypt(b"x"), session_a.encrypt(b"x")
    assert e1 != e2  # fresh nonce per message
    assert e1[:12] != e2[:12]
