# Active implementation TODOs — 2026-11-18

Local-only execution list. No push until explicitly requested.
Baseline: `fe31d1af` on `develop` (post-Lean live proof, 2026-11-18).
Audit: `docs/production-audit-2026-11-18.md` (K-01…K-14).

## Now

- [x] Proposal history/list surface (server list + console panel).
- [x] Typed need envelopes: `budget_exceeded`, `approval`, `anchor_missing`.
- [x] Refusal chip renders the typed need sentence and remedy.
- [x] Run card renders typed budget/retry needs; resume clears them.
- [x] Admission needs render in the refusal chip (no job exists).
- [x] Table citation scanning + inline table highlights.
- [x] Rendered math-source citation scanning.
- [x] Attention Director wiring for notification severities.
- [x] Live-run toast duplication removed (single polite region).
- [x] Remaining per-surface toast/chip dedupe.
- [x] Proposal-dismiss reasons consumed by proposal generation (prompt signal).
- [x] Skill authoring/installer UI.
- [x] Live `eval-llm` model invocation (explicit provider/model).
- [x] General ledger-write secret screening.
- [x] Proposal pricing rule for plans without a numeric bound: the mint
      refuses a metered backend with no numeric `max_cost_usd` (text estimates
      are not a spend bound); covered by
      `TestProposalMintRefusesUnpriceablePlans`.

## Copilot drawer containment (2026-11-17)

- [x] C1: constrain MathText prose/table min-content width; add browser regression.
- [x] C2: Copilot-specific visible splitter, width persistence, reset, and viewport clamp.
- [x] C3: table semantics, numeric alignment/formatting, scroll cue, table copy row.
- [x] C4: full target label/wrap and separate Δ badge.
- [x] C5: mobile/desktop width and resize E2E coverage.
- [x] C6: table copy and Narrow/Wide/Reset presets (full-screen table expansion remains optional).

## Production audit slices (2026-11-18)

Executed 2026-11-18; each box is closed only by a test or a live run named in
`docs/production-audit-2026-11-18.md` §11.

- [x] R0: no fabricated/static figures; `compile_latex` refuses SVG; the
      bilingual constructor takes real caller figures; convergence data is a
      table (tests pin empty/no-figure behavior).
- [x] R1: `manuscripts`, `manuscript_sections`, `manuscript_reviews`,
      `manuscript_artifacts`, `manuscript_signoffs`, `manuscript_citations`
      tables; tenant-scoped routes under `/api/sessions/{id}/manuscripts`;
      drafts built server-side from the stored campaign snapshot; immutable
      after review.
- [x] R1: OpenAPI + console contract entries (180 modeled / 15 client-only /
      0 unmodeled, 156 schemas, 157 paths); cross-tenant refusal tests;
      content hash stable across reads.
- [x] R2: receipts resolved against the campaign's signed receipts; proof
      hashes resolved against comparator events (none exist yet, so a
      supplied hash stays unverified); `manuscript_drafted/reviewed/signed/
      built/deposit_staged` events; signoff binds the content hash; forged
      references are named unresolved.
- [x] R3: pdfTeX in the bridge image; `\input` sections; `\includegraphics`
      for declared PNG/PDF/JPEG; `references.bib`; artifact SHA-256;
      byte-identical rebuilds (test + live).
- [x] R3: source-change refusal: a recomposition whose hash differs from the
      signed content hash returns 409 `source_changed`.
- [x] R4: Publications panel in Results Studio: create draft, re-review,
      build, operator signoff, deposit staging, artifact download; browser tests
      cover the empty/create/build/signoff-refusal/deposit-disabled states.
      Annotations are durable manuscript review notes (`manuscript_annotations`
      table + add/resolve routes), and signoff refuses while any annotation is
      open (A1).
- [x] R5: Zenodo manifest generated from built artifacts with checksums;
      refuses without an ACCEPT review and a valid signoff; human upload only
      (`auto_publish:false`, `doi:null`).
