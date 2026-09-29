---
type: entity
name: Paperclip (paperclipai)
date: 2026-09-27
updated: 2026-09-27
tags: [mcp, governance, multi-agent, prior-art]
sources: [source-frontier-repo-audit-2026]
status: reference
---

# Paperclip — MCP access governance, written down

**https://github.com/paperclipai/paperclip** · MIT · HEAD verified 2026-09-26.
Integration class: **Blueprint** — reference-grade; we cite it and implement independently
([[integration-topology-and-licence-discipline]]).

The most complete governance documentation set found in the audit, and the reason Kubona does not
have to invent its own vocabulary:

- **Four separated access planes** — Application (logical) → Connection (endpoint + transport) →
  Catalog (tools + schema, risk-classified) → Profile (named allow/deny bundle), attached by
  **Bindings** to `company` / `agent` / `project` / `routine` / `issue`
  (`doc/MCP-ACCESS-GOVERNANCE.md:31,57,206`).
- **A grant must be audited to count as one** — its smoke test asserts `allow_read_tool`,
  `deny_write_tool` **and** `audit_written` together (:92).
- **Low-trust presets** for "automated work that may consume hostile or prompt-injected input, such
  as untrusted pull requests, external tickets, dependency diffs" (`doc/LOW-TRUST-PRESETS.md`), and
  review that runs in a container "intentionally separate from the normal dev image"
  (`doc/UNTRUSTED-PR-REVIEW.md`).
- **Four things agents always blur** — structure (parent/sub-issue), dependency (blockers),
  ownership (responsible now), execution (is there a live path?) (`doc/execution-semantics.md`).
- **Task watchdog** — opt-in per issue, fired only when every leaf of the watched subtree is at rest
  with no live continuation path; "verification-shaped, not execution-shaped"
  (`doc/TASK-WATCHDOG.md`).
- Eval harness in `evals/promptfoo/`; memory-as-files skill in `skills/para-memory-files`.

Lands on: [[multi-agent-research-collective]] (trust ladder, signed events, Ring-0 trigger shape),
[[secrets-and-workload-isolation]] (plane separation), [[cost-governance]] (audited grants).