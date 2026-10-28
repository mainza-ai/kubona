---
type: concept
name: Production Readiness Audit — Round 2 Details (P2/P3 + coverage)
date: 2026-10-26
updated: 2026-10-26
tags: [audit, production, details, p2, p3, coverage]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# Production Readiness Audit — Round 2 Details

Companion to [[production-readiness-audit-2026-10-26]]. This page holds the P2/P3
register, the exact file coverage, and the verification log so the main report
stays readable. Severity: **P2** wrong-but-contained / incomplete; **P3** polish,
observability, or documentation. None of these block the P0 gate, but each is a
real rework cost if left.

## Coverage

| Area | Files | LOC | Notes |
|---|---:|---:|---|
| Go orchestrator (`platform/core`) | 245 | 66,747 | 139 test files; generated pb struct-checked |
| Python runtime (`platform/kubona`) | 224 | 41,996 | 171 external test files |
| Bridge + proto | 19 (+2 proto) | 6,265 | generated Python/Go bindings spot-checked |
| Rust crates | 8 | 901 | qir-core 14 tests, ifc-core 9 |
| Console (`src`) | 135 | 28,079 | 56 test files / 272 tests |
| Tests (`platform/tests`) | 172 | 20,537 | 1,053 collected, 25 skip clauses, 0 xfail |
| Contracts + domain packs + docker + scripts + CI | 43 | 8,677 | 7 contracts, 3 packs, 9 Docker files, 24 scripts |

Out of scope by rule: `dev-docs/` (gitignored scratch), `node_modules/`,
`.next/`, `platform/.venv/`, `target/`, binaries/images, and generated binding
bodies beyond structural verification.

## Verification log

