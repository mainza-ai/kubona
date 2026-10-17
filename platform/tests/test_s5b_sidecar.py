"""S5b/C2 companion: the Go replay algorithm (prev rewrite + record_hash
formula) yields files the real Python Ledger validates. Mirrors
replayPythonRecords in platform/core/pkg/session/pyrail.go step for step."""

import hashlib
import json

from kubona.provenance import Ledger


def _record_hash(prev: str, content: str) -> str:
    return hashlib.sha256(f"{prev}:{content}".encode()).hexdigest()


def test_go_style_replay_validates(tmp_path):
    src = Ledger(path=str(tmp_path / "src.jsonl"))
    src.append("workflow_step", {"session_id": "s", "step": "a"})
    src.append("workflow_step", {"session_id": "s", "step": "b"})
    src.append("workflow_step", {"session_id": "other", "step": "c"})
    ok, msg = src.validate_chain()
    assert ok, msg

    # Go export: filter session rows, keep full record dicts.
    exported = [
        json.loads(r.to_json())
        for r in src
        if json.loads(r.to_json())["payload"].get("session_id") == "s"
    ]
    assert len(exported) == 2

    # Go replay into a fresh file: only prev_hash + record_hash rewritten.
    tail = "0" * 64
    lines = []
    for rec in exported:
        rec["prev_hash"] = tail
        rec["record_hash"] = _record_hash(tail, rec["content_hash"])
        tail = rec["record_hash"]
        lines.append(rec)

    dst_path = tmp_path / "ledger.default.jsonl"
    dst_path.write_text("\n".join(json.dumps(r, sort_keys=True) for r in lines) + "\n")
    dst = Ledger(path=str(dst_path))
    ok, msg = dst.validate_chain()
    assert ok, msg
    assert dst.count() == 2
    kinds = [r.kind for r in dst]
    assert kinds == ["workflow_step", "workflow_step"]
    # Payload content preserved through the replay.
    assert [r.payload["step"] for r in dst] == ["a", "b"]
