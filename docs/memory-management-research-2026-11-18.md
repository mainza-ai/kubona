# Memory management research & audit — 2026-11-18

> **What this is.** A measured audit of how Kubona manages memory across four
> layers — agentic memory, Go runtime/process memory, the Python compute
> bridge, and storage/retention — plus a 2026 practice review and an ordered
> `MEM-*` program. It is the companion to
> `docs/performance-research-2026-11-18.md`; the two plans share instrumentation
> and gate slices.
>
> Every number was produced on this host on 2026-11-18 at `5446b2a2`; absolute
> rates are trend evidence, not a capacity budget.

## 0. Execution status (2026-11-18)

Landed: **MEM-01…MEM-16, MEM-18, MEM-19**. Evidence: global item/byte budget
with startup `LIMIT` and fair-share eviction; Go/store memory gauges; decay
daemon started and `EffectiveScore` participates in recall ranking; cgroup
bridge budget (`Aer max_memory_mb`, per-job width derived from the budget,
in-flight 70%); `MALLOC_ARENA_MAX`/thread caps plus `malloc_trim`;
retention/vacuum; bounded automatic recall; deterministic extractive
compaction; 1,000-row transcript read; content/near-duplicate dedupe; temporal
validity (`valid_from`/`valid_to`/`superseded_by` + retain `supersedes`);
int8 vectors (1 byte/dim: 384 B vs 1,536 B per 384-dim vector) with f32 kept
for export; tombstone purge + terminal-loop eviction; hybrid semantic/lexical/
recency ranking; single-flight embedding cache; sparse PPR (5k entities no
longer allocates a 200 MB dense matrix).

Landed after the first pass: **MEM-17** `FeedbackScoped` calibrates importance
from explicit useful/harmful outcomes (beta prior, 50/50 blend), with a
`kubonad memory-feedback` operator path; **MEM-20** `kubonad memory-status`
reports budget pressure and pgvector readiness (sidecar still operator-
provisioned, never silently activated); **MEM-21** `kubonad memory-graph`
exports tenant-scoped temporal nodes and `supersedes` edges; **MEM-22** tiered
storage is explicitly quarantined (`UNSERVED` + import test). Bridge RSS is
published to `bridge-rss.json` and surfaced as `kubona_bridge_rss_bytes` when
fresh (<60 s).

## 1. Verdict

Kubona has a **real agentic memory model with production-shaped isolation and
provenance** (tenant + readership scoping, IFC lattice, retraction tombstones,
bounded banks), but its **memory budget is per-tenant and its lifetime policy
is partly inert**. Three facts define the gap:

1. **No global memory budget.** The store caps 500 items / 250k tokens per
   bank, per tenant, per readership. Measured: 50,000 memories cost **97.3 MiB**
   of live Go heap and take **169 ms** to load at startup. There is no total
   cap, no byte cap, and `loadFromDB` loads every tenant. At the 1 GiB daemon
   limit, roughly **80–130 tenants at full caps** exhaust the safe heap before
   GC headroom and process overhead (estimate, not a measurement).
2. **The advertised decay does not run.** `StartDecayDaemon` has **no caller**,
   and semantic recall ranks by cosine only — `EffectiveScore` is never read on
   the live path. `DecayRatePerHour`, `DecayInterval`, and the consolidation
   curve are dead configuration today.
3. **The compute bridge can be OOM-killed by its own advertised limit.** The
   bridge container is **4 GiB / 2 CPUs**. Measured peak RSS: Aer statevector is
   **1,100 MiB at 26 qubits**, **2,124 MiB at 27**, and **4,172 MiB at 28** — so
   `MAX_STATEVECTOR_QUBITS = 28` is over the container limit. The global
   in-flight rail defaults to **8 GiB** (2× the 4.29 GB single-job ceiling), and
   no `AerSimulator` sets `max_memory_mb`, whose default is *system* memory
   (128 GiB on this host), not the cgroup. Aer is not cgroup-aware.
4. **Memory is invisible.** `/metrics` exposes HTTP counters and ops gauges but
   **no Go heap, GC, RSS, cgroup, or memory-store gauges**; there is no pprof
   (shared with PERF-01) and no Python RSS/tracemalloc surface. This is why the
   gaps above survived.

Small anchors are not the problem: PySCF peaks at **107–224 MiB** for H2 →
benzene/6-31G*. The risk is the *unbounded* dimensions (tenants, transcripts,
events, context) and the *unguarded* quantum widths, not today's working set.