| Check | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./...` | **fail**: `pkg/llm/env_defaults_test.go:196` copies a lock value |
| `go test ./...` | pass (api 54 s; 29 packages) |
| `go test -race` ledger/session/tenants | pass |
| `pytest` fast slices | **3 failed**: `test_mcp_catalog`, `test_mcp_surface`, `test_codesign_audit` |
| `vitest --run` | 56 files / 272 tests pass |
| `tsc --noEmit` | pass |
| `eslint` | **fail**: 2 errors + 1 warning (`FlightRecorderCard.tsx:37,53`) |
| `next build` | pass; first-load chunk **1.4 MB**, contains the dev bearer |
| `cargo test` + `clippy` | 23 pass; clippy clean |
| `wiki-lint --strict` | clean (151 pages before this audit) |
| GitHub API | branch protection **off**; repo **public**; 9 Dependabot alerts; run `40297011884` red |
| Repros | sandbox escape (UID + `/etc/hosts`), H2 identical at R=0.7414/2.0, token in `.next` chunk |

---

## Go API P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| GO-16 | P2 | Request bodies unbounded on almost every write route (only 4 `MaxBytesReader`s); asset base64 decoded before cap | `server.go:2025,1681,2917,4876`; `observatory_assets.go:78` |
| GO-17 | P2 | Disk guard missing on lens/quantum/workflow/memory/import spend rails | `observatory_lens.go:252`; `server.go:218-250`; `workflows.go:143` |
| GO-18 | P2 | Cross-tenant quantum job status/cancel if bridge omits `session_id` | `quantum_jobs.go:301-307,327-337` |
| GO-19 | P2 | Abort not sealed as an abort; ghost abort 500s; suspension does not stop loops/queued work | `provenance.go:130-146`; `tenants.go:292-298` |
| GO-20 | P2 | User/Atlas operation IDs interpolate into URLs and filesystem paths (SSRF/path traversal) | `observatory_lens.go:103`; `observatory_assets.go:53`; `observatory_webhooks.go:272` |
| GO-21 | P2 | Public mandate verify mutates ledger; public PQC verify expensive and unthrottled | `server.go:452-456,1183-1216,549` |
| GO-22 | P2 | Client-controlled `role` stored and fed back to the model (`system`/`assistant` injection) | `server.go:2057-2061,2184-2189` |
| GO-23 | P2 | No FK enforcement, swallowed migrations, no schema version/backup; session delete erases ledger | `ledger.go:92-100,250-282`; `session.go:444-451` |
| GO-24 | P2 | Swallowed ledger/campaign write errors after success responses | `server.go:1583,2083,4337,4409` |
| GO-25 | P2 | JWKS fetch holds a global mutex over network I/O; output downloads lack URL policy | `observatory_webhooks.go:50-56,316-319` |
| GO-26 | P2 | Missing `Vary: Origin`; wildcard ACAO with credentials; per-request warning log | `server.go:399-408`; `edge_auth.go:61-64` |
| GO-27 | P2 | Disk-only readiness; no metrics/tracing/request IDs; no panic recovery in A2A goroutines | `server.go:713-718`; `a2a.go:486` |
| GO-28 | P2 | Mandates accept empty `job_hash` and unbounded TTL; `Verify` skips empty expectations | `mandates/mandate.go:89-95`; `server.go:1059-1061` |
| GO-29 | P2 | Tests pin weaker claims: 2 s vs 200 ms; self-signed A2A success; no forged refund/import/cross-lab negatives | `flight_recorder_test.go:38`; `server_a2a_test.go:94-107`; `spend_authority_test.go:20-23` |
| GO-30 | P3 | Public tenant list exposes admin pubkeys/ceilings/spend; public agents expose system prompts | `server.go:601,5248-5251`; `tenants.go:54-62` |
| GO-31 | P3 | Grant revoke scans all rows; `/api/radar` is an internal port scanner; raw HF repo ID in URL | `grants.go:155-181`; `server.go:990-1034`; `hf/hub.go:290` |

## Go domain packages P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| GOD-12 | P2 | Autoresearch `Step` not serialized; baseline holds the write lock over sandbox I/O | `autoresearch/engine.go:302-312,398-432` |
| GOD-13 | P2 | Loop spend claim read-then-zero TOCTOU | `ledger/loop_spend.go:67-78` |
| GOD-14 | P2 | Message rewrite CAS is TOCTOU (no content predicate) | `session/session.go:342-360` |
| GOD-15 | P2 | Float money + epsilon over-refund dust | `ledger.go:97-98`; `tenants.go:269,404-447` |
| GOD-16 | P2 | Import can reorder chains on ID collision and never verifies the result | `session/migrate.go:539-548` |
| GOD-17 | P2 | gVisor not implemented; warm container runs unhardened (default network/root/no cap-drop) | `sandbox/docker.go:50,146-148,226-228` |
| GOD-18 | P2 | Sandbox timeout misclassified as a normal exit | `sandbox/docker.go:248-264` |
| GOD-19 | P2 | Epistemic assertion accepts duplicate/pending-review sources and caller-supplied delta | `epistemics/wavefunction.go:123-131`; `source_labels.go:31,76-80` |
| GOD-20 | P2 | Memory eviction is global across tenants; persistence errors ignored | `memory/hindsight.go:219-275` |
| GOD-21 | P2 | Grant budget accepts NaN; revocation read errors fail open | `grants/grants.go:286-288,412,486-513` |
| GOD-22 | P2 | QIR fallback claims `BaseProfile-Estimate` but emits `qir_profiles=custom`/`inttoptr` | `qir/compiler.go:203,520-545` |
| GOD-23 | P2 | Tenant mutators do not bind action/params internally (API does) | `tenants/tenants.go:191-193,227-239` |
| GOD-24 | P2 | Fidelity gate undercounts unknown 2-qubit gates on non-`q` registers | `jobs/admission.go:298-333` |
| GOD-25 | P2 | Fidelity contract load panics in a library constructor if JSON absent | `fidelity/accounting.go:73-76` |
| GOD-26 | P2 | Metasearch returns internal pointers; "human approved" is an unchecked flag | `metasearch/evolution.go:211-296` |
| GOD-27 | P3 | Randomness errors ignored; OpenBao may use plaintext `http://` off-loopback | `vault/vault.go:128`; `vault/client.go:45-48` |
| GOD-28 | P3 | Test weaknesses normalize the risks (unregistered label type, HF fallback accepted, unsigned receipts, self-signed A2A) | `ledger_test.go:157`; `hf/hub_test.go:20,45`; `jobs_test.go:333`; `a2a_test.go:213` |
| GOD-29 | P3 | Neutral-atom QEC exponent disagrees with the stated scaling law | `qec/estimator.go:114,160` |
| GOD-30 | P3 | Codesign TQ multiply can overflow int64; frontier lacks a mutex | `codesign/tq.go:26-30`; `frontier.go:12-29` |

