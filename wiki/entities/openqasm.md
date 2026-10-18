---
type: entity
name: openqasm (OpenQASM)
date: 2026-09-27
updated: 2026-10-17
tags: [quantum, language, grammar, submodule-candidate]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# openqasm — the parse contract for anything an agent emits

**https://github.com/openqasm/openqasm** · Apache-2.0. Integration class:
**Candidate submodule (grammar only)** — the design proposes vendoring `source/grammar/`, not a runtime
([[integration-topology-and-licence-discipline]]); in the prototype today, only `submodules/math` is
mounted in `.gitmodules`, and the compiler/validator enforces the locked OpenQASM 2.0 subset while
explicitly refusing `OPENQASM 3` inputs.

The load-bearing artefacts are the ANTLR grammars **`source/grammar/qasm3Lexer.g4`** and
**`source/grammar/qasm3Parser.g4`**, alongside the language specification and `examples/*.qasm`
corpus.

Why this earns the only submodule slot that is *data*: when agents author circuits, the language
they write in needs a machine-readable boundary. A grammar file gives Kubona three things at once —

1. a **contract** every front-end is validated against, so "the agent produced valid QASM" is a
   parse result rather than an opinion ([[mcp-and-context-flooding]]);
2. a **ground truth for repair loops** — the parser's error positions are where a fix passes get
   applied ([[agentic-qir-compilation]]);
3. a **conformance corpus** — the shipped examples become table-stakes tests
   ([[evaluation-and-benchmarks]]).

Apache-2.0 keeps the grammar distributable inside the Apache-2.0 Kubona tree. If promoted from
`candidate` to `adopted`, it would be pinned as a submodule with a SHA and an SBOM entry like any
other dependency; the spec text itself is cited, not vendored.