## 2. Method

1. Read the memory code: `pkg/memory` (Hindsight store, vectors, dormant
   pgvector), `api/memory_writeback.go`, `api/agent_tools.go` (recall tool),
   `llm/budget.go` + `llm/history.go`, `session.GetMessages`,
   `supervisor` maps, `cost/payment.go`, `agents/a2a.go`.
2. Read the runtime/compute limits: compose resource limits, `executor.py`
   caps and in-flight admission, every `AerSimulator(...)` call site, PySCF
   anchor code, the bridge Dockerfile.
3. Measured: Hindsight load/recall at 50,000 rows with real vectors; Aer and
   Qiskit `Statevector` peak RSS at 24–28 qubits; PySCF anchor peak RSS by
   molecule/basis; PPR memory at 2k/5k entities; a fresh daemon's idle RSS;
   host-ledger table counts and SQLite vacuum state.
4. Reviewed 2026 practice: agentic memory frameworks (Mem0, Zep/Graphiti,
   Letta/MemGPT, Hindsight), context compaction/offloading, Go `GOMEMLIMIT`,
   glibc/Python container memory, Qiskit Aer `max_memory_mb`, PySCF
   `max_memory`/density fitting, vector quantization, SQLite auto-vacuum, and
   Three.js/React disposal.

## 3. Inventory

### 3.1 Agentic memory (live)

- **Banks:** `world-facts`, `experiences`, `mental-models`
  (`pkg/memory/hindsight.go`).
- **Store:** in-memory maps plus SQLite tables `hindsight_memories` and
  `hindsight_vectors`; 384-dim float32 embeddings (1.5 KiB/vector), lexical
  hash fallback, optional HTTP embedder.
- **Isolation:** tenant + readership, enforced through the IFC `ReaderSet`
  lattice; recall never crosses tenant.
- **Writes:** one bounded experience per completed model turn (≤600 runes),
  importance by heuristic (proposal/tool use), best-effort and fail-closed on
  ghost sessions; manual `/api/memory/retain` (≤4,000 chars, secret-redacted).
- **Consolidation:** `Reflect` is called on autoresearch stagnation and writes
  a mental model; nothing schedules it.
- **Recall:** explicit `recall_memory` tool (250 tokens, 0.35 floor), HTTP
  recall for the Memory panel, and tribunal recall. No automatic per-turn
  retrieval.
- **Retraction:** `RetractScoped` clears content, deletes the vector, and keeps
  a tombstone row; no purge.

### 3.2 System / runtime memory

- **kubonad:** 1 GiB / 1 CPU; Go 1.27; no `GOMEMLIMIT` or `GOGC`; fresh daemon
  RSS **27.6 MiB** (empty ledger).
- **console:** 1 GiB / 1 CPU; E2E heap gate **≤250 MB**; Three.js disposal is
  present in `SpatialCanvas3D` (geometries, materials, controls, renderer).
- **backup sidecar:** 256 MiB / 0.5 CPU; `VACUUM INTO` snapshot + encryption.
- **SQLite:** WAL, `synchronous=NORMAL`, `cache_size=-2000` (~8 MiB **per
  connection**), `mmap_size=0`, `temp_store=0`, no pool limits, `auto_vacuum=0`.

### 3.3 Compute bridge

- 4 GiB / 2 CPUs, `python:3.11-slim` (glibc), `/tmp` as **tmpfs** (RAM), no
  `MALLOC_ARENA_MAX`, no `OMP_NUM_THREADS`/`OPENBLAS_NUM_THREADS`.
- `MAX_STATEVECTOR_QUBITS = 28`, `MAX_AER_SIM_QUBITS = 30`; per-job theoretical
  bytes `16 × 2^n`; global in-flight default **8 GiB**.
- Every simulator call uses `AerSimulator()` with no `max_memory_mb`; some VQE
  paths use `Statevector.from_instruction` (a ~4× peak over the state itself).
- PySCF anchors set no `mol.max_memory`, no density fitting, no chkfile cache.

### 3.4 Storage / retention

- Unbounded tables: `ledger_events` (1,470 rows), `chat_messages` (210),
  `hindsight_memories` (45) + vectors (45), `jobs` (44).
