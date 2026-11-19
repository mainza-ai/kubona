# Performance research & audit — 2026-11-18

> **What this is.** A measured performance audit of the current platform plus a
> 2026 practice review, ending in an ordered `PERF-*` program. Every number here
> was produced on this host on 2026-11-18; nothing is inferred from a vendor
> benchmark unless it is labelled as vendor/research evidence. The host is an
> Apple M3 Max (16 CPUs), macOS, with a local LLM server resident — treat the
> absolute rates as *trend evidence for this machine*, not a release budget.
>
> Companion plan: `docs/active-implementation-todos.md` §Performance. Durable
> wiki summary: [[performance-engineering-2026-11-18]].
> **Memory companion:** `docs/memory-management-research-2026-11-18.md`
> (`MEM-01…MEM-22`) covers agentic memory, Go/Python runtime memory, simulator
> peak RSS, and retention. It shares PERF-01 (profiling), PERF-05 (SQLite
> pool/pragmas), PERF-08 (Aer strategy), PERF-12 (gate), and PERF-02
> (transport) with this plan.

## 1. Verdict

The platform is **fast where it is simple and unmeasured where it is complex**.

- The Go edge is genuinely quick: `/api/health` sustains **24–32k req/s** at
  concurrency 20 with **0.6–0.8 ms** mean latency and a **1.9 ms p99**.
- The first DB-backed list endpoint is **~15× slower per request** than that
  (`/api/sessions`: **1.3–1.7k req/s**, **11–15 ms** mean at c=20) and has a
  **161 ms p99**. The isolated SQL (39–70 µs) and JSON (35 µs) do not explain
  the difference, and the repo has **no pprof endpoint, no Go benchmark, and no
  request-phase tracing** to find it. That is the single biggest performance
  gap: the platform cannot currently answer "where did the millisecond go?"
  for its own hot path.
- Several mechanical wins are unclaimed: **no HTTP compression on the Go
  API** (a 9.8 KB session list compresses to 2.2 KB), **chunked**
  responses for every body over 2 KB, **no ordering indexes** for the session
  list (100× at 100k rows), and **unbounded SQLite connections** with an 8 MB
  per-connection page cache.
- The compute side is correct but conservatively configured: the simulator
  always takes the statevector path (26 qubits: **3.48 s** statevector vs
  **0.19 s** MPS, measured), chemistry recomputes every request (benzene
  **1.2–2.0 s** per call, no cache), and hardware transpilation is pinned at
  `optimization_level=1`.
- The console has real budgets (first load **1,135.6 KiB raw / 336.1 KiB
  gzip**, 12 chunks, largest 276.6 KiB) but runs **102 of 214** TSX files as
  client components and has **React Compiler, Cache Components, and PPR
  disabled**.

The plan in §7 is ordered by measured payoff per risk: instrument first, then
the cheap transport/SQL wins, then the compute strategy, then the architectural
items (SSE fan-out, MCP transport, tool-loop concurrency).

## 2. Method

1. Read the hot paths: `pkg/api` (router, `writeJSON`, SSE), `pkg/ledger`,
   `pkg/session`, `pkg/jobs`, `cmd/kubonad`, the bridge executor/experiments/
   chemistry, the simulator provider, the console build config and budgets.
2. Measured the live local stack: `ab -k` matrices, a Go keep-alive client with
   p50/p90/p99, `curl` headers/sizes, `PRAGMA` reads and `EXPLAIN QUERY PLAN`
   on the host ledger, and isolated Go benchmarks against the real schema.
3. Measured the compute path directly with the pinned venv (`qiskit-aer
   0.18.2`, `pyscf 2.15.0`): VQE wall time, PySCF anchor cost by molecule/basis,
   and Aer method/thread trade-offs.
4. Reviewed 2026 practice: Go 1.27 JSON v2, SQLite WAL/pool/cache guidance,
   Next.js 16 Cache Components/PPR/Turbopack, React Compiler 1.0, Qiskit Aer
   0.18 GPU/MPS, PySCF GPU4PySCF, MCP transport benchmarks, and compression
   middleware.
5. Ran a controlled synthetic experiment where the live stack was too small to
   expose a scaling effect (100k sessions; 10 KB chunked vs Content-Length).

**Baseline commit:** `bbcd2958` (tree clean before this audit).

## 3. Measured baseline

### 3.1 HTTP edge (loopback, keep-alive)

