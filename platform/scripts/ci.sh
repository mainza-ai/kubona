#!/usr/bin/env bash
# Kubona local CI — mirrors .github/workflows/ci.yml so the whole pipeline
# (python tests, Rust core, canary + correctness release-gate) is reproducible
# locally on any machine. Exit non-zero on any gate failure.
set -euo pipefail
cd "$(dirname "$0")/.."

export KUBONA_STATE_DIR="$(mktemp -d)"
# SDKs (pytket) write a config under XDG_CONFIG_HOME; point it at a writable dir
# so the pipeline is robust on hosts where ~/.config is root-owned.
export XDG_CONFIG_HOME="$(mktemp -d)"

echo "== 0. compile check =="
PY=.venv/bin/python
[ -x "$PY" ] || PY=python3
$PY -m compileall -q kubona

echo "== 1. build Rust qir-core (production compiler core) =="
pushd crates/qir-core >/dev/null
cargo build --release >/dev/null
cargo test --lib -q
popd >/dev/null
export KUBONA_QIR_CORE="$(pwd)/crates/qir-core/target/release/qir-core"

echo "== 1b. Go orchestrator (build + full suite) =="
( cd core && go build ./... && go test ./... )

echo "== 1c. Console (tests + lint + typecheck) =="
( cd console && npm test -- --run && npm run lint && npx tsc --noEmit )

echo "== 1d. Bridge contract + quantum RPC suites =="
$PY -m pytest -q bridge/test_bridge.py bridge/test_quantum_rpc.py

echo "== 1e. Wiki structural lint =="
( cd .. && python3 scripts/wiki-lint.py --strict )

echo "== 1f. Secret + local-path scan =="
bash ../scripts/secret-scan.sh

echo "== 2. full Python suite (Rust core wired) =="
# -rs: report skip reasons. A count of "N passed, M skipped" without the reasons is
# how a suite stops covering things unnoticed -- the same legibility rule release_gate()
# applies to providers applies here to tests.
$PY -m pytest -q -rs

echo "== 3. hardware canary (GA providers) =="
$PY -m kubona.canary

echo "== 4. correctness-regression release gate =="
$PY -m kubona.canary --gate

echo "== 5. doctor (state-root + fail-closed config) =="
$PY -m kubona.run doctor
KUBONA_ENV=production KUBONA_STATE_DIR="$(mktemp -d)" $PY -c \
  'import kubona.config as c; assert c.console_token() is None; assert c.is_production()'

echo "== ALL CI GATES GREEN =="