- `campaign_versions` is pruned; manuscript artifacts have a 256 MiB/session
  ceiling; `chat_messages` is deleted only by explicit clear/delete/regenerate.
- No `DELETE FROM ledger_events` exists; no retention policy for events,
  messages, jobs, or tombstones; no `incremental_vacuum`.

### 3.5 Dormant / unserved

- **Postgres + pgvector** store is dormant by design (Q-34) and unused.
- **Tiered storage** (`kubona/memory/tiered_storage.py`) is used only by the
  unserved `console_api.py` and tests: unbounded Tier 0 map, append-only Tier 1
  list, never-decayed Tier 2 graph, and a dense `n × n` PPR matrix.
- **Supervisor snapshots** are never saved in production (`SaveSnapshot` has no
  caller), but `loops` is never evicted; **A2A tasks** have no TTL (feature is
  disabled by default).

## 4. Measured baseline

### 4.1 Hindsight store at scale

Synthetic DB with the real schema, 50,000 memories + 384-dim vectors, one
tenant, loaded through `NewPersistentHindsightStore`:

| Metric | Value |
|---|---:|
| Startup load | **168.9 ms** |
| Live heap delta | **97.3 MiB** |
| Runtime `Sys` | 159.0 MiB |
| `SemanticRecallScoped` over 50k | **34.3 ms** (holds the write lock) |
| Per-item cost (derived) | ~1.9 KiB |
| 500-item bank (derived, linear) | ~1.0 MiB, ~0.34 ms |

`loadFromDB` has no `WHERE` and no cap: it loads every tenant and every
tombstone. The store's only caps are per `(bank, tenant, readership)`.

### 4.2 Quantum simulator peak RSS

Separate processes, shallow 24–28 qubit circuits, measured with
`resource.getrusage(RUSAGE_SELF).ru_maxrss`:

| Path | 24q | 25q | 26q | 27q | 28q |
|---|---:|---:|---:|---:|---:|
| `AerSimulator(method="statevector")` | 332 MiB | 588 MiB | **1,100 MiB** | **2,124 MiB** | **4,172 MiB** |
| `Statevector.from_instruction` | 1,093 MiB | — | **4,165 MiB** | **8,261 MiB** | — |

The bridge container limit is **4,096 MiB**. Aer at 28 qubits is already over
it; the Qiskit statevector path crosses it at 26. `max_memory_mb=3072` at 28
qubits makes Aer refuse (`Required memory: 4096M, max memory: 3072M`), but
`run().result()` returns a failed result rather than raising, so callers that
only call `get_counts()` do not get a clean typed refusal.

### 4.3 PySCF anchor peak RSS

| Molecule / basis | Wall | Peak RSS |
|---|---:|---:|
| H2 / STO-3G | 1.14 s | 107 MiB |
| H2O / 6-31G* | 0.32 s | 113 MiB |
| N2 / cc-pVDZ | 0.34 s | 113 MiB |
| benzene / 6-31G* | 2.20 s | **224 MiB** |

Small anchors are comfortable; the number is dominated by the Python/Qiskit
import baseline (~100 MiB). The missing guard matters only for larger
molecules/bases and for concurrent anchors.

### 4.4 Process and host

| Measurement | Value |
|---|---:|
| Fresh kubonad RSS (empty ledger) | **27.6 MiB** |
| Host ledger | 3.61 MiB |
| `hindsight_memories` / vectors | 45 / 45 |
| `ledger_events` | 1,470 |
| `chat_messages` | 210 |
| `jobs` | 44 |
| `sessions` | 77 |
| `auto_vacuum` / freelist | 0 / 0 |
| Console heap gate | ≤250 MB |

### 4.5 Dormant PPR

| Entities | PPR wall | Peak RSS |
|---|---:|---:|
| 2,000 | 0.02 s | 92 MiB |
| 5,000 | 0.09 s | **415 MiB** (+382) |

The dense `n × n` adjacency is 200 MB at n=5,000 and 3.2 GB at n=20,000. The
module is not served, but if it is ever wired as-is it becomes a memory cliff.

## 5. Findings

### F1 — No global memory budget (P0)

Per-bank/per-tenant caps are correct isolation, but total memory scales with
tenant count and bank count. `loadFromDB` loads all of it at boot, and the only
pressure valve (LRU eviction) runs on writes, not on reads or startup. Measured
1.9 KiB/item means a 1 GiB daemon cannot host an unbounded tenant population.
**Fix:** a global `MaxTotalItems`/`MaxTotalBytes` (e.g. 200k items / 300 MiB of
vectors) enforced on retain *and* load, with a startup trim and a gauge.

