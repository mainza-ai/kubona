"""Hugging Face Model Hub Client & Direct In-Platform Downloader (Phase 3).

Allows searching Hugging Face for quantized models (GGUF, MLX, Safetensors),
inspecting available quantizations, and streaming downloads directly into the
local platform storage with progress tracking and local engine registration.

Invariants:
  1. Sovereign Storage: Downloaded models reside in local air-gapped storage
     (~/.kubona/models or configured dir), ready for zero-egress local inference.
  2. Integrity: Streaming hash computation (SHA-256) ensures uncorrupted weights.
  3. Non-blocking Async Streaming: Background task manager tracks progress,
     speed (MB/s), and allows pause/cancellation.
"""

from __future__ import annotations

import asyncio
import hashlib
import os
import re
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

import httpx

HF_API_BASE = "https://huggingface.co/api"
DEFAULT_MODELS_DIR = Path(os.environ.get("KUBONA_MODELS_DIR", Path.home() / ".kubona" / "models"))

#: Default per-file download cap (W2.16): downloads are multi-GB weights, so
#: the default is generous but bounded — no unbounded sink. Override per
#: manager or per download.
DEFAULT_MAX_DOWNLOAD_BYTES = 50 * 1024 ** 3


class HFHubError(RuntimeError):
    """Typed refusal for Hub failures (W2.16): transport errors and non-2xx
    responses raise this — never a silent [] or None that reads as "no
    models"."""


@dataclass
class HFModelQuantization:
    filename: str
    size_bytes: int | None
    quant_type: str  # e.g. "Q4_K_M", "Q5_K_M", "Q8_0", "FP16", "MLX-4bit"
    download_url: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class HFModelSummary:
    id: str
    author: str
    downloads: int
    likes: int
    pipeline_tag: str
    tags: list[str]
    quantizations: list[HFModelQuantization] = field(default_factory=list)
    description: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "author": self.author,
            "downloads": self.downloads,
            "likes": self.likes,
            "pipeline_tag": self.pipeline_tag,
            "tags": self.tags,
            "quantizations": [q.to_dict() for q in self.quantizations],
            "description": self.description,
        }


@dataclass
class HFDownloadTask:
    task_id: str
    repo_id: str
    filename: str
    status: str  # "pending", "downloading", "completed", "failed", "cancelled"
    bytes_downloaded: int = 0
    total_bytes: int = 0
    progress_percent: float = 0.0
    speed_mb_s: float = 0.0
    local_path: str = ""
    error: str | None = None
    sha256: str = ""
    started_at: float = field(default_factory=time.time)
    completed_at: float | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def _infer_quant_type(filename: str) -> str:
    """Extracts quantization type from filename.

    W2.16: unknown labels return "UNKNOWN" (never a confident-sounding
    default like "GGUF") — and start_download refuses UNKNOWN downstream, so
    unlabeled weights are never fetched on a guessed precision.
    """
    upper = filename.upper()
    if "MLX" in upper:
        if "4BIT" in upper:
            return "MLX-4BIT"
        if "8BIT" in upper:
            return "MLX-8BIT"
        return "MLX"
    if filename.lower().endswith(".safetensors"):
        for q in ["FP16", "BF16", "F32", "4BIT", "8BIT"]:
            if q in upper:
                return q
        return "SAFETENSORS"
    for q in [
        "Q4_K_M", "Q4_K_S", "Q5_K_M", "Q5_K_S", "Q8_0", "Q6_K", "Q2_K", "Q3_K_M",
        "Q3_K_S", "Q3_K_L", "Q4_0", "Q4_1", "Q5_0", "Q5_1", "FP16", "BF16", "F32",
        "4BIT", "8BIT"
    ]:
        if q in upper:
            return q
    return "UNKNOWN"


def sanitize_filename(filename: str, dest_dir: Path) -> str:
    """Fail-closed filename validation (W2.16): rejects path separators,
    traversal, absolute paths, and empty names, then resolve + containment
    check against `dest_dir` (a `dest_dir` symlink or `..` smuggling refuses
    instead of writing outside the model store). Returns the safe name."""
    if not isinstance(filename, str) or not filename or "\x00" in filename:
        raise HFHubError(f"refusing download with invalid filename {filename!r}")
    if ("/" in filename or "\\" in filename or os.sep in filename
            or filename in (".", "..") or filename.startswith("~")):
        raise HFHubError(f"refusing download: filename {filename!r} carries path separators")
    if filename.startswith("/") or (len(filename) > 1 and filename[1] == ":"):
        raise HFHubError(f"refusing download: absolute filename {filename!r}")
    candidate = (dest_dir / filename).resolve()
    try:
        candidate.relative_to(dest_dir.resolve())
    except ValueError:
        raise HFHubError(
            f"refusing download: {filename!r} escapes the model store") from None
    if candidate.name != filename:
        raise HFHubError(f"refusing download: filename {filename!r} normalizes unsafely")
    return filename


