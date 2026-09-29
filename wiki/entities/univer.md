---
type: entity
name: Univer (dream-num)
date: 2026-09-27
updated: 2026-09-27
tags: [frontend, ux, spreadsheet, submodule-candidate, apache-2.0]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Univer — the office-grade surface inside the console

**https://github.com/dream-num/univer** · Apache-2.0. Integration class:
**Submodule (candidate)** — vendored only if the notebook/results grid actually needs it, not
because the demo looked good ([[integration-topology-and-licence-discipline]]).

A spreadsheet/document/slide engine split into composable packages:
`packages/{core,engine-formula,engine-render,docs,drawing,action-recorder,data-validation}`.

Three of those names map onto Kubona surfaces directly ([[agent-ux-control-plane]]):

- **`engine-formula`** — a real formula engine behind the experiment notebook, so a derived number
  in a table is *computable and diffable* rather than a pasted value; the same requirement
  [[provenance-and-reproducibility]] imposes on results applies to the UI's own arithmetic.
- **`action-recorder`** — user actions as recorded events. In a platform whose ledger is the product
  ([[claim-credibility-wavefunctions]]), a human edit to a claim or a cost ceiling should be an
  event with an identity attached, not a silent overwrite.
- **`data-validation`** — the enforcement point for [[quantum-domain-packs]]: pack editing is
  schema-checked, and the pack must **round-trip losslessly through JSON** or every graphical edit
  silently corrupts the pack.

Apache-2.0 keeps it inside the licence budget. The constraint that decides *whether*: it is a large
dependency, so "does the console need a spreadsheet engine, or a table renderer?" must be answered
with a UI prototype before this earns a slot in `.gitmodules`.