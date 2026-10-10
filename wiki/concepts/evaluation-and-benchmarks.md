---
type: concept
name: Evaluation & Benchmarks
date: 2026-08-16
updated: 2026-10-10
tags: [evals, testing, benchmarks]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Evaluation Framework & Benchmarks

The architecture paper proposed only **code-generation evals**; the gap analysis splits
evaluation into four complementary tracks (source-gap-analysis-plan).

## 1. Code-generation correctness (from the architecture paper)

- **QuantumKatas** + **Qiskit HumanEval** — hundreds of tasks (basic gates → Grover,
  Deutsch-Jozsa) verified deterministically via classical simulation vs canonical
  statevectors.
- **PennyLang** ([[pennylane]]) — 3,000+ curated samples; RAG-enhanced generation lifts QML
  code-gen success from single digits to ~84%.

> ⚑ Volume-corpus status (2026-10-10): **QuantumKatas — transcribing (done:
> 13 tasks).** 8 MIT-licensed kata exercises transcribed into QASM
> satisfiability tasks (`quantum_katas_volume.json`, kata provenance per
> entry); all 16 corpus tasks score 1.0 on the simulator. Katas without a
> strongly-checkable invariant (Deutsch-Jozsa, teleportation, arithmetic)
> are excluded by design. **HumanEval / PennyLang — deferred, new harness
> required.** Both are *code* corpora (Python codegen + unit tests / QML
> samples for RAG), not circuits: they cannot drop into the {qasm, check}
> registry without misrepresenting the benchmark. Ingesting them means a
> sandboxed code-execution harness (untrusted generated code + test runner),
> a security-sensitive build tracked as P4 follow-up work — not a corpus
> file. Faking it via QASM transcription would corrupt every claim built
>   on the score.

## Code-execution harness (scoped then built 2026-10-10 — live-proven)

> Update: `kubona/eval/codeexec.py` implements this scope the same day.
> 10 tests green including a live-container test: golden completion passes,
> socket attempt fails closed with `Network is unreachable` named in the
> ledger record. An image ENTRYPOINT trap (`python3 python ...`) was caught
> live and fixed by pinning `--entrypoint`. HumanEval/PennyLang sample
> ingestion (licence-gated) remains the follow-up, not the harness.

## HumanEval ingested (2026-10-10): 136 golden-verified, 15 excluded-named

`kubona/eval/corpora/humaneval.json` (Apache-2.0 meta with source URL, terms
pointer, and parquet SHA) + `humaneval.excluded.json`. Every admitted task
was golden-verified live the same day: its reference passes its tests inside
`kubona-quantum-runner:1.1.0` (image extended with pinned
qiskit-ibm-runtime + matplotlib for fake-provider tasks; `codeexec` default
bumped 1.0.0 → 1.1.0 by version, never silently). Exclusion taxonomy, nothing
dropped silently: 9 touch live IBM credentials (unverifiable without the
operator token, which never enters a sandbox); 6 are toolchain/drift facts
(version-brittle transpiler assertion, seaborn/Graphviz/pylatexenc drawer
deps deliberately unbaked, transpiler kwarg drift, qiskit_qasm3_import).
Reproducer (outside the hermetic venv): HF `Qiskit/qiskit_humaneval` parquet
→ records JSON → `ingest_humaneval` tiering → per-task `run_codeexec_task`
golden runs (48+48+40/42 passes across three batches; the 6 failures became
the named exclusions above). PennyLang samples still pending (per-file
licence metadata).

## PennyLang refused under Q-26 (licence audit 2026-10-10)

Source: `eBrain4Everyone/Pennylang` (`dataset.json`, 49MB, 3,347 samples;
repo LICENSE is MIT). Audited against the Q-26 rule — **refused, whole
dataset**, for two independent reasons:
1. **No per-sample provenance.** Entries carry only `{instruction,
   response}` — no source URL, no licence, nothing to verify. Per the paper
   (arXiv 2503.02497) composition: 1,952 samples from *unofficial* GitHub
   repos (licences unknown), 21 from *textbooks* (presumptively
   copyrighted), annotations GPT-4o-generated. A curator's repo-level MIT
   cannot relicense scraped third-party code or textbook excerpts, and
   without provenance no licence-clean subset is identifiable — filtering
   is impossible, not just unimplemented.