## Python core/MCP P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| PYC-17 | P2 | Tests mutate real `~/.kubona`; no conftest isolation | `platform/tests/`; `jobs.py:95`; `session.py:66` |
| PYC-18 | P2 | Tests codify the money bug and trust-the-caller isolation; hardcoded 36 | `test_execution_discipline.py:23-30`; `test_control_tenants.py:19-27`; `test_mcp_surface.py:20` |
| PYC-19 | P2 | Cancellation never calls vendor recall; `request_provider_recall` unmounted and has a latent `NameError` | `execution.py:537-584`; `_mount.py:40-50` |
| PYC-20 | P2 | Unbounded ledger/session reads and science matrix/qubit sizes | `provenance.py:41-85`; `execution.py:615-618`; `console_api.py:407-437` |
| PYC-21 | P2 | Unknown fidelity device inherits the perfect simulator; qubit count unchecked | `fidelity/accounting.py:172-176` |
| PYC-22 | **Closed** | Loop/steering/delegation state now persists (loops.jsonl, delegations.json); restart recovery is paused/fail-closed; forks inherit parent tenant. | tests: `test_control.py`, `test_delegation_crypto.py` |
| PYC-23 | P2 | Non-atomic JSON stores; lost-update in `record_job`; corrupt-line crashes | `labs.py:58`; `identity/registry.py:68`; `session.py:182-186` |
| PYC-24 | P2 | Ledger pre-label rows readable by ambient; reads without flock; no fsync | `provenance/ledger.py:185-236` |
| PYC-25 | P2 | Canary snapshot hash computed before the calibration field is attached | `canary/canary.py:140-150` |
| PYC-26 | P2 | Stale provider SDK assumptions (IBM `ibm_quantum`, Heron mismatch, QCS stub, Leap sampler), no retry/backoff | `qiskit_provider.py:167,209,256`; `pyquil_provider.py:133-140`; `annealing_provider.py:143-148` |
| PYC-27 | **Closed** | A2A dispatch now enforces a named custom agent's declared scopes and spend envelope (`AuthorizeTask`); unknown/out-of-envelope requests refuse before queueing. The Go registry is the runtime authority and shares the `custom_agents` table with the Python manifest view. | `agents/custom.go`; `a2a.go`; `a2a_test.go` |
| PYC-28 | P2 | Console science endpoints can be DoSed; tribunal "COMMITTED_TO_LEDGER" writes nothing | `console_api.py:407-421,435-437,676,693-696` |
| PYC-29 | P2 | Audit only successful calls; internal error text leaked; approval nonces per process | `gateway.py:135-151`; `http_gateway.py:122-126`; `approval.py:172-193` |
| PYC-30 | P2 | `SecretStore.get(required=True)` returns `None`; corrupt stores crash rather than quarantine | `secrets.py:191-195`; `governance.py:80-83`; `identity/registry.py:64` |
| PYC-31 | P2 | Local-model discovery blocks 11 ports sequentially; 401/404 counts as available | `llm/registry.py:29-99`; `ollama.py:24-30`; `openai_compat.py:27-33` |
| PYC-32 | P3 | O(N) JSONL reads/polls; no schema version; semaphore leak on submit failure | `session.py:120-134`; `jobs.py:113-130,245-301` |
| PYC-33 | P3 | HF/arXiv input hygiene: unsanitized repo ID/details URL, unbounded cache, `ElementTree` | `hf_hub.py:243-250,398`; `sources.py:44-93` |
| PYC-34 | P3 | Float money; `sync_external_session` accepts caller-supplied spend without authority check | `session.py:192-222,230-282` |
| PYC-35 | P3 | Two incompatible `SpendMandate` classes in one package | `cost/mandate.py:44-57`; `governance/delegation.py:52-62` |