| Endpoint | Concurrency | Throughput | Mean | p50 | p90 | p99 | Body |
|---|---:|---:|---:|---:|---:|---:|---:|
| `/api/health` | 20 | 23,665–32,124 req/s | 0.62–0.85 ms | 0.72 ms | 1.12 ms | 1.88 ms | 161 B |
| `/api/sessions` (bearer) | 20 | 1,333–1,736 req/s | 11.5–15.0 ms | 2.07 ms | 7.21 ms | 160.9 ms | 9,811 B |
| `/api/sessions` (anonymous) | 20 | ~14,400 req/s | 1.39 ms | — | — | — | 221 B (401) |
| `/api/capabilities` | 20 | 26,217–29,138 req/s | 0.69–0.76 ms | — | — | — | 2,034 B |

`ab -k` for the first four rows; the p50/p90/p99 row is a Go client with 20
keep-alive workers. The anonymous row is a fast 401, not the list path — it is
included only to show that authentication is not the cost.

**What this means.** The session list is the only measured endpoint with a
pathological tail (161 ms p99 on a 77-row database). At 77 rows the ordered
query is 0.09 ms and the JSON marshal is ~35 µs; the residual is inside the
handler/middleware path and is currently unattributable. The first slice is
therefore instrumentation, not a guess-fix.

### 3.2 Transport

| Property | Value | Evidence |
|---|---|---|
| Compression (Go API) | none (no gzip/zstd/br; no reverse proxy in compose) | `grep -rn "Content-Encoding\|gzip" pkg/api` → no matches; `platform/docker/compose.prod.yml` has no nginx/caddy/traefik. The Next console uses the framework default (compression on) for its own responses. |
| Framing | `Transfer-Encoding: chunked` for bodies > 2 KB | `curl -D -` on `/api/sessions`; health (161 B) gets `Content-Length` |
| Sessions payload | 9,811 B raw → **2,159 B gzip** (4.54×) | `curl … \| gzip -c \| wc -c` |
| Capabilities payload | 2,034 B raw → **607 B gzip** (3.35×) | same |
| gzip level 1 | 51 µs per 14,244 B; 12.73× ratio | Go `testing.Benchmark`, this host |
| gzip level 6 | 80 µs per 14,244 B; 12.91× ratio | same — level 6 buys 0.18 ratio points for 56% more CPU |
| Chunked vs `Content-Length` (10,217 B) | 30,376 vs **98,665 req/s** at c=20 (3.25×) | synthetic Go server, same host/client |

### 3.3 SQLite (host ledger)

| Setting | Value | Consequence |
|---|---|---|
| File size | 3.61 MB, 924 pages | tiny today; the plan must hold at 10⁵–10⁶ rows |
| `journal_mode` | WAL | correct |
| `synchronous` | NORMAL (app DSN) | correct for WAL |
| `busy_timeout` | 5,000 ms | correct, but no backoff contract |
| `cache_size` | −2000 (~8 MB) **per connection** | unbounded pool × 8 MB is a memory risk |
| `mmap_size` | 0 | no memory-mapped reads |
| `temp_store` | 0 (file) | sorts spill to disk |
| `wal_autocheckpoint` | 1000 | default |
| Connection pool | **no `SetMaxOpenConns` / `SetMaxIdleConns`** | `database/sql` default is unlimited |
| Rows | 77 sessions, 44 jobs, 210 chat messages, 1,470 events | small |

`EXPLAIN QUERY PLAN` on the exact list query:

```
SCAN sessions
USE TEMP B-TREE FOR ORDER BY
```

There is no index on `sessions(updated_at)` or `(tenant_id, updated_at)`;
`ledger_events` has `idx_ledger_session(session_id)` and
`idx_ledger_type(event_type, id)`, which the SSE query uses (with a temp B-tree
for the `OR`).

**Synthetic scaling test (100,000 sessions, exact query):**

| Query | No index | With index | Speed-up |
|---|---:|---:|---:|
| Operator list (`ORDER BY updated_at DESC LIMIT 50`) | 6.23 ms | **0.06 ms** | 104× |
| Tenant list (`WHERE tenant_id=? ORDER BY updated_at DESC LIMIT 50`) | 5.36 ms | **0.09 ms** | 60× |
| Go parallel benchmark, operator list (pool 8) | 1.99 ms (c=4) / 2.41 ms (c=16) | **55 µs / 63 µs** | 36–38× |

