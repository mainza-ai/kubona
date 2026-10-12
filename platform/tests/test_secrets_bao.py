"""OpenBao secret sourcing: AppRole login, KV reads, fail-closed semantics.

No live vault needed: httpx is stubbed at the call boundary. Cases pin the
production contract — authoritative Bao when BAO_ADDR is set (outages raise,
never fall back to env), environment otherwise, and the Qiskit submit path
actually consulting Bao.
"""

import pytest

from kubona import secrets as secrets_mod
from kubona.secrets import OpenBaoSecretSource, SecretStore


class _Resp:
    def __init__(self, status_code=200, payload=None, text=""):
        self.status_code = status_code
        self._payload = payload
        self.text = text or ""

    def json(self):
        if isinstance(self._payload, Exception):
            raise self._payload
        return self._payload


def _stub(monkeypatch, post=None, get=None, put=None):
    calls = []

    def fake_post(url, json=None, timeout=None):
        calls.append(("POST", url, json))
        return post(url, json) if post else _Resp(500)

    def fake_get(url, headers=None, timeout=None):
        calls.append(("GET", url, headers))
        return get(url, headers) if get else _Resp(500)

    def fake_put(url, headers=None, json=None, timeout=None):
        calls.append(("PUT", url, json))
        return put(url, json) if put else _Resp(500)

    monkeypatch.setattr(secrets_mod.httpx, "post", fake_post)
    monkeypatch.setattr(secrets_mod.httpx, "get", fake_get)
    monkeypatch.setattr(secrets_mod.httpx, "put", fake_put)
    return calls


def _approle_ok(url, body):
    assert url.endswith("/v1/auth/approle/login"), url
    assert body == {"role_id": "role-1", "secret_id": "secret-1"}, body
    return _Resp(200, {"auth": {"client_token": "tok-abc", "lease_duration": 3600}})


def _client(monkeypatch, get=None, put=None):
    calls = _stub(
        monkeypatch,
        post=_approle_ok,
        get=get or (lambda url, headers: _Resp(200, {"data": {"data": {"token": "bao-ibm-token"}}})),
        put=put,
    )
    src = OpenBaoSecretSource(addr="http://bao:8200", role_id="role-1", secret_id="secret-1")
    return src, calls


def test_approle_login_and_kv_read(monkeypatch):
    calls = _stub(
        monkeypatch,
        post=_approle_ok,
        get=lambda url, headers: (
            _Resp(200, {"data": {"data": {"token": "bao-ibm-token"}}})
            if url.endswith("/v1/secret/data/quantum/ibm-qiskit") and headers == {"X-Vault-Token": "tok-abc"}
            else _Resp(404)
        ),
    )
    src = OpenBaoSecretSource(addr="http://bao:8200/", role_id="role-1", secret_id="secret-1")
    assert src.get("ibm-qiskit", "token") == "bao-ibm-token"
    methods = [c[0] for c in calls]
    assert methods == ["POST", "GET"], calls


def test_missing_secret_returns_none_not_failure(monkeypatch):
    src, _ = _client(monkeypatch, get=lambda url, headers: _Resp(404))
    assert src.get("ibm-qiskit", "token") is None


def test_outage_raises_never_falls_back(monkeypatch):
    def down(url, headers=None, **kw):
        raise ConnectionError("simulated outage")

    src, _ = _client(monkeypatch, get=down)
    with pytest.raises(ConnectionError):
        src.get("ibm-qiskit", "token")


def test_auth_refused_raises(monkeypatch):
    _stub(
        monkeypatch,
        post=lambda url, body: _Resp(403, text="permission denied"),
        get=lambda url, headers: _Resp(200, {"data": {"data": {}}}),
    )
    src = OpenBaoSecretSource(addr="http://bao:8200", role_id="r", secret_id="s")
    with pytest.raises(PermissionError):
        src.get("x", "y")


def test_credentials_required_up_front():
    with pytest.raises(ValueError):
        OpenBaoSecretSource(addr="http://bao:8200")


def test_revoke_posts_sys_endpoint(monkeypatch):
    src, calls = _client(monkeypatch, put=lambda url, body: _Resp(204))
    src.revoke_lease("lease-123")
    puts = [c for c in calls if c[0] == "PUT"]
    assert len(puts) == 1 and puts[0][1].endswith("/v1/sys/leases/revoke")
    assert puts[0][2] == {"lease_id": "lease-123"}
    with pytest.raises(ValueError):
        src.revoke_lease("")


def test_from_env_selects_backend(monkeypatch):
    monkeypatch.delenv("BAO_ADDR", raising=False)
    assert isinstance(SecretStore.from_env().source, secrets_mod.EnvSecretSource)
    monkeypatch.setenv("BAO_ADDR", "http://bao:8200")
    with pytest.raises(ValueError):
        SecretStore.from_env()  # addr without creds fails fast, never half-builds
    monkeypatch.setenv("BAO_ROLE_ID", "role-1")
    monkeypatch.setenv("BAO_SECRET_ID", "secret-1")
    store = SecretStore.from_env()
    assert isinstance(store.source, OpenBaoSecretSource)


def test_qiskit_submit_path_consults_bao(monkeypatch):
    qiskit = pytest.importorskip("qiskit")
    del qiskit
    from kubona.providers import get_provider

    _stub(
        monkeypatch,
        post=_approle_ok,
        get=lambda url, headers: _Resp(200, {"data": {"data": {"token": "bao-ibm-token"}}}),
    )
    monkeypatch.setenv("BAO_ADDR", "http://bao:8200")
    monkeypatch.setenv("BAO_ROLE_ID", "role-1")
    monkeypatch.setenv("BAO_SECRET_ID", "secret-1")
    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    provider = get_provider("qiskit-aer-simulator")
    assert provider._credential() == "bao-ibm-token"


def test_qiskit_submit_path_env_without_bao(monkeypatch):
    qiskit = pytest.importorskip("qiskit")
    del qiskit
    from kubona.providers import get_provider

    monkeypatch.delenv("BAO_ADDR", raising=False)
    monkeypatch.setenv("KUBONA_IBM_QISKIT_TOKEN", "env-token")
    provider = get_provider("qiskit-aer-simulator")
    assert provider._credential() == "env-token"


def test_qiskit_credential_sees_rotation_without_restart(monkeypatch):
    """R12: the provider must not cache SecretStore.from_env(). Caching
    froze the SOURCE SELECTION: a BAO_ADDR set after the first submit
    stayed invisible, so Bao-side rotation/revocation would never bite
    until restart. Now the store is built per call."""
    import types

    from kubona.providers.qiskit_provider import QiskitProvider

    monkeypatch.delenv("BAO_ADDR", raising=False)
    monkeypatch.delenv("BAO_TOKEN", raising=False)
    provider = QiskitProvider(types.SimpleNamespace(backend=None))
    monkeypatch.setenv("KUBONA_IBM_QISKIT_TOKEN", "env-A")
    assert provider._credential() == "env-A"
    # Operator points the fleet at Bao mid-process: the next credential
    # read must go to Bao (fail-closed here — nothing listens) instead
    # of serving the stale env credential. A cached store returns env-A.
    monkeypatch.setenv("BAO_ADDR", "http://127.0.0.1:9")
    monkeypatch.setenv("BAO_TOKEN", "dev-override")
    with pytest.raises(ConnectionError):
        provider._credential()
    # An explicitly injected store is still honored verbatim.
    injected = QiskitProvider(types.SimpleNamespace(backend=None),
                              secrets=SecretStore())
    assert injected._credential() == "env-A"
