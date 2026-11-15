---
type: concept
name: Provenance & Reproducibility
date: 2026-08-16
updated: 2026-11-14
tags: [provenance, reproducibility, ledger]
sources: [source-gap-analysis-plan]
status: adopted
---

# Provenance

> **Claim provenance as rows (B4, 2026-11-14).** A claim's evidence is no
> longer only a free `evidence_json` blob: `claim_evidence` links the claim to
> a typed record — `job_result` / `classical_anchor` (job id + SHA-256 of the
> exact measured result bytes), `literature`, `tribunal_turn`, or an explicit
> `general_knowledge` assertion. Job-minted claims (claim_eval, VQE,
> hardware_experiment, QAOA) record their measured link automatically; the
> tribunal records a turn reference. `GET
> /api/sessions/{id}/claims/{claim_id}/provenance` returns the claim, its
> evidence rows, and a summary: measured count, general-knowledge flag, and
> `complete` — a **verified** claim is complete only with at least one measured
> record or an explicit general-knowledge assertion. Evidence is recorded via
> `POST /api/sessions/{id}/claims/{claim_id}/evidence`: for measured kinds the
> server loads the job row (session-bound), derives the result SHA-256 itself,
> and the client cannot state a digest; literature/tribunal/general-knowledge
> rows require an explicit `source_ref`. Accepting a verified
> claim through the review rail refuses while `complete` is false, so a human
> decision cannot outrank its evidence. Contested verdicts are not graded by
> this rule (they are allowed to be incomplete), and the endpoint is bound to
> the claim's session with no existence oracle.
>
> **Citation-checked numeric claims (B5, 2026-11-14).** Every tool observation
> carries a turn-local id (`o1`, `o2`, …) inside its fenced bytes, and the
> copilot prompt requires a numeric value taken from an observation to be
> followed by `[oN]`. After the reply, a deterministic check (no LLM judge)
> scans prose outside fenced code for significant numbers (decimals, or
> integers ≥ 100), resolves the citation that follows within 12 characters,
> and compares the number with every numeric value in that observation within
> `1e-6 + 0.1%` tolerance. Statuses are `verified`, `uncited`,
> `unknown_observation`, or `not_in_observation`; the report is persisted as
> `citations_json` on the assistant message and returned in the unary,
> stream-done, and regenerate envelopes, so a reloaded turn keeps its
> provenance. The stream filter holds back a trailing incomplete `[oN` mark so
> a live citation is never torn; concatenated tokens plus flush equal the
> reply exactly. The console renders one badge per reply (`N/M numeric claims
> verified · K unverified`) with the unverified numbers and reasons in its
> title — unverified is shown, never hidden. Limits: prose only (tables and
> code fences are not scanned), and a quote rounded beyond the 0.1% tolerance
> is reported as not-in-observation rather than silently accepted.
>
> **Per-fact source chips (C9, 2026-11-14).** A workbench fact can now name
> the record that produced it (`facts[].from = {domain, id}`). Job cards source
> every fact to their job, loop cards to the loop, approvals to the
> obligation, tasks to the task; measurement and proof facts source to the
> measured job when one is recorded, otherwise to the campaign projection.
> Each sourced fact renders a small chip that focuses the owning card
> (`resolveFactSource` resolves by ref, prefixed id, id, or folded-run
> membership). Facts with no honest record to point at render no chip —
> nothing is invented. & Reproducibility

The quantum-computing analog of MLflow/W&B experiment tracking, absent from the
architecture paper and added by the gap analysis (source-gap-analysis-plan). Critical for
research and regulatory contexts (pharma, finance), and for the
[[karpathy-autoresearch]] algorithm library's credibility.

## Content-addressed experiment records

Every job submission writes an immutable record:
**circuit hash → backend → noise model → mitigation technique → raw results → mitigated
results → cost → wall-clock time.** At the science-discovery layer
([[science-discovery]]) the record extends to the full experiment lifecycle: **hypothesis →
protocol → controls → data sources fetched (with license/TOU flags, per
[[science-data-integrations]]) → analysis → structured report reference → citations** — so
the ledger doubles as the experiment log and the citation backbone of every result.

## Queryable by agents as an MCP Resource

The ledger doubles as a reasoning substrate: *"what mitigation technique worked best for
this ansatz class on this backend historically?"* — the platform gets smarter with use
(base for the algorithm library; see [[agent-resource-economy]]).

## Additional roles

- Enables crash-recovery / horizontal scaling of the nominally-stateless MCP servers
  (state reconstructed from the ledger via `job_id`).
- Immutable audit trail of every tool call, credential use, and dollar spent (shared with
  [[security-and-pqc]] and [[cost-governance]]).
- Human front-end: provenance/experiment browser console surface with credential-safe
  evidence export (pattern borrowed from [[qflow-studio]]).

Related: [[cost-governance]] · [[security-and-pqc]] · [[karpathy-autoresearch]] ·
[[agent-ux-control-plane]]