Controlled isolation at 77 rows: 39–70 µs per parallel op regardless of pool
width, idle count, or DSN — so the live endpoint's tail is *not* the SQL. The
index is still a required fix for scale (and a 100× win at 10⁵ rows).

### 3.4 Bridge compute

| Workload | Wall time | Notes |
|---|---:|---|
| H2 VQE (default, 2 layers, 16k shots) | 0.25 s | `run_vqe`, pinned venv |
| H2 VQE, 4 layers | 0.05 s | shallow circuit |
| LiH VQE | 1.02 s | active-space path |
| PySCF H2 anchor (repeat) | 11 ms | first call 1.07 s includes import |
| PySCF H2O/STO-3G | 21 ms | |
| PySCF H2O/6-31G* | 41 ms | |
| PySCF N2/STO-3G | 53 ms | |
| PySCF benzene/STO-3G | **2.02 s** | CASCI(6,6) |
| PySCF benzene/6-31G* | **1.22 s** | CASCI(6,6) |

No cache exists for `solve_classical_anchor`; repeated identical requests pay
the full cost. For a bond-distance sweep (the common UI action) that is N× the
same integral/SCF work.

**Aer method/threads (20-qubit, 6-layer circuit, 4,096 shots):**

| Configuration | Time |
|---|---:|
| `AerSimulator()` default (16 CPUs) | 0.072 s |
| `max_parallel_threads=1` | 0.399 s |
| `method="statevector"` | 0.081 s |
| `method="matrix_product_state"` | 0.126 s |

**26-qubit, same shape:**

| Method | Time |
|---|---:|
| statevector | 3.48 s |
| matrix_product_state | **0.191 s** (18.2×) |

So: do **not** pin threads to 1 (5.5× regression); do choose MPS for shallow,
wide circuits. The provider caps local simulation at **24 qubits**
(`local_simulator.py:62`), the bridge at **28** statevector / **30** Aer
(`executor.py:95-96`).

### 3.5 Console

| Metric | Value |
|---|---|
| First load `/` | **1,135.6 KiB raw / 336.1 KiB gzip** |
| Chunks | 12; largest **276.6 KiB** |
| Budget | recorded 1,133.4 / 336.2 + allowance 40 / 24 → ceiling 1,173.4 / 360.2; single chunk ≤ 300 KiB |
| Client components | **102 of 214** TSX files (`'use client'`) |
| Framework | Next **16.4.6**, React **19.3.1**, three **0.187.0** |
| React Compiler | **not enabled** (`next.config.ts` has no `reactCompiler`) |
| Cache Components / PPR | **not enabled** (no `cacheComponents`/`ppr`) |
| Gates | `LCP/INP/heap/request-count` E2E + per-route bundle budgets pass |

### 3.6 Other hot-path facts

- **SSE:** one `GetEventsSinceWithGlobalTypes` query **per connected client
  every 2 s** (`server.go:4615`); each event is serialized **twice** (`events`
  array + `event` object, `server.go:4578-4587`).
- **Agent tool loop:** tool calls are dispatched sequentially in a `for` loop
  (`agent_tools.go:584`), even when independent/read-only. The taint barrier is
  order-sensitive: once an untrusted observation is consumed, later effectful
  calls refuse.
- **JSON:** Go 1.27 is in use. Measured on a 14,244 B session-list payload:
  `encoding/json` marshal **35.3 µs** vs `encoding/json/v2` **35.5 µs** (no
  win); unmarshal **37.4 µs → 27.8 µs** (**26% faster**, same allocations).
  So v2 is a *read-path* option, not a blanket migration.
- **No profiling surface:** no `net/http/pprof`, no `expvar`, no Go benchmarks
  (`grep -rln "func Benchmark" platform/core` → none). Rust has `qir-bench`.
- **MCP:** six servers / 38 tools; a Streamable HTTP gateway exists
  (`kubona/run.py:94-97`, `mcp/http_gateway.py`) but no compose service runs it;
  servers are launched over stdio per process.
- **ETags:** used only for `/api/workbench/view` and `/api/workbench`; the rest
  of the read surface recomputes and resends.

## 4. Findings and gaps

### F1 — No way to attribute latency (P0)

There is no pprof, no benchmark suite, and no request-phase timing. The
`/api/sessions` tail (161 ms p99) cannot be attributed from the outside, and a
fix cannot be proven. This blocks every other perf claim.

### F2 — Large JSON responses are chunked and uncompressed (P0)

