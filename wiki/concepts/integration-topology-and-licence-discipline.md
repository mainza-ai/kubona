---
name: Integration Topology and Licence Discipline
type: concept
tags: [frontier, governance, licensing, supply-chain, submodules, architecture]
date: 2026-09-27
updated: 2026-10-08
status: settled
sources: [source-frontier-repo-audit-2026, source-openai-math-corpus]
---

# Integration Topology and Licence Discipline

**Definition.** Every external repository Kubona learns from is assigned exactly **one of four
integration classes** before any code is written. The class is chosen by *licence first,
engineering second*, because in a platform that publishes provenance records, an unlicensed
derivative is not a style violation — it is an unfixable defect in the artifact.

## The four classes

| Class | Meaning | Obligations | Failure if ignored |
|---|---|---|---|
| **Submodule** | Apache-2.0/MIT code read or linked at pinned SHA; vendorable with attribution | `LICENSE`/`NOTICE` retained, SHA pinned, sync script refresh | licence drift, silent upstream changes |
| **External binary** | Toolchain invoked as a *process*, never vendored (LLVM/MLIR, `bao`, provider CLIs) | pinned version contract, startup capability probe, degrade when absent | unmaintainable fork of a compiler monorepo |
| **Sidecar** | Service reached over MCP/HTTP with graceful degradation | health probe, timeout budget, **never** on the correctness path | platform down because a memory DB is down |
| **Blueprint** | Read for design; **reimplemented under our licence**; no code, data or assets copied | design memo + independent implementation + CI equivalence proof | legal exposure; provenance records nobody may ship |

Full assignment table: [[source-frontier-repo-audit-2026]].

**"Submodule" is a shipping decision, not a compliment.** Most of the audited repos will never
appear in `.gitmodules` — they are **reference** material, and the correct output of borrowing an
idea from them is a *citation plus an independent implementation*, not a dependency. Keeping those
two categories apart is what stops the dependency graph growing to the size of the reading list.
The submodule and candidate list is deliberately short, and each entry points at its **live GitHub
URL** (the local clone under `dev-docs/submodules/` is a temporary read mirror that is deleted when
development ends, so the URL is the durable reference):

| Submodule / Candidate | Live URL | Licence | Status & Why it earns submodule status |
|---|---|---|---|
| [[openai-math]] | `github.com/openai/math` | Apache-2.0 | **Adopted submodule** (`submodules/math`, SHA `adc7f1241`); provides 372 mathematical discovery families, 162 Lean 4 formalizations, 811 Comparator challenges, and analytical physics anchors |
| PennyLane | `github.com/PennyLaneAI/pennylane` | Apache-2.0 | Candidate; the differentiable engine itself; we ship against it, not a fork ([[differentiable-quantum-stack]]) |
| QASM3 grammar | `github.com/openqasm/openqasm` | Apache-2.0 | Candidate; `source/grammar/qasm3{Lexer,Parser}.g4` is the parse contract we validate front-ends against |
| LLVM/MLIR | `github.com/llvm/llvm-project` | Apache-2.0 + LLVM exception | **External binary**, not a submodule — the monorepo is never vendored |

Everything else in `dev-docs/submodules/` (`NVIDIA/Model-Optimizer`, `dream-num/univer`,
`actions/runner-images`, `tt-a1i/archify`, …) is currently **reference-grade**: we read the pattern,
we write the memo, we ship none of their bytes. Promotion to submodule requires three things at
once — a licence that permits it, a component we use *continuously* rather than once, and an owner
willing to run the pin/SBOM/sync choreography for as long as Kubona lives.

## The hard cases, decided

**Unlicensed ≠ permissive-by-default — but it also isn't a wall against yourself.** A
**third-party** repo carrying no `LICENSE`/`COPYING`/`LICENSE.md` at any depth is
*all rights reserved*; the assumption that a public GitHub folder is reusable is wrong, and is the
most expensive mistake available to this project ((repo-audit-2026)).
The three unlicensed repos the audit found — [[chickensoup]], `milimoquantum`, `Milimo-QDD` — are
**the maintainer's own**, which dissolves the question rather than answering it: mining them for
method, thresholds and data shapes during implementation involves no grant, because there is one
rights-holder on both sides, and several of them are deliberately **unfinished WIP** whose value here
is the idea, not the code. So there is no licence task on the roadmap. The task that does exist is a
single check at release time: *if* an artefact derived from that code ships inside the Apache-2.0
core, `LICENSE` goes in that repo **before** the release, so the provenance record is honest.
Everything below describes the third-party case, which is where the discipline earns its keep.