## Python science P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| PYM-21 | P2 | Load-bearing exactness invariants are `assert` (stripped under `-O`) | `codesign/divwalk.py:115-184`; `packing.py:207`; `eea12.py:352` |
| PYM-22 | P2 | `MetaLoop.shadow_compare` wrong for negative scores | `autoresearch/meta.py:63-66` |
| PYM-23 | P2 | `reflect_on_orchestrator` attributes a temporal split to a radius change | `autoresearch/meta.py:147-170` |
| PYM-24 | P2 | Orchestrator runs refinements after watchdog ABORT/PAUSE | `autoresearch/base.py:102-166` |
| PYM-25 | P2 | GQE harness not wired; generic `propose` fails on string lists | `autoresearch/base.py:76-88`; `gqe.py:43-56` |
| PYM-26 | P2 | `check_critic` passes vacuously when no mutant applies | `eval/check_critic.py:104-105` |
| PYM-27 | P2 | Materials pack generator missing; revocation fails open on malformed file | `science/domain_pack.py:153,222-229` |
| PYM-28 | P2 | Patent quota charged after the network call | `intelligence/patents.py:133` |
| PYM-29 | P2 | Workflow status can stay `running` on ledger failure; no atomic status/idempotency | `workflows/engine.py:390-420` |
| PYM-30 | P2 | Library "validated" accepts any non-empty proof; save non-atomic | `library/store.py:127-130,186-187` |
| PYM-31 | P2 | Corpus loading overwrites built-in eval tasks without a collision check | `eval/corpus_loader.py:167` |
| PYM-32 | P2 | Closed-form anchors have contradictions/edge bugs (parity reliability, 2D W=0, spin-independent E0) | `anchors/math_physics.py:353-360,521-523,743` |
| PYM-33 | P2 | Curvature fallback is a hand-designed metric, not Fisher–Rao | `epistemics/curvature.py:149-210` |
| PYM-34 | P2 | Continuum content hash includes the signature (hash-then-sign violated) | `epistemics/continuum.py:79-89` |
| PYM-35 | P2 | `safe_288` guarantee is per-trace, not the 620-walk claim | `codesign/packing.py:83-94`; `test_codesign_slice18.py:44-60` |
| PYM-36 | P2 | Parser skips `reset` and aliases unknown registers to raw indices | `circuits/parser.py:26-28,77-93,128-141` |
| PYM-37 | P2 | Heisenberg anchor convention unstated and likely off by 2× | `anchors/math_physics.py:49-54`; `test_math_anchors.py:31-37` |
| PYM-38 | P3 | `plan_dual_model_packing` missing from `codesign.__all__` (red test) | `tests/test_codesign_audit.py:266` |
| PYM-39 | P3 | Non-atomic writes in reports/library stores | `reports/notebook.py:89-91`; `library/store.py:186` |
| PYM-40 | P3 | Doc/implementation drift (symbol counts, index formula, shadowed `k`) | `hermitian.py:38-42,429`; `rs12.py:143,156`; `corpora.py:37-40` |
| PYM-41 | P3 | ZNE gross clipping returns ±1 with a flag instead of refusing | `mitigation/zne.py:273-284` |
| PYM-42 | P3 | `can_greenlight` accepts MW outside `[0,1]` | `circuits/ansatz_optimizer.py:248-258` |
| PYM-43 | P3 | Triage accepts negative orbitals/gap/radical | `science/triage.py:28-45` |
| PYM-44 | P3 | Patent HTTP client never closed | `intelligence/patents.py:120` |
| PYM-45 | P3 | QKSD all-singular fallback keeps a zero mode | `circuits/krylov.py:216-221` |