`writeJSON` marshals fully and writes without `Content-Length`; >2 KB bodies
become chunked. The measured 3.25× throughput gap between chunked and
content-length bodies, and the 4.5× compression ratio on the session list, are
both unclaimed. The Go API has no compression middleware and no proxy to add it (the Next console compresses its own responses by default).

### F3 — Session list has no ordering index (P0)

Every list request scans `sessions` and builds a temp B-tree. At 100k rows this
is 6.2 ms per query (104× slower than indexed). Two indexes and a schema
migration close it.

### F4 — SQLite pool and page-cache policy are unbounded (P1)

No `SetMaxOpenConns`; `cache_size` is per connection (~8 MB), `mmap_size=0`,
`temp_store=0`. The DSN reapplies `journal_mode=WAL` on every new connection.
The controlled 77-row test did not show a pool penalty for reads, so this is a
**risk/memory** fix, not the explanation for the current tail — but it must be
bounded before concurrency grows.

### F5 — Compute strategy is fixed, not adaptive (P1)

The simulator never selects MPS despite an 18× win at 26 qubits; the 24-qubit
cap excludes shallow circuits MPS could run. Chemistry recomputes identical
anchors; hardware transpilation is pinned at level 1. These are correctness-
neutral, measurable wins.

### F6 — SSE fan-out is poll-per-client (P1)

N clients produce N queries every 2 s and each event is sent twice in the
payload. At small N this is invisible; at dashboard scale it is linear DB load
and ~2× bytes. A per-session hub (one query, fan-out to subscribers) also makes
`Last-Event-ID` replay cheaper.

### F7 — Console optimization features are off (P1)

React Compiler (stable 1.0), Cache Components/PPR, and Turbopack production
builds are available on the pinned Next 16.4.6 but not enabled. 48% of TSX files
are client components. The existing budgets prove the current size is
*allowed*, not that it is *minimal*.

### F8 — MCP HTTP transport is implemented but not deployed (P2)

The shared-session Streamable HTTP gateway exists in code but no compose
service runs it; the catalog is unserved. Research (Stacklok, 2026) measures
~293–300 req/s with Streamable HTTP shared sessions vs **0.64 req/s** for stdio
at 20 concurrent connections. This matters only for multi-client agent use, but
it is a 460× class difference when it does.

### F9 — Agent tool calls are serial (P2)

Independent read-only tools run one after another. Parallel dispatch is
possible but must preserve the taint barrier (untrusted output gates later
effectful calls), the per-turn submit bound, and deterministic observation IDs.

### F10 — Static catalogs are recomputed and resent (P2)

`/api/capabilities` (2 KB), `/api/config`, rate cards, and route catalogs change
rarely. ETag/`Cache-Control` would turn repeat reads into 304s; only two
workbench routes do this today.

### F11 — No performance gate in CI (P1)

There is no benchmark artifact, no latency budget, and no regression check.
Budgets exist for bundle size and E2E LCP/INP, but not for the API or the
bridge. A perf claim can regress silently.

### F12 — Go JSON v2 is not a write-path win (anti-finding)

Measured: marshal parity, unmarshal 26% faster. Do not migrate writes for
speed; consider v2 only for large decode paths, behind a benchmark.

## 5. 2026 practice notes (what is worth adopting)

| Area | 2026 guidance | Kubona status |
|---|---|---|
| Go 1.27 | `encoding/json/v2` faster decode, parity encode; new `jsontext` for streaming | v1 in use; v2 measured; adopt decode-only where it pays |
| HTTP compression | gzip/zstd middleware at level 1–3; `Vary: Accept-Encoding`; pooled writers; don't compress SSE | absent |
| SQLite | WAL + `synchronous=NORMAL`; bound the pool; `cache_size` is per connection; consider `mmap_size` and `temp_store=MEMORY` after measuring | WAL/NORMAL good; pool/cache untuned |
| Indexes | Cover the actual `ORDER BY`/filter, not just the FK | missing on sessions ordering |
| Next.js 16 | Turbopack stable; Cache Components/PPR for static shells; React Compiler 1.0 for memoization | none enabled |
| React Compiler | Stable; incremental adoption; removes manual memo churn | off |
| Qiskit Aer 0.18 | Method choice (statevector/MPS/density matrix) dominates; GPU/multi-node for large states; don't pin single-thread | default statevector, default threads |
| PySCF / GPU4PySCF | GPU FFTDF ~25× on H100 for large systems; irrelevant for STO-3G small molecules; cache integrals/SCF | CPU, no cache |
| Qiskit transpile | Level 2/3 improve depth/2q count at compile cost; level 1 is the cheap default | pinned level 1 |
| MCP | Streamable HTTP + shared sessions ≫ stdio under concurrency; stateless since 2026-07-28 | HTTP code exists, not deployed |
| Compression ratios | gzip L1 is the knee; brotli/zstd win ratio but cost CPU and client support | not measured in-product |

