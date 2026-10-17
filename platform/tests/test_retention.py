"""I2 retention: size-based rotation with full replay, one shared helper."""

import json

from kubona.jobs import JobStore
from kubona.retention import (
    KEEP_SEGMENTS,
    MAX_JSONL_BYTES,
    maybe_rotate,
    rotated_paths,
    segment_path,
)
from kubona.session import SessionStore


def test_no_rotate_under_cap(tmp_path):
    p = tmp_path / "a.jsonl"
    p.write_text("{}\n")
    assert maybe_rotate(p) is False
    assert rotated_paths(p) == [p]


def test_missing_file_is_not_an_error(tmp_path):
    assert maybe_rotate(tmp_path / "nope.jsonl") is False
    assert rotated_paths(tmp_path / "nope.jsonl") == []


def test_rotate_shifts_and_bounds(tmp_path):
    p = tmp_path / "b.jsonl"
    p.write_text("x" * 100)
    assert maybe_rotate(p, max_bytes=10, keep=2) is True
    assert segment_path(p, 1).read_text() == "x" * 100
    assert not p.exists()  # base restarts empty for the next append
    # Fill again: .1 -> .2, base -> .1 (keep=2 holds .1+.2; the
    # consumed base restarts empty until the next append).
    segment_path(p, 0).write_text("y" * 100)
    assert maybe_rotate(p, max_bytes=10, keep=2) is True
    assert rotated_paths(p, keep=2) == [
        segment_path(p, 2),
        segment_path(p, 1),
    ]
    # Third rotation drops the oldest (bounded total).
    segment_path(p, 0).write_text("z" * 100)
    assert maybe_rotate(p, max_bytes=10, keep=2) is True
    assert segment_path(p, 2).read_text() == "y" * 100
    assert segment_path(p, 1).read_text() == "z" * 100


def test_job_store_replays_rotated_segments(tmp_path):
    import kubona.retention as ret

    store = JobStore(path=str(tmp_path / "jobs.jsonl"))
    j1 = store.create(provider="local", session_id="s")
    # Force rotation with a tiny cap, then keep writing.
    orig = ret.MAX_JSONL_BYTES
    ret.MAX_JSONL_BYTES = 10
    try:
        j2 = store.create(provider="local", session_id="s")
    finally:
        ret.MAX_JSONL_BYTES = orig
    assert (tmp_path / "jobs.jsonl.1").exists()
    loaded = JobStore(path=str(tmp_path / "jobs.jsonl"))._load()
    assert j1.job_id in loaded and j2.job_id in loaded


def test_session_store_replays_rotated_segments(tmp_path):
    import kubona.retention as ret

    store = SessionStore(path=str(tmp_path / "sessions.jsonl"))
    s1 = store.create(name="a", owner="o")
    orig = ret.MAX_JSONL_BYTES
    ret.MAX_JSONL_BYTES = 10
    try:
        s2 = store.create(name="b", owner="o")
    finally:
        ret.MAX_JSONL_BYTES = orig
    assert (tmp_path / "sessions.jsonl.1").exists()
    assert store.get(s1.session_id).name == "a"
    assert store.get(s2.session_id).name == "b"


def test_constants_pinned():
    assert MAX_JSONL_BYTES == 8 * 1024 * 1024
    assert KEEP_SEGMENTS == 3


def test_triggering_write_lands_in_fresh_file(tmp_path):
    import kubona.retention as ret

    store = JobStore(path=str(tmp_path / "jobs.jsonl"))
    store.create(provider="local", session_id="s")
    orig = ret.MAX_JSONL_BYTES
    ret.MAX_JSONL_BYTES = 10
    try:
        store.create(provider="local", session_id="s")
    finally:
        ret.MAX_JSONL_BYTES = orig
    base_lines = (tmp_path / "jobs.jsonl").read_text().strip().splitlines()
    assert len(base_lines) == 1  # rotation preceded the append
    assert len(json.loads(base_lines[0])) > 0
