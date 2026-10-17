#!/usr/bin/env bash
# Context-overhead probe (leanrea I1): measure the live gateway surface
# and fail past budget. Manual/operator runs; CI teeth live in
# platform/tests/test_mcp_surface.py (same measurer, same budget).
set -euo pipefail
cd "$(dirname "$0")/.."
export PYTHONPATH="${PYTHONPATH:-}:platform"
exec platform/.venv/bin/python bench/surface.py "$@"