### F2 — Memory is unobservable (P0)

No Go heap/GC/RSS/cgroup gauges in `/metrics`, no pprof (PERF-01), no Python
RSS/tracemalloc, no memory-store item/vector counts. The 28-qubit OOM risk and
the tenant-scaling risk are both invisible until the kernel kills a container.
**Fix:** MEM-02 gauges from `runtime/metrics`, `/proc/self/statm`, cgroup
`memory.current`, store counts, and a bridge health message; share the pprof
and perf-gate work with PERF-01/PERF-12.

### F3 — Decay is dead code (P1)

`StartDecayDaemon` is never called and `SemanticRecallScoped` ignores
`EffectiveScore`; only the lexical `Recall` sorts by it, and that path is not
the production recall tool. The configured 1%/hour decay and the 0.1 floor have
no effect. Stale experiences rank exactly as high as fresh ones.
**Fix:** either start the daemon with the server context and include
`EffectiveScore` (or an explicit recency prior) in semantic ranking, with
tests; or delete the dead config. Do not leave a claimed lifetime policy
inert.

### F4 — Embeddings run under the store lock (P1)

`RetainScoped` and `SemanticRecallScoped` both hold `s.mu.Lock()` while calling
the HTTP embedder (5 s timeout). One slow embedding endpoint freezes recall,
retain, and tribunal memory for every tenant. The lexical fallback is fast, so
the risk is latent until `KUBONA_EMBEDDING_URL` is configured — which is the
intended production path.
**Fix:** embed outside the lock, then take the lock only to read/insert; add a
single-flight cache keyed by content/query hash.

### F5 — No dedupe or consolidation (P1)

Every turn appends a new experience; repeated prompts create near-duplicates
that consume the 500-item cap and evict genuinely useful history. There is no
content hash, no similarity merge, and no scheduled consolidation (only the
autoresearch `Reflect` call).
**Fix:** content hash + near-duplicate cosine threshold, merge into an existing
item (importance/count bump) instead of appending; nightly `Reflect` on the
highest-importance cluster, bounded.

### F6 — No temporal validity / supersession (P1)

A contradiction is additive: both memories remain and both can be recalled.
Retraction is manual and terminal. 2026 memory systems (Zep/Graphiti) model
validity windows and invalidate superseded facts while keeping history.
**Fix:** add `valid_from`/`valid_to`/`superseded_by`; writes that contradict a
known fact close the old interval; recall defaults to `valid_to IS NULL`, with
an explicit "as of" query for history.

### F7 — Context is truncated, not compacted (P0)

`BudgetHistory` keeps the newest 40 messages / 48k chars and drops the rest
with a boundary marker. There is no summary, no offload of large tool results,
and no reversible pointer. Research (ACON 2026, Claude/OpenAI/LangChain
compaction) reports **26–54% lower peak tokens** and better long-horizon task
success from compaction/offloading versus truncation.
**Fix:** MEM-07/08 — threshold-triggered summary of older turns, offload large
observations to a durable store with a pointer, keep the full transcript in the
ledger (already immutable) so compaction is reversible.

### F8 — Transcript reads are unbounded (P1)

`GetMessages` selects **all** rows for a session, then callers budget. A long
session loads and unmarshals its entire history on every chat, proposal,
regenerate, and context-read request. The 48k cap applies *after* the load.
**Fix:** SQL-side `ORDER BY created_at DESC LIMIT N` (plus a paginated
`before_id`) and a streaming budget; never materialize the full transcript for
a turn.

### F9 — No retention or vacuum policy (P0)

`ledger_events` is append-only forever; `chat_messages`, `jobs`, and retracted
memories have no retention. `auto_vacuum=0` and no `incremental_vacuum` means
deleted rows never return disk. The 256 MiB backup sidecar runs `VACUUM INTO`
(a second copy of the DB) and will OOM before the DB is large.
**Fix:** explicit retention classes (events referenced by claims/manuscripts
are preserved), a reaper with `PRAGMA incremental_vacuum(N)`, `auto_vacuum =
INCREMENTAL` via a one-time `VACUUM`, and a backup temp-space budget.

### F10 — Bridge memory limits are internally inconsistent (P0)