- [x] R6: `/metrics` adds job-status, manuscript-state, bridge-configured, and
      backup count/age gauges; `kubonad` mounts `kubona-backups` read-only;
      `backup-status`/preflight fail when encryption is declared but the newest
      backup is plaintext; `scripts/backup-restore-check.sh` restores the real
      scheduled artifact and re-verifies the chain.
- [x] R6 operator: `KUBONA_BACKUP_AGE_RECIPIENT` + `REQUIRE_ENCRYPTION=1` are
      set, encrypted backups restore clean, and `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1`
      is now set (preflight refuses without a fresh backup).
- [x] R7: citations persist in `manuscript_citations` and become a resolved
      BibTeX file with `\cite` keys; novelty distance is recorded as measured or
      explicitly unmeasured. Citations carry `claim_id`/`source_id`/`verified`,
      a bound claim is checked against the ledger, and the Publication panel
      searches OpenAlex and adds/removes references (A2a/A2b).
- [ ] Operator: fresh-volume reset to restore the declared `local_private`
      posture (documented procedure; preserves a verified snapshot).

## Agentic science frontier (2026-11-18)

Research: `docs/agentic-science-frontier-2026-11-18.md`; wiki
[[agentic-science-frontier-2026]]. Ordered so each slice produces a measured
capability, not a demo.

- [x] AG-1a: benchmark runner + **11-task** catalog + unit tests; disposable
      stack baseline published (8/11 pass@1; TFIM 4/6/8 miss their exact
      anchors at 11–54σ; QAOA 2/3 layers pass after the bit-order fix; H2
      ground state reaches chemical accuracy at 20k shots). See the frontier
      report §8 and `docs/accuracy-gap-investigation-2026-11-18.md`.
- [x] AG-1b: `.github/workflows/agentic-bench.yml` nightly/dispatch run with
      `--allow-failures`, a retained JSON artifact, and failure-only stack logs.
- [x] AG-2a: persisted hypothesis engine (evidence-grounded generation,
      dedupe, ranking, falsifier, literature refs, explicit novelty status,
      shortlist API + tests). 10 hypotheses on the H2 campaign.
- [x] AG-2b: strict-JSON LLM proposer with honest fallback; deterministic Elo
      tournament + derived meta-review; `lexical_containment_v1` measured
      novelty; evidence-gated collapse into a cited claim; shortlist-before-
      plan gate. Schema 7.
- [x] AG-3a: `experiment_plans` table/API; plan requires acceptance metric +
      stop rule + spec; approval gate; execute through the job rail and score
      the observable against the acceptance target.
- [x] AG-3b: `pkg/plans.Synthesize` merges campaign facts + the hypothesis's
      relative `spec_patch`; provenance is persisted (`spec_basis`); ungrounded
      methods/targets refuse with `spec_unavailable`. Schema 8.
- [x] AG-4a: `GET /research-graph` projects campaign, hypotheses, plans, jobs,
      claims, and manuscripts from the ledger.
- [x] AG-4b: `pkg/researchgraph` priority summary inside a byte budget with a
      truncation report; `Budget_bytes` validation; `since_seq`/`event_limit`
      resume cursor over the ledger chain.
- [x] AG-5: `pkg/verify` numeric + citation verification; `POST/GET /verify`
      with a content-hash-bound report; signoff blocks fabricated numbers and
      dead citations (an unavailable resolver requires an explicit, sealed
      acknowledgment); `kubonad verify-calibration` reports 12/12 against
      maintainer labels. Schema 9.
- [x] AG-6a: RO-Crate 1.1 metadata in `manuscript-export` (files, hashes,
      content hash, signoff).
- [x] AG-6b: `pkg/prov` W3C PROV-JSON + CRediT attribution; the export writes
      `prov.json`/`credit.json` and folds PROV-O nodes into the RO-Crate.
- [x] AG-7: `pkg/autonomy` stage modes (suggest/approve/act), v1 risk
      classifier (hardware/dual-use/budget + large-simulation), enforced at
      plan execution, persisted policy, and an operator halt that cancels live
      jobs. Schema 10.
- [x] AG-8a: `kubona_hypotheses{status}` + `kubona_experiment_plans{status}`
      gauges.