_REPO_ID_RE = re.compile(r"^[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+$")


def sanitize_repo_id(repo_id: str) -> str:
    """Fail-closed repo validation (W2.16): Hub ids are owner/name. Anything
    else (traversal, extra slashes, blank, dot-components) refuses."""
    if not isinstance(repo_id, str) or not _REPO_ID_RE.match(repo_id):
        raise HFHubError(f"refusing download: invalid repo id {repo_id!r}")
    owner, _, name = repo_id.partition("/")
    if owner in (".", "..") or name in (".", ".."):
        raise HFHubError(f"refusing download: repo id {repo_id!r} hides traversal")
    return repo_id


class HuggingFaceHubClient:
    """Hugging Face API explorer and model search engine."""

    def __init__(self, timeout: float = 10.0):
        self.timeout = timeout

    async def search_models(
        self,
        query: str = "",
        library: str = "gguf",
        pipeline_tag: str = "text-generation",
        limit: int = 15,
        sort: str = "downloads",
    ) -> list[HFModelSummary]:
        """Searches Hugging Face for models matching query and filters."""
        params: dict[str, Any] = {
            "limit": limit,
            "full": "false",
        }
        if query:
            params["search"] = query
        if library:
            params["filter"] = library
        if pipeline_tag:
            params["pipeline_tag"] = pipeline_tag
        if sort:
            params["sort"] = sort
            params["direction"] = "-1"

        url = f"{HF_API_BASE}/models"
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.get(url, params=params)
        except Exception as e:
            raise HFHubError(f"Hub search unreachable ({url}): {e}") from e
        if resp.status_code != 200:
            raise HFHubError(
                f"Hub search refused (HTTP {resp.status_code}): "
                f"{resp.text[:200]} — never an empty list masquerading as 'no models'")
        data = resp.json()

        results: list[HFModelSummary] = []
        for item in data:
            repo_id = item.get("id", "")
            author = item.get("author", repo_id.split("/")[0] if "/" in repo_id else "")
            downloads = item.get("downloads", 0)
            likes = item.get("likes", 0)
            ptag = item.get("pipeline_tag", "text-generation")
            tags = item.get("tags", [])

            # Extract siblings if present
            siblings = item.get("siblings", [])
            quants: list[HFModelQuantization] = []
            for s in siblings:
                rfile = s.get("rfilename", "")
                if rfile.endswith(".gguf") or "mlx" in rfile.lower():
                    quants.append(
                        HFModelQuantization(
                            filename=rfile,
                            size_bytes=s.get("size"),
                            quant_type=_infer_quant_type(rfile),
                            download_url=f"https://huggingface.co/{repo_id}/resolve/main/{rfile}",
                        )
                    )

            results.append(
                HFModelSummary(
                    id=repo_id,
                    author=author,
                    downloads=downloads,
                    likes=likes,
                    pipeline_tag=ptag,
                    tags=tags,
                    quantizations=quants,
                )
            )
        return results

    async def get_model_details(self, repo_id: str) -> HFModelSummary | None:
        """Fetches detailed file tree and quantizations for a specific repository."""
        url = f"{HF_API_BASE}/models/{repo_id}"
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.get(url)
        except Exception as e:
            raise HFHubError(f"Hub details unreachable for {repo_id!r}: {e}") from e
        if resp.status_code != 200:
            raise HFHubError(
                f"Hub details refused for {repo_id!r} (HTTP {resp.status_code}): "
                f"{resp.text[:200]} — never None masquerading as 'no such model'")
        data = resp.json()

        siblings = data.get("siblings", [])
        quants: list[HFModelQuantization] = []
        for s in siblings:
            rfile = s.get("rfilename", "")
            if rfile.endswith(".gguf") or "mlx" in rfile.lower() or rfile.endswith(".safetensors"):
                quants.append(
                    HFModelQuantization(
                        filename=rfile,
                        size_bytes=s.get("size"),
                        quant_type=_infer_quant_type(rfile),
                        download_url=f"https://huggingface.co/{repo_id}/resolve/main/{rfile}",
                    )
                )

        return HFModelSummary(
            id=data.get("id", repo_id),
            author=data.get("author", repo_id.split("/")[0] if "/" in repo_id else ""),
            downloads=data.get("downloads", 0),
            likes=data.get("likes", 0),
            pipeline_tag=data.get("pipeline_tag", "text-generation"),
            tags=data.get("tags", []),
            quantizations=quants,
            description=data.get("description", ""),
        )


