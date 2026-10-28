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


def test_update_allowlist(tmp_path):
    """W0.13: arbitrary setattr is gone — ceilings, spend, and ids are not
    mutable via update; bad statuses refuse."""
    import pytest
    store = SessionStore(path=str(tmp_path / "s.jsonl"))
    s = store.create("v", "agent", budget_ceiling_usd=50.0)
    for bad in ("spent_usd", "budget_ceiling_usd", "owner", "created_at"):
        with pytest.raises(ValueError):
            store.update(s.session_id, **{bad: 0})
    with pytest.raises(ValueError):
        store.update(s.session_id, status="archived")
    store.update(s.session_id, name="renamed")
    assert store.get(s.session_id).name == "renamed"


def test_spend_validation(tmp_path):
    """W0.13: negative/NaN/zero spend refuses (no silent unwinding); poisoned
    ceilings refuse at create."""
    import math
    import pytest
    store = SessionStore(path=str(tmp_path / "s.jsonl"))
    s = store.create("v", "agent", budget_ceiling_usd=50.0)
    for bad in (0.0, -5.0, math.nan, math.inf):
        with pytest.raises(ValueError):
            store.record_spend(s.session_id, bad)
    for bad_ceiling in (-1.0, math.nan, math.inf):
        with pytest.raises(ValueError):
            store.create("v", "agent", budget_ceiling_usd=bad_ceiling)


def test_release_spend_refunds_with_reason_and_no_negative_books(tmp_path):
    """R-32 mechanism: audited refund primitive. Explicit amount, mandatory
    reason, no-negative-books clamp — mirrors the Go tenant RefundSpend."""
    import pytest
    store = SessionStore(path=str(tmp_path / "s.jsonl"))
    s = store.create("v", "agent", budget_ceiling_usd=50.0)
    store.record_spend(s.session_id, 5.0)
    s2 = store.release_spend(s.session_id, 2.0, "vendor error, job never ran")
    assert s2.spent_usd == 3.0
    # Zero, negative, NaN refuse; empty reason refuses; over-refund refuses.
    import math
    for bad in (0.0, -1.0, math.nan, math.inf):
        with pytest.raises(ValueError):
            store.release_spend(s.session_id, bad, "r")
    with pytest.raises(ValueError):
        store.release_spend(s.session_id, 1.0, "")
    with pytest.raises(ValueError):
        store.release_spend(s.session_id, 1.0, "   ")
    with pytest.raises(ValueError, match="no negative books"):
        store.release_spend(s.session_id, 4.0, "too much")
    assert store.get(s.session_id).spent_usd == 3.0


def test_concurrent_spend_no_overshoot(tmp_path):
    """W0.13: two concurrent spends serialize under the ceiling."""
    import threading
    store = SessionStore(path=str(tmp_path / "s.jsonl"))
    s = store.create("v", "agent", budget_ceiling_usd=10.0)
    won = []

    def spend():
        try:
            store.record_spend(s.session_id, 6.0)
            won.append(True)
        except ValueError:
            won.append(False)

    ts = [threading.Thread(target=spend) for _ in range(4)]
    [t.start() for t in ts]
    [t.join() for t in ts]
    assert won.count(True) == 1, f"exactly one 6.0 spend fits in 10.0: {won}"
    assert store.get(s.session_id).spent_usd == 6.0


def test_corrupt_lines_quarantined(tmp_path):
    """W0.13: one corrupt line never bricks the store."""
    path = tmp_path / "s.jsonl"
    store = SessionStore(path=str(path))
    s = store.create("v", "agent")
    with open(path, "a") as f:
        f.write("not-json{{{\n")
    assert store.get(s.session_id).session_id == s.session_id


def test_tenant_slug_and_labs(tmp_path):
    """W0.13: hostile tenant names refuse; labs share the slug rule and
    cannot collide."""
    import pytest
    from kubona.session import tenant_slug
    assert tenant_slug("Lab-A") == "Lab-A"
    for bad in ("", "   ", "...", "..", "x" * 65, "/etc/passwd"[:0] or "///"):
        with pytest.raises(ValueError):
            tenant_slug(bad)
    from kubona.labs import LabRegistry
    labs = LabRegistry(path=str(tmp_path / "labs.json"))
    a = labs.create("Lab A", "owner")
    assert a.tenant == "LabA"
    with pytest.raises(ValueError):
        labs.create("Lab A", "owner")  # same slug collides


def test_record_job_concurrent_appends_do_not_lose_ids(tmp_path):
    import threading
    from kubona.session import SessionStore
    store = SessionStore(path=str(tmp_path / "sessions.jsonl"))
    sess = store.create("race", "owner")
    threads = [threading.Thread(target=store.record_job, args=(sess.session_id, f"job-{i}"))
               for i in range(12)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    got = store.get(sess.session_id)
    assert set(got.jobs) == {f"job-{i}" for i in range(12)}


def test_lab_registry_concurrent_creates_do_not_lose_rows(tmp_path):
    import threading
    from kubona.labs import LabRegistry
    labs = LabRegistry(path=str(tmp_path / "labs.json"))
    errors = []

    def make(i):
        try:
            labs.create(f"Lab {i}", "owner")
        except Exception as e:  # noqa: BLE001
            errors.append(e)

    threads = [threading.Thread(target=make, args=(i,)) for i in range(8)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert not errors, errors
    assert len(labs.all()) == 8