## Bridge/proto/Rust P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| BR-21 | P2 | No root `.dockerignore`; Lean build sends `.env`/`dev-docs` (21 G) to the builder | `Dockerfile.lean:13` |
| BR-22 | P2 | Materials pack points at a nonexistent module | `domain_packs/materials_condensed_matter_v1/pack.yaml:10` |
| BR-23 | P2 | Provider names used as filesystem paths without validation | `schema/provider.py:242-247` |
| BR-24 | P2 | Model manifest declares absent LoRA weights/hash/metrics | `platform/models/adapters/manifest.json:34-43` |
| BR-25 | P2 | Domain-pack signatures optional; self-asserted owner; one bad pack blocks all | `science/domain_pack.py:90-102,191-262` |
| BR-26 | P2 | `register_default_packs()` overwrites tracked manifests and drops benchmarks | `science/domain_pack.py:107-188` |
| BR-27 | P2 | Literature URL injection, `None` summary crash, unbounded cache | `bridge/literature.py:17,57,72,141` |
| BR-28 | P2 | Proto drops `method`/`measured`/`fetched_live`/license/QPD labels | `proto/kubona.proto:85-186` |
| BR-29 | P2 | `mandate_ref_json` not recorded; status estimate always empty | `bridge/server.py:277-288,333-338` |
| BR-30 | P2 | Workflow cost accepts NaN/Inf (`json.loads` default) | `workflows/engine.py:236-256`; `bridge/server.py:585` |
| BR-31 | P2 | Subprocess inherits full env; Go QIR binary path unrestricted | `kubona/qir/native.py:88-90`; `core/pkg/qir/compiler.go:39-49` |
| BR-32 | P2 | Tag-only sidecars, no Hindsight data volume, DB/Vault ports published | `docker-compose.hindsight.yml:5,14`; `docker-compose.vault.yml:13-16` |
| BR-33 | P2 | Default/minimal RPC requests fail (empty backend, `range(0,n,0)`) | `bridge/executor.py:461-482,639` |
| BR-34 | P2 | Molecule label uppercased changes raw SMILES (`c1ccccc1`→cyclohexane) | `bridge/chemistry.py:68,240` |
| BR-35 | P2 | `EvaluateCodesign` accepts unbounded truth tables/artifacts | `bridge/codesign_ops.py:37-58` |
| BR-36 | P2 | "3-seam" IFC is three drifting implementations; Rust uncalled, Python lacks universe | `core/pkg/ifc/lattice.go:22-38`; `provenance/ifc.py:34-52`; `crates/ifc-core/src/lib.rs:7-31` |
| BR-37 | P2 | `probe_clifford_fidelity` is a deterministic formula; "circular n=1" emits `cx q[0],q[0]` | `bridge/fidelity.py:146-161`; `ansatz_search.py:167-207` |
| BR-38 | P3 | Proto regen has no version/diff check; root mirror can be overwritten | `scripts/regen-proto.sh:6-38` |
| BR-39 | P3 | Missing crate license, dead proto messages, misleading `fci_exact_energy` name | `crates/qir-core/Cargo.toml`; `proto/kubona.proto:7-15,90,356-367` |
| BR-40 | P3 | ZNE divide-by-zero on empty circuits; `b[-1]` assumes one register | `bridge/mitigation.py:52-86` |
| BR-41 | P3 | QIR instruction count omits final measurements; float formatting may not assemble | `crates/qir-core/src/codegen.rs:62-64,165` |
| BR-42 | P3 | No server-side deadlines; PySCF/Qiskit can pin all 8 workers | `bridge/server.py:629`; `bridge/executor.py:494`; `bridge/chemistry.py:176` |