class HFDownloadManager:
    """Direct in-platform model file streaming downloader.

    Integrity discipline (W2.16): every download pins an expected SHA-256
    (from the Hub file metadata or the caller) and the bytes are verified
    BEFORE the rename — a mismatch refuses and the partial is deleted, never
    promoted. Filenames/repo ids are sanitized, sizes gated, and workers are
    tracked in the task registry (no fire-and-forget asyncio tasks).
    """

    def __init__(self, models_dir: Path | None = None,
                 max_bytes: int = DEFAULT_MAX_DOWNLOAD_BYTES):
        self.models_dir = models_dir or DEFAULT_MODELS_DIR
        self.models_dir.mkdir(parents=True, exist_ok=True)
        if max_bytes <= 0:
            raise HFHubError(f"max_bytes must be positive, got {max_bytes!r}")
        self.max_bytes = max_bytes
        self.tasks: dict[str, HFDownloadTask] = {}
        self._cancel_flags: dict[str, bool] = {}
        self._handles: dict[str, asyncio.Task] = {}

    def list_tasks(self) -> list[HFDownloadTask]:
        return list(self.tasks.values())

    def get_task(self, task_id: str) -> HFDownloadTask | None:
        return self.tasks.get(task_id)

    def cancel_task(self, task_id: str) -> bool:
        if task_id in self.tasks and self.tasks[task_id].status == "downloading":
            self._cancel_flags[task_id] = True
            self.tasks[task_id].status = "cancelled"
            return True
        return False

    async def wait_for(self, task_id: str, timeout: float = 300.0) -> HFDownloadTask:
        """Await a tracked download (W2.16: no fire-and-forget — every worker
        has a handle the caller can join)."""
        handle = self._handles.get(task_id)
        if handle is None:
            task = self.tasks.get(task_id)
            if task is None:
                raise HFHubError(f"unknown download task {task_id!r}")
            return task  # already settled
        await asyncio.wait_for(asyncio.shield(handle), timeout)
        task = self.tasks.get(task_id)
        if task is None:  # pragma: no cover — defensive
            raise HFHubError(f"download task {task_id!r} vanished from registry")
        return task

    async def start_download(self, repo_id: str, filename: str, *,
                             expected_sha256: str,
                             max_bytes: int | None = None) -> HFDownloadTask:
        """Initiates a tracked background download task.

        - `expected_sha256` is REQUIRED (hex digest pin): verified before the
          rename, mismatch refuses.
        - UNKNOWN quant labels refuse: unlabeled weights are never fetched.
        - `max_bytes` caps this download (defaults to the manager cap).
        """
        sanitize_repo_id(repo_id)
        if not expected_sha256 or not isinstance(expected_sha256, str):
            raise HFHubError("start_download requires expected_sha256 (hash pin refused empty)")
        quant = _infer_quant_type(filename)
        if quant == "UNKNOWN":
            raise HFHubError(
                f"refusing download of {filename!r}: quantization UNKNOWN — "
                "unlabeled weights are never fetched on a guessed precision")
        cap = self.max_bytes if max_bytes is None else max_bytes
        if cap <= 0:
            raise HFHubError(f"max_bytes must be positive, got {cap!r}")
        safe_repo = repo_id.replace("/", "__")
        dest_dir = self.models_dir / safe_repo
        dest_dir.mkdir(parents=True, exist_ok=True)
        safe_name = sanitize_filename(filename, dest_dir)
        target_path = dest_dir / safe_name

        task_id = str(uuid.uuid4())[:8]
        task = HFDownloadTask(
            task_id=task_id,
            repo_id=repo_id,
            filename=safe_name,
            status="pending",
            local_path=str(target_path),
            total_bytes=cap,
        )
        self.tasks[task_id] = task
        self._cancel_flags[task_id] = False

        # Tracked worker (W2.16): the handle lives in the registry with a
        # done-callback that records stray exceptions instead of dropping
        # them into "exception never retrieved" warnings.
        handle = asyncio.create_task(
            self._download_worker(task, target_path, expected_sha256.lower(), cap))
        self._handles[task_id] = handle

        def _done(h: asyncio.Task, tid: str = task_id) -> None:
            try:
                exc = h.exception()
            except asyncio.CancelledError:
                exc = None
            if exc is not None:
                t = self.tasks.get(tid)
                if t is not None and t.status not in ("completed", "failed", "cancelled"):
                    t.status = "failed"
                    t.error = f"worker crashed: {exc}"
            self._handles.pop(tid, None)

        handle.add_done_callback(_done)
        return task

    def _cancelled(self, task: HFDownloadTask) -> bool:
        return bool(self._cancel_flags.get(task.task_id, False))

    async def _download_worker(self, task: HFDownloadTask, target_path: Path,
                               expected_sha256: str, cap_bytes: int) -> None:
        url = f"https://huggingface.co/{task.repo_id}/resolve/main/{task.filename}"
        task.status = "downloading"
        hasher = hashlib.sha256()

        try:
            async with httpx.AsyncClient(timeout=httpx.Timeout(60.0, connect=10.0), follow_redirects=True) as client:
                async with client.stream("GET", url) as resp:
                    if resp.status_code >= 400:
                        task.status = "failed"
                        task.error = f"HTTP Error {resp.status_code} while fetching model"
                        return

                    total_len = resp.headers.get("Content-Length")
                    if total_len and total_len.isdigit():
                        task.total_bytes = int(total_len)
                        # Max-bytes gate, early (W2.16): refuse before streaming.
                        if task.total_bytes > cap_bytes:
                            task.status = "failed"
                            task.error = (
                                f"refusing download: declared {task.total_bytes} bytes "
                                f"exceeds cap {cap_bytes}")
                            return

                    part_path = target_path.with_suffix(f"{target_path.suffix}.part")
                    downloaded = 0
                    start_time = time.time()
                    last_time = start_time

                    with open(part_path, "wb") as f:
                        async for chunk in resp.aiter_bytes(chunk_size=1024 * 512):
                            if self._cancelled(task):
                                task.status = "cancelled"
                                part_path.unlink(missing_ok=True)
                                return

                            downloaded += len(chunk)
                            # Max-bytes gate, streaming (W2.16): a lying or
                            # absent Content-Length cannot smuggle oversize.
                            if downloaded > cap_bytes:
                                task.status = "failed"
                                task.error = (
                                    f"refusing download: streamed {downloaded} bytes "
                                    f"exceeds cap {cap_bytes}")
                                part_path.unlink(missing_ok=True)
                                return
                            f.write(chunk)
                            hasher.update(chunk)
                            task.bytes_downloaded = downloaded

                            if task.total_bytes > 0:
                                task.progress_percent = round((downloaded / task.total_bytes) * 100, 1)

                            now = time.time()
                            if now - last_time >= 0.5:
                                elapsed = now - start_time
                                task.speed_mb_s = round((downloaded / (1024 * 1024)) / max(elapsed, 0.001), 2)
                                last_time = now

                    # Cancel-rename TOCTOU guard (W2.16): re-check the flag
                    # AFTER the loop — a cancel that landed on the final chunk
                    # must not be promoted by the rename below.
                    if self._cancelled(task):
                        task.status = "cancelled"
                        part_path.unlink(missing_ok=True)
                        return

                    # Hash pin BEFORE rename (W2.16): mismatch refuses and the
                    # partial is deleted, never promoted to the model store.
                    digest = hasher.hexdigest()
                    if digest != expected_sha256:
                        task.status = "failed"
                        task.error = (
                            "refusing download: SHA-256 mismatch "
                            f"(got {digest[:16]}…, want {expected_sha256[:16]}…)")
                        part_path.unlink(missing_ok=True)
                        return

                    if self._cancelled(task):
                        task.status = "cancelled"
                        part_path.unlink(missing_ok=True)
                        return
                    # Atomic guard: os.replace is atomic on POSIX/Windows —
                    # readers never see a half-written weight file.
                    os.replace(part_path, target_path)
                    task.status = "completed"
                    task.progress_percent = 100.0
                    task.sha256 = digest
                    task.completed_at = time.time()

        except Exception as exc:
            if self._cancelled(task):
                task.status = "cancelled"
            else:
                task.status = "failed"
                task.error = str(exc)
