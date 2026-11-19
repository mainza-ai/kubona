#!/usr/bin/env bash
# Repeatable local performance smoke. Prints real timings and byte counts;
# it is trend evidence for the host, not a release budget.
set -euo pipefail

BASE="${KUBONA_BASE_URL:-http://127.0.0.1:8080}"
TOKEN="${KUBONA_API_TOKEN:-}"
AUTH=()
if [[ -n "$TOKEN" ]]; then AUTH=(-H "Authorization: Bearer $TOKEN"); fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

probe() {
  local path="$1"
  local out="$tmp/body"
  local code time size
  read -r code time size < <(curl -sS -o "$out" -w '%{http_code} %{time_total} %{size_download}' "${AUTH[@]}" "$BASE$path")
  local gz="n/a"
  if [[ -s "$out" ]] && command -v gzip >/dev/null 2>&1; then
    gz="$(gzip -c "$out" | wc -c | tr -d ' ')"
  fi
  printf '%-34s code=%s time=%ss bytes=%s gzip=%s\n' "$path" "$code" "$time" "$size" "$gz"
}

echo "perf-smoke base=$BASE"
probe /api/health
probe /api/ready
probe /api/capabilities
probe /api/sessions
probe /api/metrics

if command -v ab >/dev/null 2>&1; then
  echo
  echo "ab keep-alive c=20 n=1000"
  ab -q -k -n 1000 -c 20 "${AUTH[@]}" "$BASE/api/health" 2>/dev/null | grep -E 'Requests per second|Time per request|Failed requests' || true
  ab -q -k -n 1000 -c 20 "${AUTH[@]}" "$BASE/api/sessions" 2>/dev/null | grep -E 'Requests per second|Time per request|Failed requests' || true
fi