- [x] AG-8b: `GET /api/ops/science` with stage taxonomy, cost per accepted
      claim (actual-vs-reserved basis), human intervention counts with human
      minutes explicitly unmeasured, queue/loop health; matching Prometheus
      gauges.
- [x] AG-9: tenant-scoped `lab_knowledge` registry (negative results, claims,
      reports) with fingerprint dedupe, provenance, a prior-negative gate on
      plan creation, and owner-granted sharing. Schema 11.

## Agentic mode and oversight (2026-11-18)

Research/design report: `docs/agentic-mode-toggle-2026-11-18.md`.

- [x] AG-17: session agentic master switch (`agentic_enabled`) + operator lock
      (`agentic_locked`); session `PUT /agentic`; operator policy merge; halt
      lock; gates on loop start/resume/wake/step, plan execution, and restart
      rehydration; Autoresearch panel switch/status; Go and browser tests.
- [x] AG-18: global Manual/Auto/Locked indicator (status bar / command palette).
- [x] AG-19: durable approval inbox (`/approvals`, approve/reject/defer,
      restart-safe) with a console panel and browser tests. Reviewer edits are
      validated and risk-reclassified; rejection feedback persists on the plan;
      undo restores the pre-decision protocol.
- [x] AG-20: `notify` stage mode executes low/medium risk and seals
      `notify:true` receipts; `/agentic/digest` batches them and the console
      digest panel renders them (empty/unavailable states included).
- [x] AG-21: per-tool `allow|ask|block` policy enforced at copilot dispatch.
      `ask` records a durable one-time approval bound to the tool and the
      canonical argument digest; grants are spent once within a 15-minute TTL,
      and deny/spend/expiry all refuse. Schema 14.
- [x] AG-22: approval-fatigue telemetry (queue depth, latency percentiles,
      rubber-stamp proxy).
- [x] AG-23: `/receipts` + `/receipts/{id}/undo` for mode flips, unrun plan
      decisions, and deferrals; double-undo refused. Job/claim undo remains
      intentionally unsupported (immutable evidence).
- [x] AG-24: operator-owned tenant default (`/api/tenants/agentic-default`),
      policy history + sealed rollback, strict unreadable-policy refusal, and an
      expanded dual-use taxonomy. The default is operator-owned, not an
      admin-signed tenant action; document that boundary.

## Accuracy-gap follow-ups (2026-11-18)

Root-cause report: `docs/accuracy-gap-investigation-2026-11-18.md`.

- [x] AG-10: QAOA simulator shot scoring uses Qiskit bit order (shared
      `_counts_cut`, unit + 20k-shot tests); benchmark 11 tasks with
      depth-appropriate QAOA targets and 20k-shot accuracy tasks; runner records
      `uncertainty`/`z`; Inspector header wraps with a real-browser non-overlap
      test.
- [x] AG-11: TFIM multi-start optimizer (3–4 restarts; keep best) and
      gap-based `converged`/`optimizer_status`. Acceptance: n=8 4 layers ≤0.10 Ha
      and 6 layers ≤0.05 Ha statevector gap; small-n exactness preserved.
- [x] AG-12: noise-aware benchmark scoring (`uncertainty`, z, perfect-state
      control). Acceptance: exact-state TFIM control passes at task shots;
      fabricated results still fail.
- [x] AG-13: H2 chemical-accuracy task at 1.6 mHa; evaluate HVA/UCCSD and
      optimizer restarts.
- [x] AG-14: QAOA multi-start and depth tasks; document the p=2 ceiling.
- [x] AG-15: console container-query/width-aware panel controls; layout audit
      for every multi-group panel header.
- [x] AG-16: telemetry semantics (`optimizer_status`, `execution_time_ms`,
      QAOA `energy` naming).

## Follow-up audit slices (2026-11-18, second pass)

Findings and evidence: `docs/publication-followup-audit-2026-11-18.md`.
F-01…F-10 (nested artifacts, order-sensitive hash, citation collisions, target
escaping, size checks, build limiter, schema v2, preflight env, figure-hash
verification, receipt cap) are landed with tests.