## Console P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| UI-15 | P2 | Polling effect restarts on every run-phase transition; duplicate fetches/races | `page.tsx:127-214,316-321` |
| UI-16 | P2 | No fetch timeout/cancellation; stale responses overwrite newer state | `api.ts:68-70`; `page.tsx:100-104`; `CopilotDrawer.tsx:127-135` |
| UI-17 | P2 | No React error boundaries; one surface throw blanks the shell | `app/` (no `error.tsx`) |
| UI-18 | P2 | 3D canvas fails silently, pointer-only, no context-loss/visibility pause, hover rerenders | `SpatialCanvas3D.tsx:613-627,973-1031,1090` |
| UI-19 | P2 | 1-second shell tick re-renders 3D/copilot | `page.tsx:205-208,425` |
| UI-20 | P2 | Unknown-size HF download bypasses the multi-GB confirmation | `HuggingFaceModelHub.tsx:144-150`; `api.ts:1053-1056` |
| UI-21 | P2 | HF search failure becomes "No models found" after the toast clears | `HuggingFaceModelHub.tsx:56-71,415-419` |
| UI-22 | P2 | "Key stored" inferred from provider enabled; API key may enter React state | `LLMSettings.tsx:28,71-86,406` |
| UI-23 | P2 | Fleet Radar copy claims real-time 1 s telemetry while serving catalog/15 s backoff | `FleetRadar.tsx:106,175,308-314` |
| UI-24 | P2 | Autonomy Cockpit hardcodes crypto/storage/sentinel posture | `AutonomyCockpit.tsx:177-182,319-329` |
| UI-25 | P2 | Review annotations ephemeral; "addressed" only means a prompt was sent | `ResultsStudio.tsx:48,203-238` |
| UI-26 | P2 | Admin-envelope canonicalization diverges from Go for `<>&` and float exponents | `adminAction.ts:29-39`; `tenants.go:85-90` |
| UI-27 | P2 | Modals lack focus traps/initial focus/restore | `AboutLabModal.tsx:12-25`; `NewCampaignModal.tsx:92-111`; `CommandPalette.tsx:178-214` |
| UI-28 | P2 | "Approve once" and "approve up to max" are identical; expiry decorative | `ActionGate.tsx:190-191`; `ApprovalPacketCard.tsx:129`; `approvalPacket.ts:89,112` |
| UI-29 | P2 | Tribunal proposal card rendered twice; duplicate turns possible | `CopilotDrawer.tsx:333-355,1128,1322` |
| UI-30 | P2 | Persisted ambient mute ignored on unlock | `ambient-audio.ts:20-46,58-60` |
| UI-31 | P2 | API URL discovery one-shot; a transient failure pins localhost forever | `api.ts:14-29` |
| UI-32 | P2 | Tests miss page orchestration, streaming success, quote staleness, session isolation, a11y | `vitest.config.ts:5`; `page.test` absent |
| UI-33 | P3 | Flight Recorder starts at milestone 4 | `FlightRecorder.tsx:31` |
| UI-34 | P3 | Job poll continues forever after transport failure | `QuantumDispatch.tsx:80-84` |
| UI-35 | P3 | 16 MB unused public assets | `public/assets/` |
| UI-36 | P3 | `?` shortcut fires in `SELECT`/contenteditable | `page.tsx:405` |
| UI-37 | P3 | Tooltip semantics inaccessible (`role=tooltip` always present) | `Tip.tsx:28-34` |
| UI-38 | P3 | Global `select-none` blocks copying results/hashes | `page.tsx:446`; `CopilotDrawer.tsx:789` |
| UI-39 | P3 | Unhandled clipboard rejections | `CopilotDrawer.tsx:405-409` |
| UI-40 | P3 | Side effects inside a state updater cause double switches/fetches | `page.tsx:254-269` |
| UI-41 | P3 | Markdown/KaTeX reparsed for every streamed token (quadratic) | `CopilotDrawer.tsx:244-247`; `MathText.tsx:202-219` |
| UI-42 | P3 | About page states OpenBao/Buzz as attested posture | `AboutLabModal.tsx:113-118` |

## CI/contracts/hygiene P2/P3