2. **Wrong shape for execution.** Instruction–response RAG pairs with no
   tests; running them in `codeexec` would require synthesized tests, i.e.
   grading our own invention.

This is the exact corruption class the licence gate exists to catch, and
the refusal is its first live fire. Path back, if ever wanted: re-derive
from PennyLaneAI-official sources (Apache-2.0) with per-sample provenance
— a new curation build, not an import. P4 eval front is therefore closed:
Katas ✓, HumanEval ✓ (136), harness ✓, PennyLang refused-named.
**Threat model.** The harness runs *untrusted generated Python* (HumanEval
completions, PennyLang-style samples): arbitrary code that may attempt
exfiltration, fork bombs, miners, or payload-bearing task text. It is
therefore a sandbox build, not a test-runner script — same discipline as
the Q-31 Lean sandbox (`comparator_eval.py`): pinned image, `--read-only`,
`--network none`, `--cap-drop ALL`, memory/CPU/PID caps, timeout, no
implicit pulls, and a hard infra-failure-vs-verdict distinction (a crashed
container is never a passing test).

**Reuse, not new infrastructure.** Execution reuses `Dockerfile.quantum-runner`
(qiskit + pennylane already pinned at build time); no runtime `pip install`.
Config mirrors `LeanSandboxConfig` as `PyexecSandboxConfig` in a new
`kubona/eval/codeexec.py`: task = {prompt, entrypoint, tests}; the runner
writes the completion into the container workspace, runs the task's tests of
record, parses pass/fail, and appends one ledger record per task. No Docker
daemon → skip, never pass (the all-skip-is-not-a-pass rule from `release_gate`).

**Licence gate first.** HumanEval is Apache-2.0 (admissible); PennyLang
samples need per-file licence metadata through the Q-26 loader rule before a
single sample executes — unlicensed RAG text is the same corruption class as
an unlicensed benchmark.

**Acceptance.** (1) A golden HumanEval-style fixture passes; a malicious
fixture (socket connect, fork bomb, `/etc` read) fails closed with the
attempt named in the report. (2) Infra kill (daemon stop mid-run) reports
infra-failure, not test-failure. (3) Every verdict carries image digest +
test hash in the ledger record.

**Non-goals.** No RAG generation pipeline (the PennyLang generation side is
separate work); no network at runtime; no host-side fallback execution —
anything that cannot run containerized does not run.

## 2. Platform integration/regression (new)

- Does the [[qir-mlir-compilation|QIR lowering]] pipeline still produce correct bitcode after
  a compiler change?

## 3. Hardware-in-the-loop CI (new)

- **Hardware-canary** — nightly run of a canary circuit against *every* connected backend to
  catch silent provider API breakage / calibration drift before an agent hits it mid-workflow
  (Phase 3; candidate home: Go service or Rust).
- The correctness-regression harness of the [[karpathy-autoresearch]] engine runs here and
  **gates releases**, not just logging.

## 4. Agent-behavior evals (new, Phase 4)

- Does the agent route simulator-first? Respect budget ceilings? Correctly detect and
  respond to a [[barren-plateau|WBP]] signal instead of looping? Budget-compliance and
  simulator-first compliance are explicit eval dimensions.

Related: [[karpathy-autoresearch]] · [[qir-mlir-compilation]] · [[barren-plateau]] ·
[[cost-governance]] · [[qiskit]]

## Admissible corpora (Q-26, closed 2026-10-10)

- **Licence-clean only.** Every corpus file carries `_meta: {license, source_url,
  retrieved_at}`; the loader admits in-repo authored, CC0/CC-BY(+SA),
  Apache-2.0, MIT/BSD, Unlicense — and refuses whole files that are
  proprietary, unknown, or unlabelled, with a per-file reason
  (`load_corpus_dir_with_report`). An unlicensed benchmark laundered into evals
  corrupts every claim built on it, so the gate is fail-closed, not advisory.
- **Good-answer criterion.** A good answer is QASM satisfying the task's
  *declared invariant* on the simulator (score = fraction passing). No
  LLM judging, no vibes — satisfiability is the entire criterion, verified by
  `evaluate_codegen` (in-repo corpora additionally prove satisfiability at
  load-test time).