- [x] A1: `manuscript_annotations` table + routes; annotations survive reload;
      signoff refuses while any annotation is open; panel add/resolve UI and
      browser tests. (closes G-03)
- [x] A2a: citations gain `claim_id`/`source_id`/`verified`; a bound claim is
      verified against the ledger (unknown claim → 400); `citation_distance`
      is accepted, validated, and passed to the review (measured or unmeasured).
      (part of G-05, G-13)
- [x] A2b: the Publication panel searches OpenAlex and adds/removes references;
      `PUT /citations` recomposes the draft; source ids persist. (G-05)
- [x] A3 (boundary): operator-run `kubona.eval.comparator_cli` refuses any
      non-kernel result, signs a canonical attestation; the daemon verifies the
      configured comparator key and records `formal_verification`; ACCEPT +
      signoff + deposit exercised end to end with a signed attestation.
      (G-04 boundary; the live Lean run needs the pinned sandbox image)
- [x] A4a: per-session artifact byte ceiling
      (`KUBONA_MANUSCRIPT_ARTIFACT_CEILING_BYTES`, default 256 MiB) enforced on
      build/deposit; delete removes rows + artifacts + events; artifact-bytes
      gauge. (part of G-06)
- [x] A4b: `kubonad manuscript-export --session --out` exports metadata,
      sections, citations, reviews, annotations, signoff, and artifacts
      (hash-checked) from a read-only ledger. Exported evidence is the
      precondition for deleting a lab/session. (G-06)
- [x] Operator gate: `kubonad preflight --strict` refuses an unacknowledged
      plaintext backup (`KUBONA_BACKUP_PLAINTEXT_ACK=1` explicitly accepts).
- [x] Operator: comparator key generated (`~/.kubona/comparator.key`) and the
      public key configured on the daemon (`KUBONA_COMPARATOR_PUBKEY_HEX`).
- [x] Operator: warm the Lake cache and run a real challenge. **Investigated
      2026-11-18:** upstream publishes no `leanprover/lean4` Docker image (the
      Hub repository API returns 404; the community image is a stale 2024
      snapshot), and the official manual install is elan. `Dockerfile.lean`
      builds on `debian:trixie-slim` from the checksummed
      `lean-4.34.1-linux{,_aarch64}.tar.zst` release assets (SHA-256 pinned per
      architecture; `TARGETARCH` selects it) and installs comparator v4.34.0
      (last 4.34-line tag; comparator merged into Lean core at v4.35.0) with
      `lean4export` v4.34.0 and landrun v0.1.17. The platform invocation is
      corrected from `lake exe comparator <name>` (never a target) to
      `lake env comparator ComparatorChallenges/<name>.json`, and the sandbox
      runs as the checkout-owner UID with `HOME=/tmp`
      (`KUBONA_SANDBOX_USER` overrides VM-remapped mounts). **Warm + live proof
      completed:** `lake update` + `lake exe cache get` (8,908 Mathlib files);
      `ThreeStateSupercritical` built 8,950 targets, exported both
      environments, and the Lean kernel accepted the solution
      (`live_kernel_proven`; propext/Quot.sound/Classical.choice; 0 `sorryAx`).
      The signed attestation was accepted by the API (HTTP 201, event 780,
      proof hash `4762a59cac58…`). Host requirement: run on a standard Linux
      kernel with working landrun (Colima virtiofs, a Linux host, or CI);
      Docker Desktop's linuxkit kernel denies landrun's rules. Runbook:
      `docs/lean-comparator-runbook-2026-11-18.md`.

## Later

- [x] Hardware parent allowance rails.
- [x] Hardware-in-the-loop VQE/QAOA (bridge implements both; live evidence recorded).
- [x] Plan draft persistence.
- [x] Obligation 1 h ageing escalation.
- [x] Obligation push events for session-owned rows (key-owned still polls).
- [x] Chart-series and campaign-level fact refs.
- [x] Run parts from literature/prover/tool rails — out of scope until a real loop identity exists.
- [x] Legacy-surface decision: `WidgetStage`, the Recorder, and the Manifold
      stay as explicitly proposal-only/scroll products until object cards reach
      five-widget parity and the old aliases complete a release of support
      (recorded in [[remaining-work]]); `KUBONA_CONSOLE_WORKBENCH=0` is the
      tested rollback. This is a decision, not an unfinished migration.
