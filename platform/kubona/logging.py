"""Structured logging (JSON-lines) for operations (G11 — observability).

`setup_logging()` configures the `kubona` logger to emit parseable JSON-lines to
stderr (and, with `log_dir`, an ops file under the state root). Used by the
`kubona serve`/`serve-gateway` entrypoints so a production control plane has a
machine-parseable operational log (startup, gateway calls, job transitions,
errors) to export/alert on, complementing the immutable audit ledger.

Example line (JSON):
{"ts": "2026-08-17T..Z", "level": "INFO", "logger": "kubona.http", "msg": "rpc",
 "server": "execution", "tool": "submit_job", "status": 200, "duration_ms": 12}
"""

from __future__ import annotations

import json
import logging
import sys
import time
from typing import Any

from kubona.config import state_path

_LOGGER_NAME = "kubona"

# LogRecord reserved fields — everything else on the record is caller `extra`
_RESERVED = {
    "name", "msg", "args", "levelname", "levelno", "pathname", "filename",
    "module", "exc_info", "exc_text", "stack_info", "lineno", "funcName",
    "created", "msecs", "relativeCreated", "thread", "threadName", "processName",
    "process", "taskName", "message", "asctime",
}


def _json_handler(stream=None):
    handler = logging.StreamHandler()  # stream resolved lazily at emit time

    def _emit(record: logging.LogRecord) -> None:
        entry = {
            "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(record.created)),
            "level": record.levelname,
            "logger": record.name,
            "msg": record.getMessage(),
        }
        for k, v in record.__dict__.items():
            if k not in _RESERVED and not k.startswith("__"):
                entry[k] = v
        out = stream or sys.stderr  # lazy: survive stream redirection/None at emit time
        try:
            out.write(json.dumps(entry, sort_keys=True) + "\n")
            out.flush()
        except Exception:  # logging must NEVER break the caller path (like the audit sink)
            return

    handler.emit = _emit
    return handler


def setup_logging(*, level: int = logging.INFO, log_dir: str | None = None) -> logging.Logger:
    """Configure the kubona logger with a JSON-lines handler (idempotent)."""
    logger = logging.getLogger(_LOGGER_NAME)
    logger.setLevel(level)
    logger.propagate = False  # JSON-lines only; never double-emit the root default format
    if not logger.handlers:
        logger.addHandler(_json_handler())
    if log_dir:
        path = state_path("logs/kubona.log")
        logger.addHandler(_json_handler(open(path, "a")))
    return logger


def get_logger(name: str) -> logging.Logger:
    """A child logger (kubona.<name>) that records structured `extra` on calls."""
    return logging.getLogger(f"{_LOGGER_NAME}.{name}")


def log_call(logger: logging.Logger, *, server: str, tool: str, status: int,
             duration_ms: float, **extra: Any) -> None:
    """Emit a structured JSON request log (used by the HTTP gateway)."""
    logger.info("rpc", extra={"server": server, "tool": tool, "status": status,
                              "duration_ms": round(duration_ms, 2), **extra})