`MAX_STATEVECTOR_QUBITS=28` is 4.29 GB of state; measured Aer peak is 4,172 MiB
and the container is 4 GiB. The in-flight rail defaults to 8 GiB. Aer's
`max_memory_mb=0` uses **system** memory (128 GiB here), not the cgroup, so Aer
will attempt allocations the container cannot hold.
**Fix:** derive a `BRIDGE_MEM_BUDGET` from cgroup v2 `memory.max` (fallback
env), set `max_memory_mb` on every `AerSimulator`, cap the per-job ceiling at a
measured safe width (27 qubits = 2.1 GiB peak; 26 = 1.1 GiB), set the in-flight
cap to ~70% of the budget, and preflight-refuse before allocation with a typed
error. Check `result().status` for Aer's own refusal.

### F11 — Two statevector paths with different memory profiles (P1)

Aer uses ~1× the state; `Statevector.from_instruction` peaks at ~4× (26q:
4,165 MiB vs 1,100 MiB). VQE currently stays small (H2/LiH), but any future
wide-state path inherits the 4× profile and the 4 GiB container.
**Fix:** standardize on Aer for wide states or add an explicit
`Statevector`-path budget check; record the peak model used for admission.

### F12 — No Python allocator/thread hygiene (P1)

The bridge is glibc Python with Qiskit/NumPy. No `MALLOC_ARENA_MAX`, no
`OMP_NUM_THREADS`/`OPENBLAS_NUM_THREADS` bound to the 2-CPU limit, no
`malloc_trim` after large jobs. 2026 container practice: `MALLOC_ARENA_MAX=2`,
cap BLAS/OMP threads to the CPU limit, trim after big payloads; the Lean image
already sets `MALLOC_ARENA_MAX=2` — the bridge does not.
**Fix:** set the env in the bridge image/compose, add `malloc_trim(0)` after
job completion, and expose RSS in the health response.

### F13 — Dormant tiered storage is a latent cliff (P2)

Tier 0 and Tier 1 are unbounded, Tier 2 never decays, and PPR builds an
`n × n` dense matrix (415 MiB at 5k entities). It is only used by the unserved
`console_api.py` and tests.
**Fix:** either delete/clearly quarantine it, or add caps + a sparse power
iteration (no dense adjacency) before any wiring.

### F14 — Supervisor/A2A maps grow without eviction (P2)

`Supervisor.loops` is never pruned (rehydrated in full at boot); `A2A.tasks`
has no TTL (A2A is disabled by default); `snapshots` would grow per step if
`SaveSnapshot` were wired. Today's 17 loops are harmless; a long-lived daemon
creating loops forever is not.
**Fix:** evict terminal loops after a retention window, cap snapshots per loop
(e.g. last 50 + best), TTL completed A2A tasks, and add a map-size gauge.

### F15 — No vector quantization or ANN (P2)

Each memory costs ~1.5 KiB of vector plus ~0.4 KiB of content/maps; recall is
brute-force cosine. At 500/bank this is fine. At 100k+ vectors, int8 scalar
quantization is a measured ~4× reduction (industry practice) with negligible
recall loss and optional f32 rescoring; HNSW/pgvector is the scale path
(already coded but dormant).
**Fix:** store int8 (or float16) with per-dimension calibration and f32
rescoring; keep the f32 vector only for rescoring or drop it after a recall
gate; revisit pgvector only when the global cap is reached.

### F16 — No automatic working-set retrieval (P1)

Memory is a tool call, not part of the turn. Mem0/Letta-style systems inject a
small, query-relevant memory slice before generation. Kubona relies on the
model choosing `recall_memory`, so continuity depends on model behavior.
**Fix:** inject a bounded (e.g. 250-token) recall of the user's turn into the
system context, with the same tenant/IFC boundary, and keep the explicit tool
for deeper recall. Measure token cost and task continuity.

## 6. 2026 practice notes

