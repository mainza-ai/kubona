---
type: concept
date: 2026-08-16
updated: 2026-09-29
tags: [kubona, production, gaps]
sources: [source-gap-analysis-plan]
status: tracked
---

# Prototype Gap Register (production vs. scaffold)

> **What this page is.** The honest register of what the [[overview|prototype]] does vs.
> what production requires. It is a *code* register, not a design register: it answers
> "what does the scaffold already do", never "what should Kubona be". Design questions
> live in [[questions]]. This page exists so a skeleton/engine implementation is never
> mistaken for a deployable one, and so each gap is closed with evidence, not intent.

## Status legend
- 🟢 **Closed (M0)** — structural, cheap-now fixes landed.
- 🟠 **Open + required before real deployment** — the "we'd redo it later" items.
- 🔵 **Deferred by design** — matches the plan's own "functional product first" sequencing.
- 🔴 **Regressed / failing now** — verified broken at the stated date; not a future concern.

## Critical (structural)

| Gap | Status | What's real / what's missing |
|---|---|---|
| **Persistent state root (data-loss)** | 🟢 M0 | Was split: 8 stores hardcoded to volatile `/tmp/kubona-state` (settings, identity, agents, the algorithm library, reports, labs, workflows, meta-proposals) while sessions/gov/ledger used `KUBONA_STATE_DIR`. Now all route through `kubona/config.py` (`state_root()`) — default `~/.kubona`, epoch-safe. |
| **Prod auth default** | 🟢 M0 | Gateway + console defaulted to `dev-token`. Now: OAuth2 signed tokens get `aud`/`nbf`, console uses constant-time compare against `KUBONA_CONSOLE_TOKEN`, and `KUBONA_ENV=production` **fails closed** (no token → no access). Production gateway factory in `kubona/run.py`. |
| **Deployment entrypoint** | 🟢 M0 | No `[project.scripts]`, no way to boot the system. Now: `kubona serve` / `kubona doctor` (package entrypoint + `run.py`), starlette/uvicorn are hard deps. |
| **CI / release-gate** | 🟢 green on hosted runner 2026-09-29 | `ci.yml` + `nightly-canary.yml` authored; `platform/scripts/ci.sh` reproduces the pipeline locally (compile, Rust build+`--lib`, pytest, canary, release gate, doctor/fail-closed). Test counts are environment-specific and must be quoted as such: **159 passed locally, 156 passed + 3 skipped on the runner** — the 3 are the optional-SDK tests (pytket, pyquil, langgraph: installed on the dev machine, absent on a clean runner), not a coverage loss. Both CI paths now run `pytest -q -rs`, so the runner names its own skips rather than leaving a bare count to be argued about — the same rule `release_gate()` applies to providers applies to tests — confirmed on run 36511131388, whose log prints exactly those three reasons. **The hosted push pipeline had never been green — all 17 recorded `ci.yml` runs failed, HEAD (`6970127`) included — fixed in `3e24487`, with the CI trigger gap closed by `2675816`, and verified green on Actions run 36508507064.** Root cause, correcting the diagnosis first written here: `run_all()` *did* classify fail-closed providers correctly (`ok=False, skip=True`); what was broken is that the gate's **rule existed twice** — `ci.sh` filtered skips before asserting, `ci.yml:68` asserted `all(r['ok'] for r in results)` across all 8 plugin-bearing providers, so 5 legitimate skips (no credentials, no local `qvm`, optional SDK absent, annealing/AHS/CV program model) failed a build whose canary log said `3/3 passed`. Fix removes the duplication rather than patching one copy: `release_gate()` in `kubona/canary/canary.py` is the single verdict, exposed as `python -m kubona.canary --gate` and called by both the hosted job and `ci.sh`. An all-skip run is deliberately a **failure** (a gate that executed nothing has proven nothing), and 5 new tests pin the semantics — including a negative test proving an injected calibration-drift regression still exits 1, so the fix cannot have softened the gate. `nightly-canary.yml` was always green: the mechanism worked, only the arithmetic around it was wrong. History confirms the duplication: the earlier attempt `17dc27f` ("Canary regression/skip classification fix") patched the `ci.sh` copy and left `ci.yml` untouched, and failed at this identical step. **Second finding, surfaced by pushing the fix:** `on.push.branches` was `[main]`, and main is advanced by fast-forwarding develop onto it — so all 17 failures were pushes on `main` and `develop` had *zero* runs; no commit was ever gated as it landed, which turns one bad commit into 17 red runs at the next advance. `develop` + `workflow_dispatch` are triggers now. Verified on run 36508507064: step *Correctness-regression harness (release gate)* → `release gate: OK (3 executable providers; 5 skipped)`, and the newly added *Wiki structural lint* step passes in CI's pristine checkout (no `dev-docs/` present, as expected for a text-only check). |
| **x402 fail-closed** | 🟢 M0 | The stub verifier accepted any `x402:`-prefixed proof (free-spend hole if enabled). Now `X402Payments` rejects every proof unless a real `verifier` is injected; metering/payment + P1 gate updated to assert fail-closed. |
| **Async, persistent job model** | 🟢 M1 | `kubona/jobs.py`: persistent JSONL `JobStore` (status survives restart) + background `JobRunner`; `submit_job(wait=True/False)`, `cancel_job` (P2 watch-and-interrupt), scoped `execution:override`, provenance on every transition, 4 dedicated tests. |
| **Real provider SDK execution** | 🟢 SDK-validated | IBM: real Qiskit Runtime. **Quantinuum (pytket) + Rigetti (pyquil) conversions now validated against the REAL SDKs** (installed in dev; CI-skip tests) — the real SDKs caught bugs the stubs missed (pyquil CX→CNOT, integer qubits; fixed). D-Wave: credential-free QUBO solver. Rigetti credential-free run needs the local `qvm` daemon (fails closed with guidance); QPU needs creds. Pulser/StrawberryFields remain modality-gated (gate→AHS/CV not auto-derivable; per plan §2.1), fail-closed. Live QPU runs need credentials (opted-in). |
| **Rust QIR/MLIR core on the call path** | 🟢 M1 | `kubona/qir/native.py` runs the built `qir-core` binary on `lower_circuit` (`engine: rust-qir-core`) with Python fallback; CI builds it and wires `KUBONA_QIR_CORE`; native-path test added. OpenQASM 3 + MLIR/QIR-bitcode remain. |
| **Streamable-HTTP + ML-KEM on the wire** | 🟢 M1 | `kubona/mcp/http_gateway.py` serves `/rpc` over a real uvicorn socket with OAuth2 bearer + a real two-party X25519+ML-KEM hybrid envelope (unwrapped, dispatched, response re-encrypted; tamper-rejected). `kubona serve-gateway` boots it; `kubona/secrets.py` adds env-backed (KMS-ready) per-provider credentials that fail closed. TLS/KEMTLS termination at the edge remains for deployment. |

