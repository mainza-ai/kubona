#!/usr/bin/env bash
# Flight-recorder load-test proof (LOCAL handler measurement, not prod).
#
# Measures the real Go read path GET /api/campaigns/:id/flight-recorder on a
# throwaway local kubonad (fresh temp state dir, test bearer token) under two
# concurrent-curl phases:
#   soak  : N=20 clients x M=25 sequential fetches = 500 samples
#   burst : N=50 clients x M=10 sequential fetches = 500 samples
# Reports p50/p99/max latency, non-200 error count, and a verdict line
# comparing p99 against the console's documented 60s gate window
# (FLIGHT_RECORDER_GATE_BUDGET_MS = 60000, platform/console) plus an
# informational reference against a sane interactive budget (1000 ms).
# Pass/fail is on zero errors + p99 under the 60s gate window only.
# The 1000 ms figure is a stated reference, not an SLO.
#
# Seeding exercises the REAL read path (no stubs): session creation
# synthesizes + persists campaign state (SynthesizeCampaignStateWithOpts +
# SaveCampaignStateWithTrigger), and one tribunal_commit event is sealed via
# /api/ledger/append, so the handler's ListClaimRecords + GetRecentEvents +
# GetCampaignState branches all hit real SQLite state.
#
# Usage: scripts/load-flight-recorder.sh
# Always tears down the daemon + temp state dir, even on failure (trap).

set -u
set -o pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CORE_DIR="$REPO_ROOT/platform/core"
PORT=18081
BASE="http://127.0.0.1:$PORT"
TOKEN="test-load-token"
GATE_MS=60000
INTERACTIVE_REF_MS=1000
BIN="/tmp/kubonad-load-test-$$"
STATE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/kubona-load.XXXXXX")"
DB_PATH="$STATE_DIR/kubona.db"
DAEMON_PID=""
FAIL=0

log() { printf '%s\n' "$*"; }

fail() {
  log "FATAL: $*"
  if [ -f "$STATE_DIR/daemon.log" ]; then
    log "--- daemon.log tail ---"
    tail -30 "$STATE_DIR/daemon.log" || true
  fi
  FAIL=1
  exit 1
}

cleanup() {
  if [ -n "$DAEMON_PID" ] && kill -0 "$DAEMON_PID" 2>/dev/null; then
    kill "$DAEMON_PID" 2>/dev/null || true
    wait "$DAEMON_PID" 2>/dev/null || true
  fi
  rm -f "$BIN"
  rm -rf "$STATE_DIR"
}
trap cleanup EXIT INT TERM

# 1. Build kubonad to /tmp from platform/core.
log "BUILD: go build ./... + kubonad binary (from $CORE_DIR)"
(cd "$CORE_DIR" && go build ./...) || fail "go build ./... failed"
(cd "$CORE_DIR" && go build -o "$BIN" ./cmd/kubonad) || fail "kubonad build failed"
[ -x "$BIN" ] || fail "binary missing at $BIN"

# 2. Start daemon on 127.0.0.1:18081 with isolated temp state.
log "START: kubonad on 127.0.0.1:$PORT (KUBONA_STATE_DIR=$STATE_DIR)"
KUBONA_STATE_DIR="$STATE_DIR" \
KUBONA_API_TOKEN="$TOKEN" \
KUBONA_ENV=test \
  "$BIN" --bind 127.0.0.1 --port "$PORT" --db "$DB_PATH" \
  >"$STATE_DIR/daemon.log" 2>&1 &
DAEMON_PID=$!

log "WAIT: /api/ready (up to 60s)"
READY_OK=0
i=0
while [ "$i" -lt 120 ]; do
  if curl -sf -o /dev/null --max-time 2 "$BASE/api/ready" 2>/dev/null; then
    READY_OK=1
    break
  fi
  if ! kill -0 "$DAEMON_PID" 2>/dev/null; then
    fail "daemon exited before ready"
  fi
  sleep 0.5
  i=$((i + 1))
done
[ "$READY_OK" -eq 1 ] || fail "/api/ready never became ready"
log "READY: daemon is up"

AUTH_H="Authorization: Bearer $TOKEN"

# 3. Seed state: tenant + session + one tribunal event (real read path).
log "SEED: create tenant"
TENANT_RESP="$(curl -s --max-time 10 -w '\n%{http_code}' -X POST "$BASE/api/tenants" \
  -H "$AUTH_H" -H 'Content-Type: application/json' \
  -d '{"id":"load-test-lab","display_name":"Load Test Lab"}')" || fail "tenant request failed"
TENANT_CODE="$(printf '%s' "$TENANT_RESP" | tail -1)"
[ "$TENANT_CODE" = "201" ] || fail "tenant create returned $TENANT_CODE: $TENANT_RESP"
log "SEED: tenant load-test-lab created (201)"

log "SEED: create session (synthesizes + persists real campaign state)"
SESS_RESP="$(curl -s --max-time 30 -X POST "$BASE/api/sessions" \
  -H "$AUTH_H" -H 'Content-Type: application/json' \
  -d '{"title":"load-test flight recorder","tenant_id":"load-test-lab"}')" || fail "session request failed"
SID="$(printf '%s' "$SESS_RESP" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))')" || fail "session decode failed"
[ -n "$SID" ] || fail "empty session id: $SESS_RESP"
log "SEED: session $SID created"

