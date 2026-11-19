---
type: concept
name: Performance Engineering — 2026-11-18
date: 2026-11-18
updated: 2026-11-18
tags: [performance, latency, throughput, sqlite, http, console, aer, pyscf, plan]
sources: [source-gap-analysis-plan]
status: tracked
---

# Performance Engineering — 2026-11-18

> **What this page is.** The durable summary of a measured performance audit of
> the current platform, with the ordered `PERF-*` plan. The full report,
> commands, and per-slice acceptance tests are in
> [`docs/performance-research-2026-11-18.md`](https://github.com/mainza-ai/kubona/blob/develop/docs/performance-research-2026-11-18.md).
> This page is `tracked`: the numbers are measurements, the slices are not yet
> all landed.

## Verdict

Kubona is **fast where the path is simple and unmeasured where it is complex**.
The Go edge sustains 24–32k req/s on `/api/health` with a 1.9 ms p99, while the
first DB-backed list endpoint (`/api/sessions`) sustains 1.3–1.7k req/s with a
**161 ms p99**. The isolated SQL (39–70 µs) and JSON (35 µs) do not explain that
tail, and the repo has **no pprof, no Go benchmark, and no request-phase
timing** — so the platform cannot yet say where its own milliseconds go. That
instrumentation gap is the first slice.

The mechanical wins are unclaimed: no HTTP compression in the shipped stack, no
`Content-Length` on large JSON, no ordering index for the session list (100× at
100k rows), and an unbounded SQLite pool with an ~8 MB per-connection page
cache. The compute path is correct but fixed: statevector always (MPS is 18×
faster at 26 qubits), chemistry recomputed per request (benzene 1.2–2.0 s), and
hardware transpilation pinned at level 1.

## Measured baseline (this host, `bbcd2958`)

| Surface | Measurement |
|---|---|
| `/api/health` c=20 | 24–32k req/s; 0.62–0.85 ms mean; p50 0.72 ms, p99 1.88 ms |
| `/api/sessions` c=20 (bearer) | 1.3–1.7k req/s; 11–15 ms mean; p50 2.07 ms, p90 7.21 ms, **p99 160.9 ms** |
| `/api/sessions` payload | 9,811 B raw → **2,159 B gzip** (4.54×) |
| Framing | `Transfer-Encoding: chunked` for >2 KB bodies; no compression anywhere |
| Chunked vs `Content-Length` (10 KB, synthetic) | 30,376 vs **98,665 req/s** at c=20 (3.25×) |
| gzip L1 vs L6 (14,244 B) | 51 µs / 12.73× vs 80 µs / 12.91× |
| SQLite list query, 100k rows | 6.23 ms → **0.06 ms** with `(updated_at DESC)`; tenant 5.36 → 0.09 ms |
| SQLite host DB | 3.61 MB, WAL, `cache_size=-2000` (~8 MB per connection), `mmap_size=0`, `temp_store=0`, no pool limits |
| VQE | H2 0.25 s; LiH 1.02 s |
| PySCF anchor | H2 11 ms; H2O 21–41 ms; N2 53 ms; benzene **1.22–2.02 s** (no cache) |
| Aer 26 qubits | statevector **3.48 s** vs MPS **0.19 s** (18.2×) |
| Aer 20 qubits | default threads 0.072 s vs 1 thread 0.399 s (5.5×) |
| Console first load | **1,135.6 KiB raw / 336.1 KiB gzip**, 12 chunks, largest 276.6 KiB |
| Console composition | **102 / 214** TSX files are client components; React Compiler and Cache Components/PPR off |

Absolute rates are trend evidence for this machine (a resident local LLM server
adds variance); the ratios, query plans, and method comparisons are the durable
part.

## Findings

1. **No latency attribution.** No `net/http/pprof`, no `expvar`, no Go
   benchmarks. The session-list tail cannot be explained or fixed safely.
2. **Transport costs unclaimed.** Chunked large JSON, no gzip, no
   `Content-Length`, no `Vary`; no reverse proxy in the compose stack to do it
   instead.
3. **Missing ordering indexes.** The list query scans `sessions` and builds a
   temp B-tree. At 100k rows that is 6.2 ms per request.
4. **Unbounded SQLite policy.** No `SetMaxOpenConns`; the page cache is per
   connection; the DSN reapplies `journal_mode=WAL` on every new connection.
5. **Fixed compute strategy.** Statevector always; no chemistry cache; transpile
   level 1 always.
6. **Poll-per-client SSE.** One query per client every 2 s, and each event is
   serialized twice (`events` array + `event` object).
7. **Console optimizations off.** React Compiler 1.0 and PPR/Cache Components
   are available on Next 16.4.6 but not enabled; nearly half the TSX files are
   client components.
8. **MCP HTTP exists but is not deployed.** The Streamable HTTP gateway is in
   `kubona/run.py`, but no compose service runs it; stdio is the live transport.
9. **Serial tool loop.** Independent read-only calls run one at a time; the
   taint barrier makes naive parallelism a security bug.
10. **No performance gate.** Bundle and E2E budgets exist; API/bridge
    regressions are invisible.
11. **JSON v2 is a decode win only.** Marshal parity (35.3 vs 35.5 µs),
    unmarshal 37.4 → 27.8 µs. Do not migrate writes for speed.

## Plan

Full acceptance tests are in the report §7 and in
`docs/active-implementation-todos.md` §Performance.

- **P0:** PERF-01 profiling + benchmarks; PERF-02 `Content-Length`; PERF-03
  gzip L1; PERF-04 session indexes (schema 16, pre-migration snapshot); PERF-05
  bounded SQLite pool/PRAGMAs; PERF-06 ETag/`Cache-Control` catalogs.
- **P1:** PERF-07 SSE hub + payload dedupe; PERF-08 Aer method heuristic;
  PERF-09 chemistry LRU; PERF-10 configurable transpile level; PERF-11 React
  Compiler + static-shell PPR; PERF-12 perf gate in CI.
- **P2:** PERF-13 MCP Streamable HTTP in compose; PERF-14 read-only tool-call
  concurrency with the taint barrier intact; PERF-15 JSON v2 on measured decode
  paths only.

## Anti-findings (do not do)

- Do not pin Aer to one thread (5.5× slower).
- Do not raise `cache_size` without bounding the pool (per-connection memory).
- Do not enable PPR/Cache Components on session-scoped interactive surfaces.
- Do not parallelize effectful tool calls (taint barrier).
- Do not migrate JSON marshal to v2 for speed (no measured win).
- Do not add brotli/zstd before gzip L1 and a client-support check.

## Register

This page describes a **measured prototype on one host**, not a production
capacity plan. The indexes, compression, and pool policy are ordinary
production hygiene; the SSE hub and tool-loop concurrency are design changes.
Re-measure on the deployment host before quoting rps as a capacity figure.