## Medium

| Gap | Status | Note |
|---|---|---|
| Structured observability/metrics/audit export | 🟢 partial | Gateway audit now persists to a JSONL export (restart-safe, `audit_export()`, wired by default in `kubona run`/`serve-gateway`). Structured logging + OTel/Prometheus metrics remain. |
| Perf-regression guards (G12) | 🟢 | tests/test_perf_guard.py guards the lower + local-simulator hot paths against accidental O(exponential) regressions (passed in ~0.1s). |
| Volume eval corpora (QuantumKatas/HumanEval/PennyLang) | 🟡 mechanism-ready | `kubona/eval/corpus_loader.py` loads declarative JSON corpora (ghz/uniform/grover/bell check-kinds) into the evaluate_codegen contract; ships a verified extended corpus (ghz5/ghz6/qft4) whose every task passes its invariant on the simulator, and rejects bad entries fail-closed. The third-party datasets (QuantumKatas/HumanEval/PennyLang) are the external ingredient that drop in via this loader. |
| GQE transformer (Unsloth) training | 🔵 | n-gram stand-in today; drop-in `propose` replacement on a GPU host. |
| Full circuit knitting | 🟡 partial | LOOC-Z single-CNOT wire cut now additionally reconstructs the **full post-cut target distribution** (`cut_histogram`, exact Z-diagonal, verified against uncut — strictly more informative than a single ⟨Z⟩). Honest scope: a single-CNOT cut reaches one target qubit, so multi-qubit-gate/product-Z cuts still need the full QPD/Qiskit-CKT integration (documented; that is the remaining piece). |
| Perf/load baselines | 🔵 | Pure-numpy simulator; add perf regression + memory guards. |

## Deferred by design (correct, per plan)
- x402 activation on a real rail (Phase 5+), meta-loop auto-apply (§13.5, Phase 5+), Unsloth personas (Phase 6), real third-party provider PR, third-party security review execution, live arXiv/OpenAlex default (network-acknowledged; mock is the offline default).

## Blueprint integrity (the wiki is the deliverable, so it is linted)

| Gap | Severity | Where |
|---|---|---|
| The blueprint accumulated **build language** for months — phase gates written as "implemented", `status` values encoding finish state. The failure mode where a blueprint quietly becomes a changelog | high (fixed 2026-09-27) | reframed in [[overview]] / [[roadmap]] / [[index]]; vocabulary defined in `AGENTS.md` |
| `scripts/wiki-lint.py` existed and exited 0, but **nothing ran it** | medium → closed 2026-09-27 | wired into `ci.yml` as a *Wiki structural lint* step in the `python-tests` job — deliberately added only after the release-gate fix above, because a step added to a pipeline that already fails 17/17 adds no signal. The check is text-only over `wiki/**`, so it needs neither `dev-docs/` nor the Rust toolchain on the runner |
| `AGENTS.md` drifted from the wiki it governs (stale page counts, an outdated repo-status rule, inline-cite keys that no longer matched source pages) | medium (fixed 2026-09-27) | schema now states purpose, vocabulary, exemptions, and the two-register rule |

The lint is deliberately narrow: it checks **structural** invariants — traceability, link
integrity, classification, index coverage — not prose quality or whether a decision is
*correct*. Those stay human judgement. What a script can guarantee is that a decision is
findable, linked, and traceable to a source, which is the property that makes a blueprint
accumulate instead of rot.

## Doc-overstatement corrections (see also [[overview]] / [[roadmap]])
- "canary CI gates releases" → **CI was absent**; now scaffolded (M0), first hosted run pending.
- "OAuth-shaped token issuance" → issuer existed but was **not wired**; now the production default with aud/nbf.
- "9 providers" → 2 execute; the rest are capability entries.
- "QIR (Rust core)" in Platform P1 → Rust core exists but was **off the compiler path**; integration is an open gap.
