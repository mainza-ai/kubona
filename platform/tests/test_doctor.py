"""L4 doctor + mandate-check pins: original 5 lines intact, new checks
report (never crash), mandate verdicts separate shape from admission."""

import json
import sqlite3

from kubona.run import _cli, _mandate_check


def _mandate_file(tmp_path, **over):
    import secrets

    from kubona.cost.mandate import create_mandate, utc_now_plus

    m = create_mandate(secrets.token_bytes(32), "t", "h", 25.0, utc_now_plus(3600))
    d = m.to_dict()
    d.update(over)
    p = tmp_path / "m.json"
    p.write_text(json.dumps(d))
    return str(p)


def test_doctor_reports_all_lines(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    assert _cli(["prog", "doctor"]) == 0
    out = capsys.readouterr().out
    for line in ("state root:", "ledger:", "production:", "token configured:",
                 "signing key:", "providers:", "ledger check:",
                 "skill:", "taxonomy:"):
        assert line in out, f"doctor missing {line!r}: {out!r}"


def test_doctor_never_crashes_on_broken_state(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    (tmp_path / "state").mkdir()
    (tmp_path / "state" / "ledger.jsonl").write_text("{corrupt line\n")
    assert _cli(["prog", "doctor"]) == 0
    out = capsys.readouterr().out
    assert "unreadable" in out


def test_mandate_check_shape_verdicts(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    mp = _mandate_file(tmp_path)
    db = str(tmp_path / "missing.db")
    assert _mandate_check(["--mandate", mp, "--tenant", "t", "--job-hash", "h",
                           "--cost", "10", "--db", db]) == 0
    out = json.loads(capsys.readouterr().out)
    assert out["shape_valid"] is True
    assert out["admitted"] is False
    assert "fresh state" in out["admission_note"]

    assert _mandate_check(["--mandate", mp, "--tenant", "t", "--job-hash", "h",
                           "--cost", "99", "--db", db]) == 0
    out = json.loads(capsys.readouterr().out)
    assert out["shape_valid"] is False
    assert "exceeds mandate budget" in out["reason"]
    assert out["admitted"] is False


def test_mandate_check_finds_receipt(tmp_path, monkeypatch, capsys):
    import secrets

    from kubona.cost.mandate import create_mandate, utc_now_plus

    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    m = create_mandate(secrets.token_bytes(32), "t", "h", 25.0, utc_now_plus(3600))
    mp = tmp_path / "m.json"
    mp.write_text(json.dumps(m.to_dict()))
    db = tmp_path / "k.db"
    conn = sqlite3.connect(str(db))
    conn.execute("CREATE TABLE ledger_events (id INTEGER PRIMARY KEY AUTOINCREMENT, event_type TEXT, payload TEXT)")
    conn.execute("INSERT INTO ledger_events (event_type, payload) VALUES (?, ?)",
                 ("mandate_verified_client",
                  json.dumps({"signature_hex": m.signature_hex, "tenant_id": "t"})))
    conn.commit()
    conn.close()
    assert _mandate_check(["--mandate", str(mp), "--tenant", "t", "--job-hash", "h",
                           "--cost", "10", "--db", str(db)]) == 0
    out = json.loads(capsys.readouterr().out)
    assert out["shape_valid"] is True
    assert out["admitted"] is True


def test_mandate_check_usage_errors(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path / "state"))
    assert _mandate_check(["--mandate", str(tmp_path / "nope.json"), "--tenant", "t",
                           "--job-hash", "h", "--cost", "10"]) == 2
    assert _mandate_check([]) == 2
    with __import__("pytest").raises(SystemExit):
        _cli(["prog", "bogus-command"])
