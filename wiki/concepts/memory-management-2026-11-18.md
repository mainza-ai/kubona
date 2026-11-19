---
type: concept
name: Memory Management — 2026-11-18
date: 2026-11-18
updated: 2026-11-18
tags: [memory, agentic-memory, context, runtime, sqlite, qiskit, pyscf, plan]
sources: [source-gap-analysis-plan]
status: tracked
---

# Memory Management — 2026-11-18

> **What this page is.** The durable summary of a measured audit of Kubona's
> memory across agentic memory, Go/Python runtime memory, the quantum bridge,
> and storage retention, with the ordered `MEM-*` program. The full report,
> commands, and per-slice acceptance tests are in
> [`docs/memory-management-research-2026-11-18.md`](https://github.com/mainza-ai/kubona/blob/develop/docs/memory-management-research-2026-11-18.md).
> Companion to [[performance-engineering-2026-11-18]]; this page is `tracked`.

## Verdict

Kubona has a **real memory model with production-shaped isolation** — three
banks, tenant + readership scoping through the IFC lattice, retraction
tombstones, bounded per-bank caps — but its **global budget and lifetime
policy are incomplete**, and the compute bridge can be OOM-killed by its own
advertised limit.

- **No global budget:** caps are 500 items / 250k tokens per bank, per tenant,
  per readership. Measured 50,000 memories = **97.3 MiB** live heap and
  **169 ms** startup load. `loadFromDB` loads every tenant; there is no total
  or byte cap.
- **Decay does not run:** `StartDecayDaemon` has no caller, and semantic recall
  ignores `EffectiveScore`. The 1%/hour curve is dead configuration.
- **Bridge limit mismatch:** container **4 GiB**; measured Aer peak is
  **1.1 GiB at 26q**, **2.1 GiB at 27q**, **4,172 MiB at 28q**. The cap is 28
  qubits, the in-flight rail defaults to **8 GiB**, and Aer's
  `max_memory_mb=0` uses *system* memory (128 GiB here), not the cgroup.
- **Memory is invisible:** `/metrics` has no heap/GC/RSS/cgroup or store
  gauges; there is no pprof (PERF-01) and no Python RSS surface.

Small anchors are fine: PySCF peaks at **107–224 MiB** (H2 → benzene/6-31G*).
The risk is the unbounded dimensions (tenants, transcripts, events, context)
and unguarded simulator widths, not today's working set.

## Measured baseline (this host, `5446b2a2`)

| Measurement | Value |
|---|---|
| Hindsight 50k, live heap | **97.3 MiB** (1.9 KiB/item) |
| Hindsight 50k, startup load | **168.9 ms** |
| Hindsight recall over 50k | **34.3 ms**, holds the store write lock |
| Aer peak RSS 24 / 25 / 26 / 27 / 28 qubits | 332 / 588 / 1,100 / 2,124 / **4,172 MiB** |
| `Statevector.from_instruction` 24 / 26 / 27 | 1,093 / 4,165 / 8,261 MiB |
| PySCF H2 / H2O / N2 / benzene | 107 / 113 / 113 / **224 MiB** |
| Fresh kubonad RSS | 27.6 MiB |
| Container limits | bridge 4 GiB / 2 CPU; kubonad 1 GiB / 1 CPU; console 1 GiB / 1 CPU; backup 256 MiB |
| Host ledger | 3.61 MiB; 45 memories + 45 vectors; 1,470 events; 210 messages |
| SQLite `auto_vacuum` / freelist | 0 / 0 |
| Console heap gate | ≤250 MB; Three.js disposal present |
| Dormant PPR 2k / 5k entities | 92 / **415 MiB** |

## Findings

1. **No global budget** — per-tenant caps only; startup loads all tenants.
2. **No memory observability** — no heap/GC/RSS/cgroup/store gauges, no pprof.
3. **Dead decay** — daemon never started; semantic recall ignores the score.
4. **Embeddings under the lock** — synchronous HTTP embedding (5 s timeout)
   blocks all recall/retain for every tenant.
5. **No dedupe/consolidation** — every turn appends; repeated prompts evict
   useful history.
6. **No temporal validity** — contradictions coexist; retraction is manual.
7. **Truncation, not compaction** — 40 messages / 48k chars are dropped with a
   marker; no summary or tool-result offload (2026 practice: 26–54% lower peak
   tokens with compaction).
8. **Unbounded transcript read** — `GetMessages` selects all rows, then
   budgets the result.
9. **No retention/vacuum** — events/messages/jobs/tombstones grow forever;
   `auto_vacuum=0`; the 256 MiB backup sidecar does `VACUUM INTO`.
10. **Bridge caps exceed the container** — 28q/4,172 MiB and an 8 GiB in-flight
    default on a 4 GiB container; Aer is not cgroup-aware.
11. **Two statevector profiles** — Aer ~1× the state, Qiskit `Statevector`
    ~4× (26q: 1,100 vs 4,165 MiB).
12. **No Python allocator/thread hygiene** — no `MALLOC_ARENA_MAX`, BLAS/OMP
    caps, `malloc_trim`, or `RLIMIT_AS`; tmpfs `/tmp` counts as RAM.
13. **Dormant tiered storage** — unbounded Tier 0/1, never-decayed Tier 2, and
    a dense `n × n` PPR (415 MiB at 5k); only `console_api.py` uses it.
14. **Supervisor/A2A maps** — `loops` never evicted; A2A tasks have no TTL
    (feature-gated); snapshots would grow per step if wired.
15. **f32 vectors, brute force** — 1.5 KiB/vector; int8 is a measured ~4×
    reduction with rescoring.
16. **Memory is a tool, not a turn** — no automatic bounded recall, so
    continuity depends on model behavior.

## Plan

Full acceptance tests are in the report §7 and in
`docs/active-implementation-todos.md` §Memory.

- **P0:** MEM-01 global budget; MEM-02 observability; MEM-03 decay +
  consolidation; MEM-04 bridge budget; MEM-05 Python hygiene; MEM-06 retention
  + vacuum; MEM-07 automatic recall; MEM-08 compaction/offload; MEM-09 bounded
  transcript read; MEM-10 dedupe; MEM-11 temporal validity; MEM-12 vector
  quantization; MEM-13 tombstone/loop purge; MEM-14 memory gate; MEM-15 tiered
  storage.
- **P1:** MEM-16 hybrid retrieval + rerank; MEM-17 importance calibration;
  MEM-18 embedding single-flight cache; MEM-19 per-tenant byte accounting.
- **P2:** MEM-20 pgvector/HNSW after the cap; MEM-21 temporal graph
  projection; MEM-22 tiered storage only if served.

**Shared with the performance plan:** PERF-01 (instrument once), PERF-02
(transport/`Content-Length`), PERF-05 (SQLite pool/pragmas), PERF-08 (Aer
method/heap), PERF-12 (regression gate).

## Anti-findings

- Do not set `GOMEMLIMIT` far below the live heap or disable GOGC to "use all
  memory"; use ~90% of the cgroup with GOGC default first.
- Do not raise `MaxItemsPerBank` — it is per tenant, not global.
- Do not set Aer `max_memory_mb=-1` to bypass the check.
- Do not delete ledger events referenced by claims/manuscripts for space.
- Do not add HNSW before budgeting/quantization.
- Do not compact irreversibly; keep the transcript.

## Register

This page describes a **measured prototype on one host**, not a production
capacity plan. The 50k Hindsight and PPR benchmarks are synthetic; the
container arithmetic is the durable constraint and must be re-measured in the
deployed image. Tiered storage and supervisor/A2A findings are latent
(unserved or feature-gated).