| Area | 2026 guidance | Kubona status |
|---|---|---|
| Agentic memory | Mem0: extraction + dedupe/update (~90% token savings vs full history); Letta: agent-managed blocks + background "dreaming"; Zep/Graphiti: temporal validity; Hindsight: hybrid retrieval + rerank | Banks + vector recall + tombstones; no dedupe, no temporal windows, no rerank |
| Context compaction | Threshold compaction + tool-result offload + filesystem preservation; ACON 26–54% peak-token reduction | Truncate oldest turns only |
| Go containers | `GOMEMLIMIT` at ~90% of cgroup, leave 5–10% headroom; read `memory.max`; observe runtime vs cgroup | No limit, no gauges |
| Python containers | `MALLOC_ARENA_MAX=2`, cap BLAS/OMP threads, `malloc_trim`, soft `RLIMIT_AS` for catchable failure | None |
| Qiskit Aer | `max_memory_mb` is a hard refusal; default 0 = system memory, not cgroup; method choice dominates memory | Unset everywhere |
| PySCF | `max_memory` config/`~/.pyscf_conf.py`; density fitting and out-of-core for large systems | Defaults, no DF |
| Vector memory | int8 ~4×, binary up to 32× (index-only; rescore f32) | f32 only, brute force |
| SQLite | `auto_vacuum=INCREMENTAL` + scheduled `incremental_vacuum`; retention before vacuum | `auto_vacuum=0`, no reaper |
| Browser 3D | Explicit `dispose()` for geometry/material/texture/renderer; heap canary | Disposal present; 250 MB heap gate |

## 7. MEM program (ordered, mapped to PERF)

Each slice lands only with the named check. `PERF-*` references are the
performance plan in `docs/active-implementation-todos.md`.

### P0 — make memory visible and bounded

| ID | Slice | Change | Acceptance | PERF link |
|---|---|---|---|---|
| MEM-01 | Global budget | `MaxTotalItems`/`MaxTotalBytes`, startup trim, per-tenant fair share | 50k-item test stays under budget; eviction deterministic; no cross-tenant eviction | PERF-05 |
| MEM-02 | Memory observability | `/metrics` Go heap/GC/RSS/cgroup + store counts; bridge health RSS; heap profile behind bearer | Gauges visible; heap profile refuses without bearer; dashboard covers Go/bridge/console | PERF-01 |
| MEM-03 | Decay + consolidation | Start the decay daemon with the server context; use `EffectiveScore`/recency in semantic ranking; scheduled bounded `Reflect` | Old low-value memory ranks below fresh equivalent; daemon test; no unbounded mental models | — |
| MEM-04 | Bridge budget | cgroup-derived budget; `max_memory_mb` on every Aer simulator; per-job cap 27q/2.1 GiB; in-flight ≤70% of budget; typed preflight refusal; check result status | 28q refuses before allocation under a 4 GiB budget; 26q runs; concurrent jobs stay under budget; test for the refusal | PERF-08 |
| MEM-05 | Python hygiene | `MALLOC_ARENA_MAX=2`, OMP/BLAS threads = CPU limit, `malloc_trim(0)` after jobs, soft `RLIMIT_AS` | RSS curve flat after repeated 26q jobs; no OOM in a 30-job soak | PERF-08 |
| MEM-06 | Retention + vacuum | Retention classes for events/messages/jobs/tombstones; reaper; `auto_vacuum=INCREMENTAL` + one-time VACUUM; backup space budget | DB stops growing under a synthetic 100k-event month; freelist reclaimed; referenced evidence preserved | PERF-05 |
| MEM-07 | Automatic recall | Bounded pre-turn recall injection through the same tenant/IFC path | Scripted continuity task improves vs tool-only; injected tokens ≤250; cross-tenant test | — |
| MEM-08 | Compaction/offload | Summarize old turns at a threshold; offload large observations with a durable pointer; keep the transcript | 100-turn scripted session: peak tokens down ≥30%, key facts retrievable; reversible | — |
| MEM-09 | Bounded transcript read | SQL-side limit + pagination, stream budget | 100k-message session reads newest N in <50 ms with bounded heap | PERF-02 |
| MEM-10 | Dedupe/merge | Content hash + near-duplicate cosine merge, importance bump | 100 repeated turns → ≤2 memories; cap not exhausted; audit retains source ids | — |
| MEM-11 | Temporal validity | `valid_from`/`valid_to`/`superseded_by`; contradiction closes the old interval | Contradictory recall returns current only; "as of" query returns history | — |
| MEM-12 | Vector compression | int8 quantization + f32 rescoring (or f16), optional ANN | ≥4× vector memory cut; recall@10 ≥0.95 of f32 on a labeled set | — |
| MEM-13 | Tombstone/loop purge | Purge retracted rows after retention; evict terminal loops; cap snapshots; A2A TTL | 100k retractions don't grow heap; loop map bounded; map-size gauge | — |
| MEM-14 | Memory gate | Peak-RSS benchmarks: Hindsight 50k, Aer 24/26/28, PySCF benzene, console heap | CI fails on >10% regression; artifacts recorded | PERF-12 |
| MEM-15 | Tiered storage | Delete/quarantine or cap Tier 0/1 and replace dense PPR with sparse iteration | n=5k PPR <100 MiB; no unbounded tier; module clearly marked unserved | — |