- [x] Console: 0 renderer migrations (dependency removed).
- [x] Console: coupling ratchet at 54 files / 55 imports (may only shrink).
- [x] Console coupling batch: memory/jobs/session transport moved behind
      domain facades; ratchet lowered 54/55 → 50/51 (may only shrink further).
- [x] Console: split the ~395 KiB chunk (largest now ~277 KiB).
- [x] Console: LCP/INP/heap/request-count gates.
- [x] Console: raw-palette token baseline is 0.
- [x] Console: raw-button ratchet (368 baseline, may only shrink).
- [x] Console: form-control ratchet at 46 files / 202 controls (may only shrink).
- [x] Console forms batch: Autoresearch and Memory controls moved to
      Field/Select (compact `hideLabel`); ratchet lowered 46/202 → 44/195.
- [x] Console motion review: 0 `transition-all`, 0 arbitrary duration/ease/
      animate values (now banned outright by `console-token-lint`); UI
      transitions use 150–300 ms and ambient surfaces 500–1000 ms with explicit
      properties. Autoresearch/Memory/Approval surfaces checked in this pass.

## Performance (2026-11-18)

Research, measurements, and the full argument:
`docs/performance-research-2026-11-18.md`; wiki
[[performance-engineering-2026-11-18]]. Every slice names the check that
closes it. Baseline (this host, `bbcd2958`): `/api/health` 24–32k rps at c=20,
0.6–0.8 ms mean, 1.9 ms p99; `/api/sessions` 1.3–1.7k rps, 11–15 ms mean,
161 ms p99, 9,811 B body (2,159 B gzipped); console first load 1,135.6 KiB raw
/ 336.1 KiB gzip, largest chunk 276.6 KiB; 102/214 TSX files are client
components. Do not treat absolute rates as a budget — the host runs a resident
LLM server.

### P0 — instrument and remove mechanical costs

- [ ] PERF-01: operator-gated `pprof` (bearer-only, off by default) and Go
      benchmarks for `ListSessions`, `GetEventsSinceWithGlobalTypes`,
      `AppendEvent`, `checkAuth`; `scripts/perf-smoke.sh` reproduces the §3
      table. Closes: the 161 ms p99 on `/api/sessions` becomes attributable;
      benchmark artifacts recorded.
- [ ] PERF-02: set `Content-Length` in `writeJSON`/`WriteAPIError` (keep
      chunked only for streams). Evidence: chunked vs CL on a 10 KB body is
      30,376 vs 98,665 rps at c=20.
- [ ] PERF-03: gzip level 1 (then optional zstd) for JSON > 1 KiB, `Vary:
      Accept-Encoding`, pooled writers, SSE skipped. Evidence: 9,811 → 2,159 B
      (4.54×) at 51 µs per 14 KB.
- [ ] PERF-04: `idx_sessions_updated(updated_at DESC)` +
      `idx_sessions_tenant_updated(tenant_id, updated_at DESC)`, ledger schema
      16 with a verified pre-migration snapshot. Evidence: 100k rows 6.23 →
      0.06 ms (operator), 5.36 → 0.09 ms (tenant).
- [ ] PERF-05: bound the SQLite pool (`SetMaxOpenConns`/`SetMaxIdleConns`),
      apply PRAGMAs once at open, `temp_store=MEMORY`, measured `mmap_size`;
      evaluate a read-only pool for GETs. Evidence: `cache_size` is ~8 MB *per
      connection* and the pool is currently unlimited.
- [ ] PERF-06: ETag + `Cache-Control` on capabilities/config/rate cards/route
      catalogs (304 on repeat). Evidence: only two workbench routes use ETags
      today; `/api/capabilities` is 2,034 B recomputed per request.

