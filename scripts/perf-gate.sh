#!/usr/bin/env bash
# Performance/memory regression gate. Fails closed on a measured regression
# in the named benchmarks; this is trend evidence for the host, not a
# cross-machine budget.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "== Go writeJSON benchmark"
OUT="$(cd platform/core && go test ./pkg/api -run '^$' -bench 'BenchmarkWriteJSON/gzip' -benchmem -benchtime 200x 2>&1)"
echo "$OUT" | grep -E 'BenchmarkWriteJSON' || { echo "FAIL: BenchmarkWriteJSON/gzip produced no output" >&2; echo "$OUT" >&2; exit 1; }
NS="$(echo "$OUT" | awk '/BenchmarkWriteJSON\/gzip/{print $3; exit}')"
BYTES="$(echo "$OUT" | awk '/BenchmarkWriteJSON\/gzip/{print $5; exit}')"
if [[ -z "$NS" || -z "$BYTES" ]] || ! awk -v ns="$NS" -v b="$BYTES" 'BEGIN { exit !(ns+0 > 0 && b+0 > 0) }'; then
  echo "FAIL: could not parse positive benchmark metrics (NS='${NS}', BYTES='${BYTES}')" >&2
  exit 1
fi
if awk -v ns="$NS" 'BEGIN { exit !(ns+0 > 200000) }'; then
  echo "FAIL: gzip writeJSON ${NS} ns/op > 200000" >&2
  exit 1
fi
if awk -v b="$BYTES" 'BEGIN { exit !(b+0 > 100000) }'; then
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
