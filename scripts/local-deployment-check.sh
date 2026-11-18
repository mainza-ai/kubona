#!/usr/bin/env bash
# Local private deployment acceptance check: loopback-only published ports,
# health/readiness, console reachability, and the effective access mode.
# Exit non-zero on any failure.
#
# Usage:
#   scripts/local-deployment-check.sh [--expect-single-lab]
#
# --expect-single-lab fails when the daemon reports multi_lab. The compose
# stack declares KUBONA_SINGLE_LAB=1, but the daemon only reports local_private
# while the sessions table holds at most one tenant. E2E labs or a restored
# multi-lab ledger flip it to multi_lab without any config change, so an
# operator who wants the declared posture must assert it here.
set -euo pipefail

expect_single_lab=0
for arg in "$@"; do
  case "$arg" in
    --expect-single-lab) expect_single_lab=1 ;;
    *)
      echo "usage: $0 [--expect-single-lab]" >&2
      exit 2
      ;;
  esac
done

fail=0
check_port() {
  local port="$1" out
  out=""
  # Prefer ss on Linux (no process-name permission needed); fall back to
  # lsof on macOS. The CI runner's lsof can be present but return nothing for
  # docker-proxy, so try both rather than choosing one at startup.
  if command -v ss >/dev/null 2>&1; then
    out="$(ss -ltn 2>/dev/null | grep -E "[:.]${port}[[:space:]]" || true)"
  fi
  if [ -z "$out" ] && command -v lsof >/dev/null 2>&1; then
    out="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  fi
  if [ -z "$out" ] && command -v netstat >/dev/null 2>&1; then
    out="$(netstat -ltn 2>/dev/null | grep -E "[:.]${port}[[:space:]]" || true)"
  fi
  if [ -z "$out" ]; then
    echo "FAIL: nothing is listening on :$port" >&2
    fail=1
    return
  fi
  if printf '%s\n' "$out" | grep -Eq '(^|[[:space:]])(\*|0\.0\.0\.0|\[::\]|:::?):?'"$port"'([[:space:]]|$)'; then
    echo "FAIL: :$port is published on a public interface:" >&2
    printf '%s\n' "$out" >&2
    fail=1
    return
  fi
  if ! printf '%s\n' "$out" | grep -Eq '(^|[[:space:]])(127\.0\.0\.1|\[::1\]|::1|localhost):'"$port"'([[:space:]]|$)'; then
    echo "FAIL: :$port is not clearly loopback-only:" >&2
    printf '%s\n' "$out" >&2
    fail=1
    return
  fi
  echo "OK: :$port is loopback-only"
}

check_port 8080
check_port 3000

if curl -fsS --max-time 5 http://127.0.0.1:8080/api/health >/dev/null; then
  echo "OK: kubonad health"
else
  echo "FAIL: kubonad health" >&2
  fail=1
fi
if curl -fsS --max-time 5 http://127.0.0.1:8080/api/ready >/dev/null; then
  echo "OK: kubonad ready"
else
  echo "FAIL: kubonad ready (disk/bridge/state not ready)" >&2
  fail=1
fi

# Access mode: the declared single-lab posture is only real when the daemon
# reports it. /api/access is a public read, so this needs no token.
if access_json="$(curl -fsS --max-time 5 http://127.0.0.1:8080/api/access 2>/dev/null)"; then
  access_mode="$(printf '%s' "$access_json" | sed -n 's/.*"mode":"\([^"]*\)".*/\1/p')"
  access_single="$(printf '%s' "$access_json" | sed -n 's/.*"single_lab":\([a-z]*\).*/\1/p')"
  echo "OK: access mode=${access_mode:-unknown} single_lab=${access_single:-unknown}"
  if [ "$expect_single_lab" = 1 ] && [ "$access_single" != "true" ]; then
    echo "FAIL: --expect-single-lab was requested but the daemon reports" >&2
    echo "      mode=${access_mode:-unknown} single_lab=${access_single:-unknown}." >&2
    echo "      singleTenantStore() counts distinct session tenants: E2E labs or a" >&2
    echo "      restored multi-lab ledger keep the stack in multi_lab. Reset to a" >&2
    echo "      fresh volume (docs/local-deployment.md 'Reset a compose ledger')," >&2
    echo "      never by deleting tenants from a live ledger." >&2
    fail=1
  fi
else
  echo "FAIL: /api/access is unreachable" >&2
  fail=1
fi

if curl -fsS --max-time 10 http://127.0.0.1:3000/ >/dev/null; then
  echo "OK: console"
else
  echo "FAIL: console" >&2
  fail=1
fi

exit "$fail"
