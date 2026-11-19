#!/usr/bin/env bash
# Performance/memory regression gate. Fails closed on a measured regression
# in the named benchmarks; this is trend evidence for the host, not a
# cross-machine budget.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "== Go writeJSON benchmark"
OUT="$(cd platform/core && go test ./pkg/api -run XXX -bench 'BenchmarkWriteJSON/gzip' -benchmem -benchtime 200x 2>&1)"
echo "$OUT" | grep -E 'BenchmarkWriteJSON'
NS="$(echo "$OUT" | awk '/gzip/{print $3; exit}')"
BYTES="$(echo "$OUT" | awk '/gzip/{print $5; exit}')"
if [[ -n "$NS" && "$NS" -gt 200000 ]]; then
  echo "FAIL: gzip writeJSON ${NS} ns/op > 200000" >&2
  exit 1
fi
if [[ -n "$BYTES" && "$BYTES" -gt 100000 ]]; then
  echo "FAIL: gzip writeJSON ${BYTES} B/op > 100000" >&2
  exit 1
fi

echo "== memory budgets and lifecycle"
(cd platform/core && go test ./pkg/memory -run 'TestGlobalBudget|TestDedupe|TestLoadRespects|TestSupersede' -count=1)

echo "== simulator policy"
(cd platform && .venv/bin/python -m pytest bridge/test_simulator_policy.py -q >/dev/null)

echo "== console bundle budgets"
node scripts/console-bundle-budgets.mjs | tail -2

echo "perf-gate: OK (writeJSON gzip ${NS} ns/op, ${BYTES} B/op)"
