"""Pin a vendor job id before the bridge acknowledges the call.

The line is appended and fsynced before the caller waits for a result. A
crash after the vendor accepts, and before the RPC returns, still leaves a
record the Go daemon can copy onto jobs.vendor_ref. Writes happen only when
a Go job id is bound, so provider unit tests do not touch the state dir.
"""

from __future__ import annotations

import contextvars
import json
import os
import time
from pathlib import Path

_ctx: contextvars.ContextVar[tuple[str, str]] = contextvars.ContextVar(
    "kubona_vendor_pin", default=("", "")
)


def pin_path() -> Path:
    root = os.environ.get("KUBONA_STATE_DIR", "").strip()
    base = Path(root) if root else Path.home() / ".kubona"
    return base / "vendor_pins.jsonl"


def bind_vendor_pin(go_job_id: str, session_id: str):
    return _ctx.set((go_job_id or "", session_id or ""))


def reset_vendor_pin(token) -> None:
    _ctx.reset(token)


def persist_vendor_job(vendor_job_id: str) -> None:
    go_job_id, session_id = _ctx.get()
    vendor_job_id = str(vendor_job_id or "").strip()
    if not go_job_id or not vendor_job_id:
        return
    path = pin_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    line = json.dumps(
        {
            "vendor_job_id": vendor_job_id,
            "go_job_id": go_job_id,
            "session_id": session_id,
            "pinned_at": time.time(),
        },
        separators=(",", ":"),
    ) + "\n"
    with path.open("a", encoding="utf-8") as handle:
        handle.write(line)
        handle.flush()
        os.fsync(handle.fileno())