Sources are listed in §9.

## 6. What not to do

- **Don't pin Aer threads to 1.** Measured 5.5× slower on 16 CPUs.
- **Don't raise `cache_size` without bounding the pool.** It is per connection.
- **Don't enable PPR/Cache Components blindly.** Most of the console is
  interactive; caching a session-scoped shell would be a correctness bug.
- **Don't parallelize every tool call.** The taint barrier and per-turn submit
  bound are security invariants, not scheduling details.
- **Don't migrate to JSON v2 for marshal speed.** There is no measured win.
- **Don't add zstd/brotli first.** gzip L1 already removes 78% of the bytes at
  51 µs per 14 KB; negotiate a second codec only after a client-support check.
- **Don't treat today's absolute rates as a budget.** The host has a resident
  LLM server; use the ratios and the plan's acceptance tests.

## 7. PERF program (ordered)

Each slice lands only with the named executable check. "Baseline" means the
number in §3 on this host; a slice must beat it or prove no regression. The
memory program (`MEM-*`) is the companion; instrument once (PERF-01), bound the
pool once (PERF-05), choose the simulator once (PERF-08), and gate both
(PERF-12).

### P0 — instrument and remove the mechanical costs

| ID | Slice | Change | Acceptance |
|---|---|---|---|
| PERF-01 | Profiling + benchmarks | Operator-gated `pprof` (bearer-only, off by default) and Go benchmarks for `ListSessions`, `GetEventsSinceWithGlobalTypes`, `AppendEvent`, `checkAuth`, plus `scripts/perf-smoke.sh` | `go test -bench` artifacts recorded; `/debug/pprof` refuses without bearer; smoke script prints the §3 table |
| PERF-02 | `Content-Length` for JSON | Set `Content-Length` in `writeJSON` (and `WriteAPIError`), keep chunked only for streams | Header present on `/api/sessions`; synthetic 10 KB endpoint retains the 3.25× gap closure; no SSE change |
| PERF-03 | gzip/zstd negotiation | Middleware: gzip L1 for JSON >1 KiB when `Accept-Encoding` allows; `Vary`; pooled writers; skip SSE/already-compressed | `/api/sessions` ≤2.5 KB on the wire; p99 ≤ baseline; unit tests for negotiation and `HEAD` |
| PERF-04 | Session indexes | `idx_sessions_updated(updated_at DESC)` and `idx_sessions_tenant_updated(tenant_id, updated_at DESC)`; schema 16 + pre-migration snapshot | `EXPLAIN QUERY PLAN` uses the index; 100k synthetic ≤0.2 ms; migration test + backup snapshot |
| PERF-05 | SQLite policy | `SetMaxOpenConns`/`SetMaxIdleConns` (bounded), PRAGMAs once at open, `temp_store=MEMORY`, measured `mmap_size`; separate read-only pool for GETs if it helps | Pool bounded in a test; 77-row and 100k benchmarks no worse; p99 stable at c=40 |
| PERF-06 | Catalog caching | ETag + `Cache-Control` for capabilities/config/rate cards/routes | Second request returns 304 with 0-byte body; tests for ETag stability |

### P1 — measured compute and console wins

| ID | Slice | Change | Acceptance |
|---|---|---|---|
| PERF-07 | SSE fan-out | Per-session hub: one query per tick, fan-out to subscribers; remove the duplicate `events`/`event` payload | N clients → one query/tick (test); payload halves; `Last-Event-ID` replay tests pass |
| PERF-08 | Aer strategy | Choose `matrix_product_state` for shallow wide circuits, statevector otherwise; configurable thread cap; raise the local cap only with correctness pins | 26-qubit benchmark ≥10× faster; statevector results unchanged on the golden set; cap documented |
| PERF-09 | Chemistry cache | LRU on `(molecule, r, basis)` and generic `(atom,basis,charge,spin,ncas,nelecas)`, bounded by bytes; hit/miss metric | Repeat benzene ≤10 ms (from 1.2–2.0 s); cache eviction test; no stale result across parameters |
| PERF-10 | Transpile level | Configurable `optimization_level` (default 2 for hardware, 1 for sim), record depth/2q before/after | Measured depth/2q reduction at bounded compile time; result schema carries the level |
| PERF-11 | Console compiler/PPR | Enable React Compiler; enable Cache Components/PPR only for static shells; shrink client components | `next build` + bundle budgets + LCP/INP gates pass; no session-scoped caching; bundle ≤ baseline |
| PERF-12 | Perf gate | `scripts/perf-gate.sh` + CI job: benchmark deltas and a latency budget on a disposable stack | CI fails on >20% regression in the named benchmarks; artifact uploaded |