### P1 — measured compute and console wins

- [ ] PERF-07: per-session SSE hub (one query per 2 s tick, fan-out to
      subscribers) and remove the duplicate `events`/`event` payload. Evidence:
      one query per client per tick today; each event is serialized twice.
- [ ] PERF-08: Aer method heuristic (`matrix_product_state` for shallow wide
      circuits, statevector otherwise), configurable thread cap, and a raised
      local cap only with correctness pins. Evidence: 26 qubits 3.48 s → 0.19 s
      (18.2×); 1 thread is 5.5× slower than default.
- [ ] PERF-09: bounded LRU for chemistry anchors keyed by
      `(molecule, r, basis)` and `(atom, basis, charge, spin, ncas, nelecas)`,
      with hit/miss metrics. Evidence: benzene 1.22–2.02 s per uncached call.
- [ ] PERF-10: configurable hardware `optimization_level` (default 2, record
      depth/2q before/after). Evidence: pinned at 1 in
      `qiskit_provider.py:397` and `bridge/experiments.py:1012,1061`.
- [ ] PERF-11: React Compiler on; Cache Components/PPR only for static shells;
      reduce the 102/214 client-component surface. Gate: build + bundle budgets
      + LCP/INP unchanged or better; no session-scoped caching.
- [ ] PERF-12: `scripts/perf-gate.sh` + CI job with benchmark deltas and a
      disposable-stack latency budget (fail on >20% regression in the named
      benchmarks).

### P2 — architectural

- [ ] PERF-13: run the existing Streamable HTTP MCP gateway
      (`kubona/run.py`, `mcp/http_gateway.py`) as a compose service with shared
      sessions; keep stdio for local. Evidence: research measures ~293–300 rps
      (HTTP shared) vs 0.64 rps (stdio, 20 concurrent).
- [ ] PERF-14: parallel dispatch for independent read-only tool calls, with the
      taint barrier, per-turn submit bound, and observation order preserved.
- [ ] PERF-15: `encoding/json/v2` only on measured decode paths (26% unmarshal
      win; marshal parity means no write-path migration).

## Memory management (2026-11-18)

Research, measurements, and the full argument:
`docs/memory-management-research-2026-11-18.md`; wiki
[[memory-management-2026-11-18]]. Companion to the Performance section above;
the shared slices are called out per item. Baseline (`5446b2a2`, this host):
kubonad 1 GiB / fresh RSS 27.6 MiB; bridge 4 GiB / 2 CPUs; console 1 GiB with
a 250 MB heap gate; 50k Hindsight memories = 97.3 MiB live heap and 169 ms
load; Aer peak RSS 1.1 GiB at 26q, 2.1 GiB at 27q, **4,172 MiB at 28q**;
PySCF benzene/6-31G* 224 MiB; `/metrics` exposes no memory gauges.

### P0 — make memory visible and bounded

- [ ] MEM-01: global budget (`MaxTotalItems`/`MaxTotalBytes`, startup trim,
      fair share) because bank caps are per tenant and `loadFromDB` loads all
      tenants. Closes: 50k items stay under budget with deterministic
      eviction; no cross-tenant eviction. (shares PERF-05)
- [ ] MEM-02: memory observability: Go heap/GC/RSS/cgroup via
      `runtime/metrics` + `/proc` + `memory.current`; store item/vector counts;
      bridge RSS in health; heap profile behind bearer. Closes: the 28q OOM
      risk and tenant growth are visible. (shares PERF-01)
- [ ] MEM-03: start the decay daemon with the server context and include
      `EffectiveScore`/recency in semantic ranking (or delete the dead config).
      Closes: stale experience ranks below fresh equivalent; daemon test.
- [ ] MEM-04: bridge budget derived from cgroup `memory.max`; set
      `max_memory_mb` on every `AerSimulator`; per-job cap 27q/2.1 GiB;
      in-flight ≤70% of budget; typed preflight refusal; check Aer result
      status. Closes: 28q refuses before allocation; 26q runs; concurrent jobs
      stay under 4 GiB. (shares PERF-08)
