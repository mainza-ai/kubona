"""Hybrid post-quantum transport — X25519 + ML-KEM768 (FIPS 203).

The Phase-0 'cryptographic agility' decision pays off here: the transport is a
swappable suite. This suite is the X25519MLKEM768 hybrid pattern: both a
classical ECDH (X25519) and a post-quantum KEM (ML-KEM-768) contribute shared
secrets, combined with HKDF-SHA256 (domain-separated, canonically key-bound)
into AES-256-GCM session keys — so the channel is secure even if one of the two
is broken (harvest-now-decrypt-later mitigation, wiki security-and-pqc).

Handshake: initiator sends (its hybrid pubkey, ML-KEM ciphertext); both sides
derive the same session key. Every ciphertext carries its own fresh 96-bit
nonce, so both directions share one key safely (random-nonce GCM).
"""

from __future__ import annotations

import os
from dataclasses import dataclass

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import mlkem, x25519
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF

HYBRID_SUITE = "X25519+MLKEM768"
_KEM_CONTEXT = b"kubona-hybrid-transport-v1"


class TransportError(RuntimeError):
    pass


@dataclass
class HybridKeyPair:
    """One party's hybrid identity: an X25519 key + an ML-KEM768 key."""

    x25519_priv: x25519.X25519PrivateKey
    mlkem_priv: mlkem.MLKEM768PrivateKey

    @classmethod
    def generate(cls) -> "HybridKeyPair":
        return cls(x25519.X25519PrivateKey.generate(), mlkem.MLKEM768PrivateKey.generate())

    def public_bytes(self) -> bytes:
        x = self.x25519_priv.public_key().public_bytes(
            serialization.Encoding.Raw, serialization.PublicFormat.Raw)
        m = self.mlkem_priv.public_key().public_bytes(
            serialization.Encoding.Raw, serialization.PublicFormat.Raw)
        return len(x).to_bytes(2, "big") + x + len(m).to_bytes(2, "big") + m

    @classmethod
    def from_public_bytes(cls, data: bytes) -> "HybridPublic":
        n1 = int.from_bytes(data[:2], "big")
        x = data[2:2 + n1]
        rest = data[2 + n1:]
        n2 = int.from_bytes(rest[:2], "big")
        m = rest[2:2 + n2]
        return HybridPublic(x=x25519.X25519PublicKey.from_public_bytes(x),
                            m=mlkem.MLKEM768PublicKey.from_public_bytes(m))


@dataclass(frozen=True)
class HybridPublic:
    x: x25519.X25519PublicKey
    m: mlkem.MLKEM768PublicKey


class HybridSession:
    """Established hybrid channel (both directions share one derived key)."""

    def __init__(self, aead_key: bytes, peer_id: bytes):
        self._aead = AESGCM(aead_key)
        self.peer_id = peer_id
        self.suite = HYBRID_SUITE

    def encrypt(self, plaintext: bytes, aad: bytes = b"") -> bytes:
        nonce = os.urandom(12)
        return nonce + self._aead.encrypt(nonce, plaintext, aad)

    def decrypt(self, ciphertext: bytes, aad: bytes = b"") -> bytes:
        try:
            nonce, body = ciphertext[:12], ciphertext[12:]
            return self._aead.decrypt(nonce, body, aad)
        except Exception as e:  # InvalidTag / truncation
            raise TransportError("decryption failed (tampered ciphertext or wrong key)") from e


def _combine(ss_classical: bytes, ss_pqc: bytes, pub_a: bytes, pub_b: bytes) -> bytes:
    """HKDF over both secrets, key-bound to the (canonically sorted) public keys."""
    lo, hi = sorted([pub_a, pub_b])
    info = _KEM_CONTEXT + len(lo).to_bytes(2, "big") + lo + len(hi).to_bytes(2, "big") + hi
    return HKDF(algorithm=hashes.SHA256(), length=32, salt=None, info=info).derive(ss_classical + ss_pqc)


def handshake_initiator(my_kp: HybridKeyPair, peer_pub_bytes: bytes) -> tuple[HybridSession, bytes]:
    """Initiator: returns (session, mlkem_ciphertext to send to the responder)."""
    peer = HybridKeyPair.from_public_bytes(peer_pub_bytes)
    ss_x = my_kp.x25519_priv.exchange(peer.x)
    ss_m, ct = peer.m.encapsulate()  # returns (shared_secret, ciphertext)
    key = _combine(ss_x, ss_m, my_kp.public_bytes(), peer_pub_bytes)
    return HybridSession(key, peer_id=peer_pub_bytes), ct


def handshake_responder(my_kp: HybridKeyPair, peer_pub_bytes: bytes, ct: bytes) -> HybridSession:
    """Responder: completes the handshake with the initiator's pubkey + ciphertext."""
    peer = HybridKeyPair.from_public_bytes(peer_pub_bytes)
    ss_x = my_kp.x25519_priv.exchange(peer.x)
    ss_m = my_kp.mlkem_priv.decapsulate(ct)
    key = _combine(ss_x, ss_m, peer_pub_bytes, my_kp.public_bytes())
    return HybridSession(key, peer_id=peer_pub_bytes)
