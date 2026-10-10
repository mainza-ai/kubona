"""Q-20: pack manifest contract, trust, and revocation."""

import pytest

from kubona.science.domain_pack import (
    DomainPackLoader,
    PackError,
    PackRegistry,
    resolve_generator,
)


def _manifest(**over):
    kw = dict(
        name="test-pack",
        version="1.2.3",
        domain_category="chemistry",
        description="Test pack.",
        author="Test Lab",
        classical_anchor_methods=["RHF"],
        benchmark_systems=["H2"],
        hamiltonian_generator="kubona.anchors.chemistry.MolecularAnchorEngine",
        min_qubit_support=2,
    )
    kw.update(over)
    return kw


def test_default_packs_load_and_resolve(tmp_path):
    loader = DomainPackLoader(packs_dir=tmp_path / "packs")
    packs = loader.register_default_packs()
    assert {p.name for p in packs} == {
        "quantum-molecular-chemistry",
        "condensed-matter-hubbard-lattice",
        "mathematical-physics-frontier",
    }
    for p in packs:
        assert p.schema_version == "kubona-pack/v1"
    chem = next(p for p in packs if p.name == "quantum-molecular-chemistry")
    assert resolve_generator(chem.hamiltonian_generator).__name__ == "MolecularAnchorEngine"


def test_non_semver_version_refuses(tmp_path):
    from kubona.science.domain_pack import DomainPackManifest
    with pytest.raises(Exception, match="semver"):
        DomainPackManifest(**_manifest(version="v1"))
    with pytest.raises(Exception, match="semver"):
        DomainPackManifest(**_manifest(version="1.0"))


def test_pack_without_anchor_refuses():
    from kubona.science.domain_pack import DomainPackManifest
    with pytest.raises(Exception, match="classical anchor"):
        DomainPackManifest(**_manifest(classical_anchor_methods=[]))


def test_wrong_schema_version_refuses():
    from kubona.science.domain_pack import DomainPackManifest
    with pytest.raises(Exception, match="schema"):
        DomainPackManifest(**_manifest(schema_version="kubona-pack/v9"))


def test_generator_allowlist_and_phantom():
    with pytest.raises(PackError, match="allowlist"):
        resolve_generator("evil_pkg.Backdoor")
    with pytest.raises(PackError, match="unresolvable"):
        resolve_generator("kubona.science.lattice.HubbardModel")


def test_signature_verifies_and_tamper_refuses():
    from nacl.signing import SigningKey
    from kubona.science.domain_pack import DomainPackManifest
    sk = SigningKey.generate()
    pub = sk.verify_key.encode().hex()
    m = DomainPackManifest(**_manifest(owner_pubkey_hex=pub))
    sig = sk.sign(m.canonical_bytes()).signature.hex()
    signed = DomainPackManifest(**_manifest(owner_pubkey_hex=pub, signature_hex=sig))
    signed.verify_signature()  # must not raise
    tampered = signed.model_copy(update={"version": "9.9.9"})
    with pytest.raises(PackError, match="signature invalid"):
        tampered.verify_signature()


def test_revocation_round_trip(tmp_path):
    reg = PackRegistry(packs_dir=tmp_path / "packs")
    reg.loader.register_default_packs()
    assert len(reg.load_all()) == 3
    reg.revoke("quantum-molecular-chemistry", "superseded by v2")
    assert reg.is_revoked("quantum-molecular-chemistry")
    assert not reg.is_revoked("mathematical-physics-frontier")
    with pytest.raises(PackError, match="revoked"):
        reg.load_all()
    with pytest.raises(PackError, match="reason"):
        reg.revoke("x", "  ")