log "SEED: append one tribunal_commit event (real ledger write)"
APP_CODE="$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$BASE/api/ledger/append" \
  -H "$AUTH_H" -H 'Content-Type: application/json' \
  -d "{\"session_id\":\"$SID\",\"event_type\":\"tribunal_commit\",\"payload\":{\"note\":\"load-test seed\"}}")" || fail "ledger append request failed"
[ "$APP_CODE" = "200" ] || fail "ledger append returned $APP_CODE"
log "SEED: tribunal_commit sealed (200)"

FLIGHT_URL="$BASE/api/campaigns/$SID/flight-recorder"

log "WARMUP: single flight-recorder fetch"
WARM_CODE="$(curl -s --max-time 20 -o "$STATE_DIR/warm.json" -w '%{http_code}' -H "$AUTH_H" "$FLIGHT_URL")" || fail "warmup request failed"
[ "$WARM_CODE" = "200" ] || fail "warmup returned $WARM_CODE"
python3 -c 'import json; d=json.load(open("'"$STATE_DIR"'/warm.json")); assert d.get("ok") is True, d; assert "campaign_state" in d, d' \
  || fail "warmup payload missing ok/campaign_state (stub check)"
log "WARMUP: 200 with ok=true + campaign_state present (real read path confirmed)"

# 4. Load phases. Each worker does M sequential curls, writing
# "<seconds> <http_code>" per line to its own file (no interleaving).
run_phase() {
  name="$1"; n="$2"; m="$3"
  out="$STATE_DIR/results-$name.txt"
  rm -f "$out"
  log "LOAD: phase=$name clients=$n fetches_each=$m expect=$((n * m)) samples"
  # NOTE: wait only on worker PIDs below — a bare `wait` would also wait
  # for the daemon background job and hang until the daemon exits.
  pids=""
  k=1
  while [ "$k" -le "$n" ]; do
    part="$STATE_DIR/part-$name-$k.txt"
    rm -f "$part"
    (
      j=1
      while [ "$j" -le "$m" ]; do
        curl -s -o /dev/null -w '%{time_total} %{http_code}\n' --max-time 70 \
          -H "$AUTH_H" "$FLIGHT_URL" >>"$part" 2>/dev/null || true
        j=$((j + 1))
      done
    ) &
    pids="$pids $!"
    k=$((k + 1))
  done
  for p in $pids; do wait "$p"; done
  cat "$STATE_DIR"/part-"$name"-*.txt >"$out"
  rm -f "$STATE_DIR"/part-"$name"-*.txt
}

stats() {
  python3 - "$1" <<'EOF'
import sys
lats, errs, n = [], 0, 0
for line in open(sys.argv[1]):
    p = line.split()
    if len(p) != 2:
        errs += 1; n += 1; continue
    try:
        sec, code = float(p[0]), p[1]
    except ValueError:
        errs += 1; n += 1; continue
    n += 1
    if code != "200":
        errs += 1
        continue
    lats.append(sec * 1000.0)
lats.sort()
def pct(q):
    import math
    if not lats: return float("nan")
    return lats[min(len(lats) - 1, max(0, math.ceil(q * len(lats)) - 1))]
print("n=%d p50=%.1f p99=%.1f max=%.1f errors=%d" % (
    n, pct(0.50), pct(0.99), (max(lats) if lats else float("nan")), errs))
EOF
}

report_phase() {
  name="$1"; n="$2"; m="$3"
  line="$(stats "$STATE_DIR/results-$name.txt")" || fail "stats failed for $name"
  samples=""; p50=""; p99=""; max=""; errors=""
  for kv in $line; do
    case "$kv" in
      n=*) samples="${kv#n=}" ;;
      p50=*) p50="${kv#p50=}" ;;
      p99=*) p99="${kv#p99=}" ;;
      max=*) max="${kv#max=}" ;;
      errors=*) errors="${kv#errors=}" ;;
    esac
  done
  [ -n "$samples$p50$p99$max$errors" ] && [ -n "$samples" ] && [ -n "$p50" ] && [ -n "$p99" ] && [ -n "$max" ] && [ -n "$errors" ] \
    || fail "unparseable stats for $name: $line"
  log "RESULT $name: ${n}clients x ${m}fetches samples=$samples p50=${p50}ms p99=${p99}ms max=${max}ms errors=${errors}"
  verdict="PASS"
  reason="zero errors + p99 < ${GATE_MS}ms gate"
  if [ "$errors" != "0" ]; then verdict="FAIL"; reason="non-200 errors present ($errors)"; fi
  over_gate="$(python3 -c "print('yes' if float('$p99') >= $GATE_MS else 'no')")"
  if [ "$over_gate" = "yes" ]; then verdict="FAIL"; reason="p99 >= ${GATE_MS}ms gate"; fi
  log "VERDICT $name: p99=${p99}ms vs 60s-gate ${GATE_MS}ms => $verdict ($reason; errors=$errors)"
  log "REFERENCE $name: p99=${p99}ms vs interactive ${INTERACTIVE_REF_MS}ms (informational only; pass/fail is on zero errors + p99 < ${GATE_MS}ms gate, not an SLO)"
  if [ "$verdict" = "FAIL" ]; then FAIL=1; fi
}

run_phase soak 20 25
report_phase soak 20 25
run_phase burst 50 10
report_phase burst 50 10

if [ "$FAIL" -eq 0 ]; then
  log "OVERALL: PASS (LOCAL handler measurement only; not a prod claim)"
else
  log "OVERALL: FAIL (LOCAL handler measurement only; not a prod claim)"
  exit 1
fi
