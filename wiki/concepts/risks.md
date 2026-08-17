---
type: concept
name: Key Risks & Mitigations
date: 2026-08-16
tags: [risks, kubona, governance]
sources: [source-gap-analysis-plan]
status: living
---

# Key Risks & Mitigations

The source plan's risk register (§9), preserved as a tracked artifact. Updated as the
project evolves.

| Risk | Mitigation | Status |
|---|---|---|
| Building a full hardware-abstraction layer from scratch is substantial scope; closed competitors ([[qbraid]], [[strangeworks]]) have a breadth head start | Phase provider onboarding deliberately (2–3 in Phase 0, expand through Phase 2); differentiate on the agentic governance layer nobody has shipped | Active |
| MCP credential compromise → runaway cloud QPU spend | [[cost-governance]] service non-optional in **Phase 1**, not deferred | Active |
| [[barren-plateau|Barren plateaus / WBPs]] silently degrade agent-driven VQA quality | Agent-behavior evals must include automated WBP-detection-response testing ([[evaluation-and-benchmarks]]) | Phase 4 |
| Meta-loop (Phase 4+) degrades search quality system-wide with **zero QPU spend** — invisible to the dollar gate | Capability-level human sign-off (a gate *separate from* [[cost-governance]]'s dollar threshold), versioned/reversible meta-changes, shadow-mode staged rollout; auto-apply only after Phase-5 proving ([[meta-search-and-self-evolution]] §13.4) | Phase 4+ |
| User/agent-created workflows & agents (Phase 6) widen the attack surface — an agent-created agent could escalate privileges or burn budget | Owner-delegated authority with instant revocation ([[user-identity-and-profiles]]), no privilege escalation through creation ([[workflows-and-agent-authoring]]), per-agent budget envelopes, capability sign-off + shadow mode for every new workflow/agent | Phase 6 |
| Scholarly/data integrations carry licensing & data-quality risk (TOU violation, stale/cited-wrong sources) | License/TOU flags + freshness/trust scores on every fetched source, citation provenance into the ledger, Tier-1 curating first ([[science-data-integrations]]) | Phase 6 |
| Six-language stack (Rust/TS/Dart/JS/Python/±Go) raises hiring & maintenance cost | Treat Go as optional; default to Rust for canary/ledger service | Decision |
| Export-control exposure from cross-border autonomous job routing | Jurisdiction field in [[provider-capability-schema]] + routing-layer enforcement before Phase 5 multi-tenant scale-out | Phase 5 |
| Open-source contribution governance for the security-sensitive MCP tool surface undefined | Deliberately deferred: Apache-2.0 from Phase 0; formal contribution/review process only when outside contributors appear (targeted Phase 4) | Deferred by design |

## The decision principle behind deferrals

Governance and monetization processes are sequenced *after* a working product
("functional product first", §9): building process before the product works is effort spent
on the wrong problem. Same logic defers [[x402]] activation and provider-readiness audits.

Related: [[roadmap]] · [[kubona]] · [[security-and-pqc]] · [[cost-governance]] ·
[[provider-capability-schema]]