### P1 — quality and scale

- Automatic hybrid retrieval (semantic + lexical + recency) with a reranker,
  measured on a labeled recall set (ties to MEM-07).
- Importance calibration from outcomes (claim accepted/refuted, user feedback)
  rather than fixed turn-shape heuristics.
- Embedding single-flight cache keyed by content/query hash (also fixes F4).
- Per-tenant byte accounting and fair-share eviction (MEM-01 refinement).
- Compression of old embeddings or dropping f32 after int8 calibration
  (MEM-12 refinement).

### P2 — architecture

- pgvector/HNSW sidecar only after the global cap is reached; keep the SQLite
  path as the local default.
- Temporal graph projection (entities/relations/validity) if MEM-11 lands and
  the graph queries justify it.
- Tiered storage only if a served surface needs it; otherwise remove it from
  the blueprint to avoid a false capability claim.

## 8. Anti-findings (do not do)

- **Do not set `GOMEMLIMIT` far below the live heap** or disable GOGC to "use
  all memory" — research shows that pins the heap goal at the limit and can
  worsen OOM/CPU behavior. Use ~90% and keep GOGC default initially.
- **Do not raise `MaxItemsPerBank`** to "keep more"; it scales per tenant, not
  globally, and makes the unbounded dimension worse.
- **Do not set Aer `max_memory_mb=-1`** to bypass the check.
- **Do not delete ledger events referenced by claims/manuscripts** to reclaim
  space; retention must preserve evidence and export boundaries.
- **Do not add HNSW before quantization/budgeting**; the index graph adds
  memory of its own.
- **Do not compact irreversibly.** Keep the full transcript; summaries are a
  view.

## 9. Sources

**First-party:** `pkg/memory/{hindsight,vector,postgres}.go`,
`api/memory_writeback.go`, `api/agent_tools.go`, `llm/{budget,history}.go`,
`session/session.go`, `supervisor/supervisor.go`, `agents/a2a.go`,
`cost/payment.go`, `bridge/{executor,chemistry,experiments}.py`,
`kubona/providers/qiskit_provider.py`, `kubona/memory/tiered_storage.py`,
`platform/docker/compose.prod.yml`, `Dockerfile.bridge`, `/metrics`
(`pkg/api/metrics.go`), console `runtime-budget.spec.ts` and
`SpatialCanvas3D.tsx`.

**2026 practice:** Mem0/Zep/Graphiti/Letta comparisons (2026);
Claude context-engineering cookbook (compaction/tool clearing, Jan 2026);
LangChain Deep Agents context management (offload + summarization, Jan 2026);
ACON (ICML 2026, 26–54% token reduction); "What to Keep, What to Forget"
(rate–distortion compaction, 2026); Go `GOMEMLIMIT`/cgroup guidance (2026);
glibc arena/Python container RSS guidance (2026); Qiskit Aer 0.18
`max_memory_mb` docs; PySCF `max_memory`/density-fitting docs; vector
quantization docs (MongoDB/Qdrant/Azure/ScyllaDB, 2026); SQLite auto-vacuum
docs; Three.js disposal manual.

## 10. Caveats

- The 50k Hindsight benchmark is **synthetic** (the host store has 45
  memories); it measures scaling, not production content.
- Peak-RSS numbers are **per process on this host**, with the LLM server
  resident. The container arithmetic (4 GiB) is the durable constraint; the
  exact headroom should be re-measured in the deployed image.
- The 4× `Statevector` peak is path-specific; Aer is ~1×. Do not apply the 4×
  factor to Aer admission.
- The "80–130 tenants" figure is an estimate from measured per-item cost and
  GC headroom, not a load test.
- The tiered-storage and supervisor/A2A findings are **latent** (unserved or
  feature-gated) and are labelled as such.
- No paid-QPU memory evidence is included; vendor-side memory is not
  observable from this repo.
