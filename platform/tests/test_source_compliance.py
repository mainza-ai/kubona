"""Q-19: scholarly source license + rate-limit compliance."""

import kubona.integrations.sources as sources
from kubona.integrations.sources import (
    _respect_arxiv_rate_limit,
    ground_literature,
)


class _FakeResp:
    def __init__(self, text="", payload=None):
        self.text = text
        self._payload = payload or {}

    def raise_for_status(self):
        pass

    def json(self):
        return self._payload


class _FakeHTTP:
    def __init__(self, resp):
        self.resp = resp
        self.calls = []

    def get(self, url, params=None, **kwargs):
        self.calls.append({"url": url, "params": params or {}})
        return self.resp


ARXIV_ATOM = """<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry><title>Quantum Krylov methods</title>
<id>http://arxiv.org/abs/2401.00001v1</id></entry>
</feed>"""


def test_arxiv_uses_export_host_and_throttles():
    import kubona.integrations.sources as s
    s._arxiv_last_call = 0.0
    slept = []
    s._respect_arxiv_rate_limit(now=100.0, sleep=slept.append)
    assert slept == []  # first call: no wait
    s._respect_arxiv_rate_limit(now=101.0, sleep=slept.append)
    assert len(slept) == 1 and abs(slept[0] - 2.0) < 1e-9  # 3s rule enforced


def test_arxiv_license_flag_and_export_endpoint():
    fake = _FakeHTTP(_FakeResp(text=ARXIV_ATOM))
    out = ground_literature("krylov", source="arxiv", _http=fake)
    assert fake.calls and "export.arxiv.org" in fake.calls[0]["url"]
    assert "CC0" in out["license"]
    assert "redistribution" in out["license"]
    assert any("2401.00001" in i["url"] for i in out["items"])


def test_openalex_license_and_polite_pool_opt_in(monkeypatch):
    fake = _FakeHTTP(_FakeResp(payload={"results": [
        {"display_name": "A paper", "doi": "10.1234/x", "id": "W1"}]}))
    out = ground_literature("vqe", source="openalex", _http=fake)
    assert out["license"].startswith("CC0")
    assert "mailto" not in fake.calls[0]["params"]  # no invented contact
    monkeypatch.setenv("KUBONA_CONTACT_EMAIL", "ops@example.org")
    out2 = ground_literature("vqe", source="openalex", _http=fake)
    assert fake.calls[-1]["params"].get("mailto") == "ops@example.org"
    assert out2["items"][0]["url"] == "10.1234/x"


def test_every_source_labels_license():
    for name, src in sources.DATA_SOURCES.items():
        assert src.license and src.license != "unknown", name