| ID | Sev | Finding | Evidence |
|---|---|---|---|
| CT-01 | P2 | Python raises 1 of 25 catalogued error codes | `provenance/ledger.py:148`; `contracts/error-catalog.csv` |
| CT-02 | P2 | Event payload `required` fields are not validated | `provenance/ledger.py:145`; `ledger/events.go:120` |
| CT-03 | P2 | `agent-spec-v1.json` is unused and permissive | `contracts/agent-spec-v1.json` |
| TS-04 | P2 | Shared `KUBONA_STATE_DIR` via `setdefault` creates import-order coupling | `test_console_api.py:10-15`; `test_codesign_api.py:6` |
| TS-05 | P2 | Wall-clock/sleep assertions are flaky | `test_perf_guard.py:30,40`; `test_security_hardening.py:27,106` |
| TS-06 | P2 | Two logging tests `assert True` | `test_logging.py:29,52` |
| SC-03 | P2 | `wiki-lint` misses list-form sources, uses substring membership, crashes on missing frontmatter | `scripts/wiki-lint.py:71,80,107-110` |
| SC-04 | P2 | Secret scan is prefix/tracked-only; misses `github_pat_`, `sk-`, JWTs, history | `scripts/secret-scan.sh:30` |
| SC-05 | P2 | `regen-proto.sh` uses a predictable temp dir and does not assert versions/diff | `scripts/regen-proto.sh:30` |
| CI-08 | P2 | Shards F/G are 120-minute single-test jobs; cancelled under preemption | `ci.yml:219,237` |
| CI-09 | P2 | No Python lint/typecheck/coverage/SAST; no Go/Rust lint | `pyproject.toml:56-60` |
| CI-10 | P2 | Single-version matrix (3.11/Node 20) while 3.12 deps exist | `pyproject.toml:14-15`; `ci.yml:82,383` |
| HY-01 | P2 | Declared pnpm 11 vs CI/Docker pnpm 9 on Node 20 | `package.json:40`; `ci.yml:380`; `Dockerfile.console:23` |
| HY-02 | P2 | Python dependencies pinned but not locked/hashed | `pyproject.toml:9-28` |
| SC-06 | P2 | Nightly "hardware" canary is simulator-only and not `--gate` | `nightly-canary.yml:27`; `canary.py:333` |
| SC-07 | P2 | Audit helper scripts are stale/unwired/crash (89 pages, missing input, hardcoded paths) | `verify_wiki_comprehensive_audit.py:4`; `build_master_audit_artifact.py:3`; `sc09_ledger_audit.py:22` |
| TS-07 | P2 | `bench/run.py` not in CI; benchmark baseline says 29 tools (actual 37) | `bench/run.py`; `bench/BENCHMARK.md` |
| HY-03 | P3 | Workspace `.env`/`.env.local` are 0644 | `.env:4-18`; `console/.env.local:1` |
| HY-04 | P3 | Documentation state stale/conflicting (129 pages vs 151; private vs public) | `AGENTS.md:18,163-165` |
| HY-05 | P3 | No CODE_OF_CONDUCT, issue/PR templates | `.github/` |
| TS-08 | P3 | `platform/test_live_e2e.py` has zero pytest tests | `test_live_e2e.py:16` |
| TS-09 | P3 | Duplicated test bodies | `test_routing_enforcement.py:28`; `test_fidelity_admission.py:14` |

---

## Positive findings to preserve (consolidated, beyond the main list)

- `codesign/gf.py` self-verifying irreducible/primitive field construction.
- `kickmix.py` exhaustive trajectory evaluator with fail-closed phase/ancilla
  contract; `group72.py` exact group law/inverse/trace; tower/f24 basis changes
  with exhaustive inverse checks.
- `mitigation/backend.py` exactly trace-preserving depolarizing channel; true
  unitary folding with exact inverses.
- `cutting/wire.py` correct Z-wire LOCC domain and refusal of spanning gates.
- `eval/comparator_eval.py`/`codeexec.py` container flags and
  infra-vs-test failure distinction (the *live* path is real).
- `eval/truncation.py` correct H2 Jordan-Wigner anchor (`-1.137284`) — the
  reference the table anchor should adopt.
- `autoresearch/population.py` exact sample accounting and equal-budget arms.
- Go: atomic spend CAS, cumulative ceilings, content-bound idempotency, disk
  guard, tool registry deny-by-default, PQC envelope, cross-language vectors,
  bounded LLM I/O, provenance tagging of tool observations.
- Python: OAuth alg/iss/aud/nbf/exp checks, HTTP body cap, secure RPC AAD,
  approval HMAC design, HF download sanitization/SHA pin/atomic replace,
  canary all-skip refusal.
- Console: canonicalization test vectors, `authorizeSpend` fail-closed gate,
  three-state backend status, XSS fixtures, WebGL disposal, session-key reset
  pattern used in several components, job-status enum drift test.
- CI: least-privilege `permissions`, SHA-pinned actions, nonroot/pinned images,
  no xfail, honest skip reasons.

## Note on the prior audit

Round 1 ([[full-system-audit-2026-10-26]]) fixed real live defects (F-R1-01..16,
F-001, F-006) and its remediation plan remains valid. This round does not
reopen those; it adds the code-level boundary findings the live battery did not
reach (bridge auth, import forgery, A2A trust, sandbox escape, anchor error,
comparator fallback, claim theater). Where both touch the same code
(session reads, Python exec), the Round-1 plan is the canonical fix path and
this page's evidence is additive.
