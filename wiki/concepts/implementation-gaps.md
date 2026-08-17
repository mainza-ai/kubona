---
type: concept
date: 2026-08-16
tags: [kubona, production, gaps]
sources: [source-gap-analysis-plan]
status: living
---

# Implementation Gaps (production register)

> The honest, living register of what the [[overview|platform]] has vs. what production
> requires. This page exists so a skeleton/engine implementation is never mistaken for a
> deployable one, and so each gap is closed with its evidence, not an intent.

## Status legend
- 🟢 **Closed (M0)** — structural, cheap-now fixes landed.
- 🟠 **Open + required before real deployment** — the "we'd redo it later" items.
- 🔵 **Deferred by design** — matches the plan's own "functional product first" sequencing.

## Critical (structural)

| Gap | Status | What's real / what's missing |
|---|---|---|
| **Persistent state root (data-loss)** | 🟢 M0 | Was split: 8 stores hardcoded to volatile `/tmp/kubona-state` (settings, identity, agents, the algorithm library, reports, labs, workflows, meta-proposals) while sessions/gov/ledger used `KUBONA_STATE_DIR`. Now all route through `kubona/config.py` (`state_root()`) — default `~/.kubona`, epoch-safe. |
| **Prod auth default** | 🟢 M0 | Gateway + console defaulted to `dev-token`. Now: OAuth2 signed tokens get `aud`/`nbf`, console uses constant-time compare against `KUBONA_CONSOLE_TOKEN`, and `KUBONA_ENV=production` **fails closed** (no token → no access). Production gateway factory in `kubona/run.py`. |
| **Deployment entrypoint** | 🟢 M0 | No `[project.scripts]`, no way to boot the system. Now: `kubona serve` / `kubona doctor` (package entrypoint + `run.py`), starlette/uvicorn are hard deps. |
| **CI / release-gate** | 🟢 M0 (scaffold) | No `.github/` at all; "canary CI gates releases" was a module you ran by hand. Now: `ci.yml` (python tests, Rust `--lib`, flake-free canary + correctness gate, Flutter) + `nightly-canary.yml`. First real run still pending a hosted runner. |
| **x402 fail-closed** | 🟢 M0 | The stub verifier accepted any `x402:`-prefixed proof (free-spend hole if enabled). Now `X402Payments` rejects every proof unless a real `verifier` is injected; metering/payment + P1 gate updated to assert fail-closed. |
| **Async, persistent job model** | 🟢 M1 | `kubona/jobs.py`: persistent JSONL `JobStore` (status survives restart) + background `JobRunner`; `submit_job(wait=True/False)`, `cancel_job` (P2 watch-and-interrupt), scoped `execution:override`, provenance on every transition, 4 dedicated tests. |
| **Real provider SDK execution** | 🟠 | "9 providers" = 2 execute (local-simulator, qiskit-aer) + 7 raise "not wired (needs credentials+SDK)"; IBM path only runs Aer, not Qiskit Runtime. Wire real SDK submission (IBM Runtime, pytket, pyquil, dimod+Leap, strawberryfields) with recorded-fixture tests. |
| **Rust QIR/MLIR core on the call path** | 🟢 M1 | `kubona/qir/native.py` runs the built `qir-core` binary on `lower_circuit` (`engine: rust-qir-core`) with Python fallback; CI builds it and wires `KUBONA_QIR_CORE`; native-path test added. OpenQASM 3 + MLIR/QIR-bitcode remain. |
| **Streamable-HTTP + ML-KEM on the wire** | 🟠 | Transport is InProcess/stdio; `secure_rpc.py` is an in-process stand-in. Ship HTTP with OAuth 2.1 bearer + hybrid TLS; secrets management. |

## Medium

| Gap | Status | Note |
|---|---|---|
| Structured observability/metrics/audit export | 🔵 | Burn-rate is in-session; add logs + OTel/Prometheus + audit export. |
| Volume eval corpora (QuantumKatas/HumanEval/PennyLang) | 🔵 | Built-ins are 5 invariants; wire volume corpora behind the same contract. |
| GQE transformer (Unsloth) training | 🔵 | n-gram stand-in today; drop-in `propose` replacement on a GPU host. |
| Full circuit knitting | 🔵 | LOOC-Z only (Z-diagonal expectations); general CKT if needed. |
| Perf/load baselines | 🔵 | Pure-numpy simulator; add perf regression + memory guards. |

## Deferred by design (correct, per plan)
- x402 activation on a real rail (Phase 5+), meta-loop auto-apply (§13.5, Phase 5+), Unsloth personas (Phase 6), real third-party provider PR, third-party security review execution, live arXiv/OpenAlex default (network-acknowledged; mock is the offline default).

## Doc-overstatement corrections (see also [[overview]] / [[roadmap]])
- "canary CI gates releases" → **CI was absent**; now scaffolded (M0), first hosted run pending.
- "OAuth-shaped token issuance" → issuer existed but was **not wired**; now the production default with aud/nbf.
- "9 providers" → 2 execute; the rest are capability entries.
- "QIR (Rust core)" in Platform P1 → Rust core exists but was **off the compiler path**; integration is an open gap.
