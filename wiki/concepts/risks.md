---
type: concept
name: Key Risks & Mitigations
date: 2026-08-16
updated: 2026-10-09
tags: [risks, kubona, governance]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: adopted
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
| Four-language stack (Go/TS/Python/Rust) maintenance cost | Standardized 2026-10-04 (Go orchestrator, TS console, Python bridge, Rust compiler); Dart/Flutter retired Q-22 | Settled |
| Export-control exposure from cross-border autonomous job routing | Jurisdiction field in [[provider-capability-schema]] + routing-layer enforcement before Phase 5 multi-tenant scale-out | Phase 5 |
| Open-source contribution governance for the security-sensitive MCP tool surface undefined | Deliberately deferred: Apache-2.0 from Phase 0; formal contribution/review process only when outside contributors appear (targeted Phase 4) | Deferred by design |
| **Credential embedded in a git remote URL**: a `ghp_…` PAT sat in plaintext in the remote URL of `dev-docs/gather_insights_from/mainza-consciousness/.git/config` ((repo-audit-2026)) — the `git clone https://user:token@…` mistake. Checked against the API on 2026-09-27: **401, already revoked/expired**, and never present in a commit (`git log --all -S"ghp_"` empty), so nothing was exposed | Nothing to revoke; the string dies with the scratch folder. Keep the rules that made it visible: SSH or credential-helper remotes, never a URL with a secret, and a pre-commit/CI secret scanner that also covers `dev-docs/` ([[secrets-and-workload-isolation]]) | **Closed by verification (2026-09-27)** |
| **Plaintext credential sink in the operator's own git config**: `credential.helper=store` was previously set globally | Dropped `credential.helper=store` globally (leaving macOS keychain and `gh auth git-credential` as exclusive helpers); verified `~/.git-credentials` contains zero GitHub OAuth tokens. Ambient tools outside OpenBao cannot write secrets | **Closed (2026-10-26)** |
| **A local-only layer reaching the public repo**: `dev-docs/` (source corpus + scratch clones) and `.obsidian/` historical residue in commits prior to public release | History scrub executed via `git filter-repo --path dev-docs/ --path .obsidian/ --invert-paths --force` on 2026-10-26. Verified 0 residual objects across all commits on all branches, rewritten history force-pushed to origin, and branch protection rulesets re-enabled | **Closed by history scrub (2026-10-26) — Q-27** |
| **Treating own unfinished WIP as supply**: `chickensoup`, `milimoquantum`, `Milimo-QDD` are the maintainer's own, incomplete, and carry no `LICENSE` — so they read as "free to lift" locally and are in fact half-finished research code | Insight-only by policy: mine them for method, thresholds and data shapes, reimplement inside `platform/` with our own tests, and never merge a file across the boundary. No licence task now (one rights-holder); the single check is at release time — if a *derived* artefact ships in the Apache-2.0 core, that repo gets its `LICENSE` first ([[integration-topology-and-licence-discipline]]) | Closed by decision (2026-09-27); release-time check only |
| **Citation provenance drift**: the audit found a folder named `milimo-quantum-drug-discovery` whose git `origin` is actually `mainza-ai/kubona`, and several earlier wiki claims pointed at files that do not exist in the repos cited — evidence that "we read it" degrades silently | Always resolve a claim to `git remote get-url origin` + `HEAD` + a path:line, and prefer the live URL over the clone path; re-verify before a claim informs an architectural decision; clones are deleted, so the URL and SHA are the record | Active — enforced by this audit |
| **Dependency graph grows to the size of the reading list**: 25 audited repos, most of them valuable as *ideas* only | "Submodule" is a shipping decision requiring a permitting licence, continuous use, and a named owner to run pin/SBOM/sync; everything else stays reference-grade with a memo, not a dependency ([[integration-topology-and-licence-discipline]]) | Active by design |
| Disk-full outage from bursty writers (HF model GBs, append-only jobs log, PySCF scratch) with no guard | S2 disk guard landed 2026-10-17: WARN 2 GB / BLOCK 1 GB / RESUME 1.5 GB with hysteresis over the resolved state + models mounts; BLOCK 503s `storage_blocked` on new work, reads stay; `/api/ready` probe | Mitigated — heaviest-writer kill still needs cgroup work |

## The decision principle behind deferrals

Governance and monetization processes are sequenced *after* a working product
("functional product first", §9): building process before the product works is effort spent
on the wrong problem. Same logic defers [[x402]] activation and provider-readiness audits.

Related: [[roadmap]] · [[kubona]] · [[security-and-pqc]] · [[cost-governance]] ·
[[provider-capability-schema]]