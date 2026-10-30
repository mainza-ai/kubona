#!/usr/bin/env bash
# Local private deployment acceptance check: loopback-only published ports,
# health/readiness, and console reachability. Exit non-zero on any failure.
set -euo pipefail

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
if curl -fsS --max-time 10 http://127.0.0.1:3000/ >/dev/null; then
  echo "OK: console"
else
  echo "FAIL: console" >&2
  fail=1
fi

exit "$fail"
