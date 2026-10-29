"""W0.9: Python mandates are real Ed25519 over Go-canonical bytes.

The VECTOR below is the same fixture the console (mandates.test.ts) and Go
(TestCrossLanguageMandateVector) assert: seed 000102...0001, payload
'tenant-vector:jobhash-vector:25.5000:2030-01-01T00:00:00Z', pubkey
c99a...0269, signature ea6a...7ed5. All three sides must agree byte-for-byte.
"""

import math
from datetime import datetime, timezone

import pytest
from nacl.signing import SigningKey

from kubona.cost.mandate import (
    canonical_job_binding,
    create_mandate,
    utc_now_plus,
    verify_mandate,
)

VECTOR = {
    "seed": "0001020304050607080900010203040506070809000102030405060708090001",
    "tenant": "tenant-vector",
    "job_hash": "jobhash-vector",
    "budget": 25.5,
    "expiry": "2030-01-01T00:00:00Z",
    "pubkey": "c99aff667c7578056bd7d4590df92bf2ae108a6a7f33dd52384a83bf325f0269",
    "sig": "ea6a9a67b22c82be13f918e22824d1fd7d8f4f6e70a51f14f19112782dbc9e6e2075ba41c3c141955f24278e1cb6b8e0f643fdd0ddc0bc818412e7ed5d97570e",
}

BINDING_QASM = "OPENQASM 2.0;\nqreg q[1];\n"
BINDING_HEX = "67cc6d01173411aecf553a038cc9873a15779416dafd58a30e3c15a98c63bbfb"


def _expiry():
    return datetime(2030, 1, 1, tzinfo=timezone.utc)


def test_cross_language_signature_vector():
    key = SigningKey(bytes.fromhex(VECTOR["seed"]))
    assert key.verify_key.encode().hex() == VECTOR["pubkey"]
    m = create_mandate(
        bytes.fromhex(VECTOR["seed"]),
        VECTOR["tenant"],
        VECTOR["job_hash"],
        VECTOR["budget"],
        _expiry(),
    )
    assert m.public_key_hex == VECTOR["pubkey"]
    assert m.signature_hex == VECTOR["sig"]
    ok, _ = verify_mandate(m, VECTOR["tenant"], VECTOR["job_hash"], VECTOR["budget"])
    assert ok


def test_canonical_binding_vector():
    assert canonical_job_binding("circuit_run", "sess-vec", BINDING_QASM) == BINDING_HEX
    base = canonical_job_binding("circuit_run", "sess-vec", BINDING_QASM)
    assert canonical_job_binding("circuit_run", "sess-other", BINDING_QASM) != base
    assert canonical_job_binding("circuit_run", "sess-vec", BINDING_QASM + "h q[0];\n") != base
    assert canonical_job_binding("mitigate", "sess-vec", BINDING_QASM) != base


def test_verify_expectations():
    import os
    key = os.urandom(32)
    m = create_mandate(key, "lab-a", "job-1", 50.0, _expiry())
    ok, _ = verify_mandate(m, "lab-a", "job-1", 50.0)
    assert ok
    # Wrong tenant / job / over-budget refuse.
    assert verify_mandate(m, "lab-b", "job-1", 10.0)[0] is False
    assert verify_mandate(m, "lab-a", "job-2", 10.0)[0] is False
    assert verify_mandate(m, "lab-a", "job-1", 50.0001)[0] is False
    # Empty expectations refuse (verification against nothing verifies nothing).
    assert verify_mandate(m, "", "job-1", 10.0)[0] is False
    assert verify_mandate(m, "lab-a", "", 10.0)[0] is False
    # Tampered budget refuses without re-signing.
    tampered = m.__class__(m.tenant_id, m.job_hash, 999.0, m.expiry_timestamp,
                           m.public_key_hex, m.signature_hex)
    assert verify_mandate(tampered, "lab-a", "job-1", 10.0)[0] is False


def test_mint_refusals():
    import os
    key = os.urandom(32)
    with pytest.raises(ValueError):
        create_mandate(key, "", "job-1", 10.0, _expiry())
    with pytest.raises(ValueError):
        create_mandate(key, "lab-a", "", 10.0, _expiry())
    for bad in (0.0, -5.0, math.nan, math.inf):
        with pytest.raises(ValueError):
            create_mandate(key, "lab-a", "job-1", bad, _expiry())
    with pytest.raises(ValueError):
        create_mandate(b"short", "lab-a", "job-1", 10.0, _expiry())
    with pytest.raises(ValueError):
        create_mandate(key, "lab-a", "job-1", 10.0,
                        datetime(2020, 1, 1, tzinfo=timezone.utc))
    with pytest.raises(ValueError):
        create_mandate(key, "lab-a", "job-1", 10.0,
                        datetime(2030, 1, 1))  # naive


def test_utc_now_plus():
    assert utc_now_plus(60) > datetime.now(timezone.utc)


def test_fractional_wire_expiry_refuses():
    """Canonical form is whole-second expiry. A wire timestamp carrying
    nanosecond fractions can never match the canonical payload the
    signature covers — it must refuse (fail-closed), even when the
    signature itself is valid over the whole-second form. Found live:
    Go minted with nanos while signing truncated time; the first
    Python-side verification of a Go mandate caught it."""
    import os
    from nacl.signing import SigningKey
    from kubona.cost.mandate import SpendMandate, verify_mandate
    key = os.urandom(32)
    pubhex = SigningKey(key).verify_key.encode().hex()
    whole = "2030-01-01T00:00:00Z"
    canon = f"lab-a:job-1:{50.0:.4f}:{whole}".encode()
    sig = SigningKey(key).sign(canon).signature.hex()
    good = SpendMandate("lab-a", "job-1", 50.0, whole, pubhex, sig)
    assert verify_mandate(good, "lab-a", "job-1", 10.0)[0] is True
    # Same signature bytes, nanos-fraction wire timestamp: the canonical
    # payload differs, so verification must refuse (not "close enough").
    nano = SpendMandate("lab-a", "job-1", 50.0,
                        "2030-01-01T00:00:00.123456789Z", pubhex, sig)
    ok, reason = verify_mandate(nano, "lab-a", "job-1", 10.0)
    assert ok is False, reason
