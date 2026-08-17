"""Identity groundwork: keypairs, delegation, instant revocation."""

import pytest

from kubona.identity import IdentityRegistry, create_identity


@pytest.fixture()
def reg(tmp_path):
    return IdentityRegistry(path=str(tmp_path / "identity.json"))


def test_register_and_agents_of(reg):
    pub_user, _ = create_identity()
    pub_agent, _ = create_identity()
    reg.register("user-1", "user", pub_user)
    reg.register("agent-1", "agent", pub_agent, owner="user-1")
    assert reg.agents_of("user-1")[0].sub == "agent-1"


def test_delegation_gives_access(reg):
    pub_user, _ = create_identity()
    pub_agent, _ = create_identity()
    reg.register("user-1", "user", pub_user)
    reg.register("agent-1", "agent", pub_agent, owner="user-1")
    reg.delegate("user-1", "agent-1", scopes=("submit_job", "get_result"))
    assert reg.access("agent-1") == ("submit_job", "get_result")


def test_revoke_cascade(reg):
    pub_user, _ = create_identity()
    for i in range(3):
        pub_a, _ = create_identity()
        reg.register(f"user-1", "user", pub_user)
        reg.register(f"agent-{i}", "agent", pub_a, owner="user-1")
        reg.delegate("user-1", f"agent-{i}", scopes=("submit_job",))
    revoked = reg.revoke_all_for("user-1")
    assert len(revoked) == 3
    for i in range(3):
        assert reg.access(f"agent-{i}") is None  # instantly revoked, no per-agent surgery


def test_expired_delegation(reg):
    import time
    pub_user, _ = create_identity()
    pub_agent, _ = create_identity()
    reg.register("u", "user", pub_user)
    reg.register("a", "agent", pub_agent, owner="u")
    reg.delegate("u", "a", scopes=("x",), expires_at=time.time() - 1)
    assert reg.access("a") is None


def test_signature_verify(reg):
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives import serialization
    priv = Ed25519PrivateKey.generate()
    pub_hex = priv.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw).hex()
    reg.register("u", "user", pub_hex)
    msg = b"delegation proof"
    sig = priv.sign(msg)
    assert reg.verify_signature("u", msg, sig) is True
    assert reg.verify_signature("u", msg, b"bad" * 16) is False