> ⚑ A fourth folder in the audit — `milimo-quantum-drug-discovery` — was listed alongside these as
> an unlicensed owner repo. It is **not a repository at all**: its `origin` is
> `mainza-ai/kubona.git`, and no such upstream exists (GitHub 404). It is a scratch clone of our own
> tree, so it grants nothing and evidences nothing; its method content stays citable only as
> [[roadmap]]-internal design ([[source-frontier-repo-audit-2026]]).

**MPL-2.0 is file-level, not project-level.** [[openbao]]'s weak copyleft means *modified files*
must be published under MPL while Kubona stays Apache-2.0 — which is why OpenBao is a **sidecar**,
not a linked dependency: the boundary that keeps obligations tidy is the same boundary that keeps
credentials from agents ([[secrets-and-workload-isolation]]).

**Copyleft stays behind the process boundary.** [[buzz]] is **Apache-2.0** and its upstream is
https://github.com/block/buzz — the local clone is just a read mirror of that URL, and any reference
in this wiki means the upstream, not our copy. Its `crates/ifc-core` lattice API
(`ConfidentialityLabel`, `can_flow_to`) is *design source* for
[[confidentiality-lattice-provenance]]; we do not vendor it today, and if a relay is ever run, it
runs as a **sidecar** for identity reasons (an agent's key is its identity, and the relay owns key
storage — see [[secrets-and-workload-isolation]]). Apache-2.0 means vendoring is *permitted*;
permitted is not obligatory. The rule that does bind us is this: any future AGPL or GPL component
may only ever cross that same process boundary — no linking into the Apache-2.0 core, at any depth.

**Trademarks are not licences.** Google's `agents`/`financial-services` are Apache-2.0 *code*, but
agent names ("Arial", "Warren") and branding are Google's. We copy `agent.yaml` *structure*, never
identity ([[multi-agent-research-collective]]).

**"Apache-2.0 repo" ≠ "all Apache-2.0 files".** `univer` keeps `MATERIALS.md` listing third-party
assets; `qiskit` carries `CODE_OF_CONDUCT.md` (Citizen Convention, its own text); MLIR sources are
Apache-2.0 *with LLVM exceptions* and the tree carries several compatible licences. Scanning must
be per file, not per repo — the naive version of licence checking is the one that fails.

**LLM-generated code is a provenance object.** `chickensoup` documents itself as created with LLM
assistance "in a single working session". Kubona records circuit and device provenance; it must
equally record *who wrote this code under what licence* ([[provenance-and-reproducibility]]),
because the derivative question is asked per line.

## Where each class lands

```
platform/kubona/, crates/   Apache-2.0 core. Only Submodule-class code may be linked here.
vendor/, submodules/        pinned SHAs + LICENSE/NOTICE retained; refreshed by script
sidecars/                   compose profiles: openbao, hindsight, buzz relay (optional features)
dev-docs/submodules/*       local working copies (never committed; .gitignore)
wiki/concepts/*             design memos Blueprint integrations must produce before code
```

## Enforcement, not exhortation

1. **Per-file CI licence scan** with a policy table (allow Apache-2.0/MIT/BSD/ISC; MPL-2.0 as
   sidecar; deny unknown, non-commercial, GPL-into-core) — *design requirement* (in the
   prototype today, corpus licence gating is enforced at load time by `kubona/eval/corpus_loader.py`,
   while a repo-wide per-file CI licence scanner remains queued; see [[deep-implementation-audit-2026-10-12]]).
2. **A CI secret scanner that fails on credential-bearing remote URLs and token prefixes** — enforced
   in `scripts/secret-scan.sh` and `.github/workflows/ci.yml`, catching the specific failure
   observed in a local working copy of a local project ((repo-audit-2026),
   [[secrets-and-workload-isolation]]).
3. **Blueprint receipts**: each design memo links its source and names the test proving behavioural
   equivalence of the independent implementation — what stops a "memo" quietly becoming paste six
   months from now.
4. **External-binary probes**: startup fails loudly with a capability report rather than
   half-working. [[qflow-studio]]'s `cloud_provider_api_configs` staleness windows are the best
   pattern for *vendor drift* — version and capability assertions must age out
   ([[provider-capability-schema]]).
5. **Degrade, never fake**: sidecars optional at runtime, mirroring `kubona/qir/native.py`
   (`engine: "rust-qir-core"` vs `"python-reference"`) and chickensoup's `classical-fallback`
   markers. The result always says which path produced it.

The licence determines the integration class, and the integration class determines the
architecture — which is why this page is architecture documentation, not a lawyer's afterthought.

See also [[source-frontier-repo-audit-2026]], [[five-ring-research-loop]],
[[secrets-and-workload-isolation]], [[provenance-and-reproducibility]], [[qflow-studio]].