- [ ] MEM-05: Python hygiene: `MALLOC_ARENA_MAX=2`, OMP/BLAS threads = CPU
      limit, `malloc_trim(0)` after jobs, soft `RLIMIT_AS`. Closes: RSS flat
      across a 30-job soak; no OOM. (shares PERF-08)
- [ ] MEM-06: retention + vacuum: event/message/job/tombstone classes, reaper,
      `auto_vacuum=INCREMENTAL` + one-time `VACUUM`, backup temp-space budget.
      Closes: a synthetic 100k-event month stops growing; evidence referenced
      by claims/manuscripts is preserved. (shares PERF-05)
- [ ] MEM-07: automatic bounded pre-turn recall through the same tenant/IFC
      path. Closes: scripted continuity improves vs tool-only; ≤250 tokens;
      cross-tenant refusal.
- [ ] MEM-08: compaction/offload: summarize old turns at a threshold, offload
      large observations to a durable store with a pointer, keep the full
      transcript. Closes: 100-turn session peak tokens down ≥30%, key facts
      retrievable, reversible.
- [ ] MEM-09: bounded transcript read: SQL-side `LIMIT` + pagination instead
      of loading the whole session. Closes: 100k-message session reads newest
      N in <50 ms with bounded heap. (shares PERF-02)
- [ ] MEM-10: dedupe/merge: content hash + near-duplicate cosine, importance
      bump. Closes: 100 repeated turns → ≤2 memories; cap not exhausted;
      source ids retained.
- [ ] MEM-11: temporal validity (`valid_from`/`valid_to`/`superseded_by`).
      Closes: contradictory recall returns current only; "as of" returns
      history.
- [ ] MEM-12: int8 vector quantization + f32 rescoring (optional ANN). Closes:
      ≥4× vector memory cut; recall@10 ≥0.95 of f32 on a labeled set.
- [ ] MEM-13: purge retracted rows after retention; evict terminal loops; cap
      snapshots; A2A TTL; map-size gauge. Closes: 100k retractions do not grow
      heap; loop map bounded.
- [ ] MEM-14: memory gate: peak-RSS benchmarks (Hindsight 50k, Aer 24/26/28,
      PySCF benzene, console heap) with a 10% regression threshold. Closes:
      CI artifact and failure. (shares PERF-12)
- [ ] MEM-15: tiered storage: delete/quarantine or cap Tier 0/1 and replace
      the dense PPR with sparse iteration. Closes: n=5k PPR <100 MiB; module
      marked unserved if kept.

### P1 — quality and scale

- [ ] MEM-16: hybrid retrieval (semantic + lexical + recency) with a reranker,
      measured on a labeled recall set.
- [ ] MEM-17: importance calibration from outcomes (claim accepted/refuted,
      user feedback) instead of turn-shape heuristics.
- [ ] MEM-18: embedding single-flight cache keyed by content/query hash (also
      removes the HTTP call from under the store lock — F4).
- [ ] MEM-19: per-tenant byte accounting and fair-share eviction.

### P2 — architecture

- [ ] MEM-20: pgvector/HNSW sidecar only after the global cap is reached; keep
      SQLite as the local default.
- [ ] MEM-21: temporal graph projection if MEM-11 lands and graph queries
      justify it.
- [ ] MEM-22: tiered storage only if a served surface needs it; otherwise
      remove it from the blueprint.

## Operator-only

- [ ] Manual VoiceOver/NVDA walkthrough. (Needs assistive-technology hardware.)
- [ ] Revoke/confirm the `gho_…` token. (External GitHub credential.)
- [x] Confirm reviewer key on the deployment host: `KUBONA_REVIEWER_PRIVKEY_HEX`
      is present in the running `kubonad` container
      (`reviewer_key_present`), so manuscript signoff refuses only for other
      reasons.
- [ ] Attach paid-QPU instance/run id/invoice or mark unpaid. (External vendor
      evidence; not available in this repo.)
- [ ] Record runtime rate card before 2026-11-29. (Operator pricing decision.)