### P2 — architectural

| ID | Slice | Change | Acceptance |
|---|---|---|---|
| PERF-13 | MCP HTTP in compose | Run `kubona.run` Streamable HTTP behind the gateway with shared sessions; keep stdio for local | Concurrency benchmark (stdio vs HTTP) committed; p95 recorded; auth/tenancy tests |
| PERF-14 | Tool-loop concurrency | Parallel dispatch only for independent read-only calls; preserve taint, submit bound, observation order | Timeline test shows overlap; taint refusal and submit-bound tests unchanged |
| PERF-15 | JSON v2 decode | Adopt `encoding/json/v2` on the largest decode paths after a benchmark | ≥20% decode win on the chosen path, parity allocations, no behavior change |

## 8. Baseline commands (reproducible)

```bash
# edge
ab -q -k -n 2000 -c 20 -H "Authorization: Bearer $KUBONA_API_TOKEN" \
  http://127.0.0.1:8080/api/sessions
ab -q -k -n 5000 -c 20 http://127.0.0.1:8080/api/health

# transport
curl -sS -D - -o /dev/null -H "Authorization: Bearer $KUBONA_API_TOKEN" \
  http://127.0.0.1:8080/api/sessions        # Transfer-Encoding: chunked
curl -sS -H "Authorization: Bearer $KUBONA_API_TOKEN" \
  http://127.0.0.1:8080/api/sessions | gzip -c | wc -c   # 2159

# sqlite plan (host ledger, read-only)
sqlite3 'file:'"$HOME"'/.kubona/kubona.db?mode=ro' \
  'EXPLAIN QUERY PLAN SELECT * FROM sessions ORDER BY updated_at DESC LIMIT 50'

# bundle
node scripts/console-bundle-budgets.mjs

# compute (pinned venv)
cd platform/bridge && ../.venv/bin/python -c \
  'import experiments,time;t=time.perf_counter();experiments.run_vqe("LiH");print(time.perf_counter()-t)'
```

## 9. Sources

**First-party (measured here):** `platform/core/pkg/api/server.go`,
`pkg/ledger/ledger.go`, `pkg/session/session.go`, `pkg/api/agent_tools.go`,
`pkg/api/auth_session.go`, `platform/bridge/{experiments,chemistry,executor}.py`,
`platform/kubona/providers/{qiskit_provider,local_simulator}.py`,
`platform/console/next.config.ts`, `platform/contracts/console-bundle-budgets.json`,
`docs/repo-counts.md`.

**2026 practice:** Go 1.27 release notes (`encoding/json/v2`, `jsontext`);
SQLite WAL/PRAGMA guidance (`sqlite.org/wal.html`, production failure-mode
write-ups); Next.js 16 release notes (Turbopack, Cache Components/PPR); React
Compiler 1.0 release; Qiskit Aer 0.18 GPU/multi-node and method docs; Qiskit
transpiler optimization-level guide; PySCF GPU4PySCF / multigrid FFTDF results;
Stacklok MCP transport benchmark (2026) and the 2026-07-28 MCP stateless
revision; CAFxX/httpcompression as a reference implementation.

## 10. Caveats

- All absolute numbers are **this host, this commit, this run**; the local LLM
  server and thermal state add variance. Ratios and `EXPLAIN` plans are the
  durable evidence.
- The 100k-row SQLite benchmark is **synthetic** (the host ledger has 77
  sessions); it is labelled as scaling evidence, not production data.
- The 26-qubit Aer comparison is a **circuit-shape-specific** result (shallow,
  low entanglement); MPS can lose on highly entangled circuits, which is why
  the slice requires a correctness pin and a heuristic, not a blanket switch.
- PySCF timings are single-point CPU runs; GPU4PySCF is out of scope for the
  small-molecule anchors this platform currently serves.
- No paid-QPU or vendor-billed performance measurement is included; paid
  evidence remains operator-owned.
