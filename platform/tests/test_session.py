"""job_id session abstraction tests."""

from kubona.session import SessionStore


def test_session_lifecycle(tmp_path):
    store = SessionStore(path=str(tmp_path / "sessions.jsonl"))
    s = store.create("vqe-loop", "researcher", budget_ceiling_usd=50.0)
    store.record_job(s.session_id, "job-1")
    store.record_spend(s.session_id, 5.0)
    s2 = store.get(s.session_id)
    assert s2.jobs == ["job-1"] and s2.spent_usd == 5.0
    assert s2.remaining() == 45.0
    store.close(s.session_id)
    assert store.get(s.session_id).status == "closed"


def test_session_persists_across_reload(tmp_path):
    path = str(tmp_path / "sessions.jsonl")
    store = SessionStore(path=path)
    s = store.create("persist", "agent", budget_ceiling_usd=10.0)
    store.record_spend(s.session_id, 2.0)
    store2 = SessionStore(path=path)  # fresh instance, same file
    s2 = store2.get(s.session_id)
    assert s2.spent_usd == 2.0
