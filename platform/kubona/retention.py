"""I2 — size-based JSONL retention (single source).

jobs.jsonl / sessions.jsonl are append-only and unbounded (the S2
follow-up). When a file exceeds MAX_JSONL_BYTES it rotates:
base -> base.1 -> base.2 (beyond KEEP_SEGMENTS the oldest drops).
Readers sweep all segments oldest-first so newer snapshots win.

RULES (both callers obey — never a second copy of this logic):
- maybe_rotate() runs INSIDE the caller's lock (rotation is a
  read-modify-rename; unlocked rotation races appends). It assumes the
  lock is held and documents it.
- Rotation checks size BEFORE the append (the triggering write lands in
  a fresh file, never in an over-cap one).
- Segments sort base, .1, .2 (base is newest). rotated_paths() returns
  read order: oldest first.
"""

from __future__ import annotations

import os
from pathlib import Path

MAX_JSONL_BYTES = 8 * 1024 * 1024
KEEP_SEGMENTS = 3


def segment_path(path: str | os.PathLike, n: int) -> Path:
    """Segment n of path (n=0 is the base file itself)."""
    p = Path(path)
    if n == 0:
        return p
    return p.parent / f"{p.name}.{n}"


def rotated_paths(path: str | os.PathLike, keep: int = KEEP_SEGMENTS) -> list[Path]:
    """Existing segments in READ order (oldest first, base last).

    Scans every managed index (base + .1 ... .keep) instead of stopping
    at the first gap, so a crashed shift can never orphan a segment
    from reads. Loaders replay oldest-first so newer snapshots win.
    """
    found = [seg for n in range(keep, 0, -1) if (seg := segment_path(path, n)).exists()]
    found.append(segment_path(path, 0))
    return [p for p in found if p.exists()]


def maybe_rotate(
    path: str | os.PathLike,
    max_bytes: int | None = None,
    keep: int | None = None,
) -> bool:
    """Rotate path when over cap: base -> .1 -> ... -> .keep, with the
    oldest backup falling off (bounded total, never silent growth).
    CALLER MUST HOLD ITS LOCK (rotation is read-modify-rename).
    Returns True when a rotation happened. Missing/under-cap files
    return False; I/O errors propagate (a silent rotation failure
    would grow the file it claims to bound).

    Caps resolve at CALL time (None = module consts) so tests pin tiny
    values without forking the production constants.
    """
    if max_bytes is None:
        max_bytes = MAX_JSONL_BYTES
    if keep is None:
        keep = KEEP_SEGMENTS
    p = Path(path)
    try:
        if not p.exists() or p.stat().st_size <= max_bytes:
            return False
    except OSError:
        return False
    # Drop the oldest backup first so no rename clobbers a live segment,
    # then shift down from the top (each target is free by construction).
    doomed = segment_path(path, keep)
    if doomed.exists():
        doomed.unlink()
    for n in range(keep, 0, -1):
        src = segment_path(path, n - 1)
        if src.exists():
            src.rename(segment_path(path, n))
    return True
