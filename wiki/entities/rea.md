---
type: entity
name: REA
date: 2026-10-17
updated: 2026-10-17
tags: [platform, agents, prior-art, reverse-engineering, mcp, contracts, typescript]
sources: [source-frontier-repo-audit-2026]
status: reference
---

# REA

Agent platform for reverse engineering ("reverse engineer anything with
agents, from app behavior down to native binaries"). TypeScript monorepo
(MCP stdio server + one-shot CLI over one application layer), MIT licence:

- `morluto/rea` — HEAD `e567033` (2026-10-17). → `https://github.com/morluto/rea`

> Method note: read as prior art for *mechanisms*, never as supply
> ([[integration-topology-and-licence-discipline]]: reference class). Claims
> below anchor to live docs at the SHA above: `docs/mcp-contracts.md`
> (canonical evidence, coverage, provenance, budgets, sessions),
> `docs/architecture.mermaid` (one application layer, thin adapters), plus
> the prior cross-repo investigation (2026-10-16, uncommitted scratch —
> this page and [[rea-adoption-plan]] are its durable, re-verified update).

## Why it matters to Kubona

REA's excellence is almost entirely in *platform mechanics*, not its reverse-
engineering domain — and Kubona's differentiators (tribunals, sealed ledgers,
mandates) are real but surrounded by engineering gaps REA has already solved.
The transferable patterns, each verified in its docs:

- **Canonical evidence envelope** — every evidence-producing record carries
  `evidence_id` (`ev_` + 64 lowercase hex, content-addressed),
  `normalized_result`, nullable provider-native `raw_result`, and an
  always-present `analysis_profile` that participates in semantic identity
  (records omitting it are rejected). Complete record in both text and
  `structuredContent` (`docs/mcp-contracts.md` §Tool results).
- **Coverage discipline** — inventories declare `coverage.scope` and
  `coverage.status` (`complete` | `unknown`) independently of diagnostic
  wording; consumers reject records without it; per-facet availability maps
  (`{available: true}` vs `{available: false, reason}`); missing coverage is
  unknown, never absence (same section).
- **Provenance-marked inputs** — any result that can carry target-derived
  text carries `_meta.provenance = {untrusted_content: true, source}` on
  success *and* failure; agents must treat it as data and report embedded
  directives as injection attempts (same doc, §Untrusted target content).
- **Bounded delivery** — pinned 10 MiB response budget; oversized results
  return `resource_constraint` *before* constructing the string, with a
  ready-to-call continuation (`inspect_analysis_view`, stable
  offset/limit paging); `detail: "summary"` projections with
  `parent_evidence_id` + `evidence_links` (same doc).
- **One application layer** — CLI and MCP server share `src/application/`
  workflows and evidence contracts; thin adapters only; module boundaries
  lint-enforced (`docs/architecture.mermaid`, `verify:module-boundaries`).
- **Validated session bundles** — imports reject unsupported fields *before
  authentication*; failed validation leaves session state unchanged; empty
  input yields an `unknown` ledger, never closure (same doc, bundle +
  obligation-ledger sections).
- **Obligation ledgers with closure criteria** — obligations carry ID,
  version, required case kinds, required authorities, dependencies, residual
  unknowns, disposition; closure needs a unique owner + every required case
  + a passing verifier of comparable authority (same doc).
- **Typed errors + exit statuses** — `isError: true` with canonical
  `{code, category, message, retryable, remediation, details}`; CLI
  `0` = completed incl. partial, `1` = could not complete, `128+N` =
  signals; prompts on stderr, JSON on stdout. Today's HEAD (`e567033`)
  shows the same discipline deepening: error causes threaded through
  cleanup owners with exact retry handles.
- **Named test lanes** (`test:fast`, `:focused`, `:changed`, `:boundary`,
  `:mcp`, `:acceptance`, `check:pr`) mapped to change type; generated
  product/test catalogs with `docs:check` CI gate (drift = CI failure).

## What this means for Kubona

Kubona already converges on several of these (S1 idempotency keys with
409-on-divergence mirror the `request_id` semantics; S5 validated manifests
mirror validated bundles; S6's enforced event taxonomy mirrors closed
vocabularies; the disk guard mirrors storage gating). The remaining transfer
is itemized, sequenced, and production-scoped in [[rea-adoption-plan]] —
which also records what was deliberately *not* borrowed.

Related: [[rea-adoption-plan]] · [[integration-topology-and-licence-discipline]] ·
[[provenance-and-reproducibility]] · [[spend-mandates-and-delegation]] ·
[[multi-agent-research-collective]] · [[cloudroom-adoption-plan]]
