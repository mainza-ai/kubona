---
name: The Multi-Agent Research Collective
type: concept
tags: [frontier, multi-agent, governance, roles, trust, orchestration]
date: 2026-09-27
updated: 2026-10-04
status: adopted
sources: [source-frontier-repo-audit-2026]
---

# The Multi-Agent Research Collective

**Definition.** Kubona's agents are not a swarm of interchangeable chatbots. A collective is a
**role graph × capability grants × signed events × trust policy** — four orthogonal objects,
each of which already exists somewhere in audited source form. The failure mode this prevents is
the common one: "multi-agent" systems where every agent can do everything, so
accountability — and therefore science — is impossible.

## Four objects, four verified references

**1 · Role graph.** [[buzz]] (https://github.com/block/buzz, Apache-2.0) already ships roles as
versioned artefacts: `crates/buzz-persona/src/{pack,manifest,persona,resolve,merge}.rs` builds a
**persona pack** with a manifest, resolves it and merges layered persona fields — so many roles
coexist on one relay without one identity each, and `docs/agent-profile-identity.md` treats the
agent's key as its identity *(design reading of code; the prose claim lives in
`ARCHITECTURE.md`)*. Kubona's roles map onto the rings: `epistemic-scribe` (Ring 1 claims),
`compiler-scout` (Ring 3 pass search), `ansatz-scout` (Ring 3 genomes), `device-auditor`
(fidelity probes), `disconfirmation-officer` (mandatory skeptic), `human-reviewer` (holds the
real-QPU grant).

**2 · Capability grants.** Anthropic's `financial-services` cookbooks (Apache-2.0) are the
cleanest declarative form found anywhere: `managed-agent-cookbooks/*/agent.yaml` opens with
`tools: [{type: agent_toolset_20260401, default_config: {enabled: false}, configs: [{name:
read, enabled: true}, ...]}]` — **deny by default, then name each tool** — adds
`mcp_servers: [{type: url, name: factset, url: "${FACTSET_MCP_URL}"}]` (endpoints from env,
never inline secrets), `skills: [{from_plugin: ...}]` (role skills as a package), and
`callable_agents: [{manifest: ./subagents/note-writer.yaml}]` with the telling comment *"only
leaf with Write"* — least privilege made visible in a manifest ((repo-audit-2026)). Kubona's
`role.yaml` adopts this shape verbatim, with tools bound to MCP server names and
[[secrets-and-workload-isolation]] leases.

**3 · Signed events.** [[buzz]] again: agents are keys, coordination is a signed event, and
`ARCHITECTURE.md:503` names the storage property we need — *"Tamper-evident append-only log with
SHA-256 hash chaining"*. [[paperclip]] (MIT, https://github.com/paperclipai/paperclip)
contributes the vocabulary that keeps this from becoming a workflow engine:
`doc/execution-semantics.md` (2026-08-18) insists on separating four things agents always blur —
**structure** (parent/sub-issue), **dependency** (blockers), **ownership** (who is responsible
now), **execution** ("whether the control plane currently has a live path to move the issue
forward") — and `doc/TASK-WATCHDOG.md` is a second pass on stopped work: opt-in per issue, fired
only when *every* leaf of the watched subtree has come to rest and "there is no live continuation
path", and explicitly "verification-shaped, not execution-shaped" — plus a separate silent
active-run watchdog (`doc/execution-semantics.md` §12). Kubona's Ring 0 borrows the **trigger
shape** (subtree at rest + no live path) while staying deliberately non-LLM. `OpenMAIC` (MIT) shows the choreography half working
at scale: `@langchain/langgraph` ^1.1.1 with `lib/choreography` +
`lib/agent-runtime/lifecycle.ts` orchestrating multi-role classroom sessions, i.e. turn-taking
and stage transitions as *data* rather than as prompt folklore.

**4 · Trust policy.** [[paperclip]] ships a **low-trust preset** —
`doc/LOW-TRUST-PRESETS.md` exists for "automated work that may consume hostile or prompt-injected
input, such as untrusted pull requests, external tickets, dependency diffs" — and separates MCP
access into four planes so no single switch does too much: **Application** (logical) →
**Connection** (endpoint + transport) → **Catalog** (tools + schema, risk-classified) →
**Profile** (named allow/deny bundle), with **Bindings** attaching profiles to `company`,
`agent`, `project`, `routine`, `issue` (`doc/MCP-ACCESS-GOVERNANCE.md:31,57,206`). Its own smoke
test asserts three things together: `allow_read_tool`, `deny_write_tool`, `audit_written` (:92) —
a grant that is not *also* audited is not a grant. Plugin sandboxing is specced in
`doc/plugins/PLUGIN_SPEC.md` and `doc/plugins/SANDBOX_PROVIDER_CAPABILITIES.md`; secrets are
scoped references (`doc/plans/2026-04-26-plugin-secret-ref-company-scope.md`).
`reverse-skill` (MIT) supplies the role and scope documents —
`skills/ops/{role-map,scope-contract,sandbox-profile,skill-supply-chain,identity}.md` — and the
toughest gate in the audit: `skills/scripts/case-guard.sh`, a "scope gate before ACT" whose
documented exit codes are 0 = ok, 2 = not ready (:3), which reads a human
`ready_for_act` sign-off field and **refuses to be forced**: `--force` prints *"--force does not
bypass scope hard gates"* and still `exit 2` (:122-127). `gods-eye-view` supplies the enforcement
idiom: one `policy.js` per layer with `scripts/check-package-boundaries.mjs` checking the seam.
Kubona's trust ladder: **low** (proposes only; every
action is a request), **member** (runs simulator jobs inside a budget), **trusted** (may hold
leases, may assert collapsed claims), **human** (grants real-QPU spend). A role's trust level is
a signed grant object, evaluated at every tool call — not a field in a prompt.

## Disconfirmation as a standing role

Ring 1's tribunal ([[five-ring-research-loop]] rule 3) is *staffed*: the
`disconfirmation-officer` holds the same toolset as the proposer it audits and is scored on
findings, not on agreement. The pattern is verified twice: `chickensoup`'s
`TribunalAgent.should_trigger_tribunal` / `run_tribunal`
(`src/agents/tribunal_agent.py:128,151`) consumes the claim wavefunction as evidence context
(`_build_evidence_context(..., wavefunction)`), and `Milimo-QDD` closes its pipeline with a
`FAILURE_ANALYSIS` agent that explains rejected candidates (`src/types.ts:14`). A collective
without a funded skeptic produces confident noise at scale.

## Concurrency without corruption

Two agents must never mutate one campaign: adopt the TTL'd single-writer gate from
`chickensoup/src/reconciliation_gate.py:25-102` (`acquire`/`release`/`is_busy`/`refresh_ttl` +
cooperative stop flag + stale-gate clear). Deliberately blunt — coarse, per-campaign, with an
explicit stop signal — because an autonomous fleet deadlocked on fine-grained locks is worse than
one serialised per campaign.

## Non-goals

No emergent-hive-mind language; no unbounded agent spawning (spawns are budget events —
[[agent-resource-economy]]); no role may edit its own grant; no agent may self-attest
([[confidentiality-lattice-provenance]] and [[user-identity-and-profiles]] carry attribution).

## Production Implementation (2026-10-04)

The collective tribunal is implemented as an automated 4-turn debate pipeline in the Go core orchestrator
(`platform/core/pkg/agents/collective.go:ExecuteTribunal`) backed by `/api/chat/tribunal`:
1. **Turn 1 (Ansatz Scout):** Proposes active space and parameterised ansatz topology with Meyer-Wallach expressibility $Q \ge 0.8$.
2. **Turn 2 (Disconfirmation Officer):** Evaluates classical anchor (e.g. ab-initio PySCF STO-3G/6-31G FCI/RHF) and barren plateau gradient variance $\text{Var}(\nabla E) > 10^{-6}$.
3. **Turn 3 (Device Auditor):** Checks physical constraints on target backend (e.g. $T_2 \le 2T_1$, Clifford canary probe fidelity $\ge 95\%$, spend ceiling $\le \$150.00$).
4. **Turn 4 (Epistemic Scribe):** Collapses Ring 1 credibility wavefunction ($P_{\text{corr}} > 80\%$) and synthesises structured AP2 Mandate Card for client-side cryptographic authorization.

See also [[workflows-and-agent-authoring]], [[quantum-domain-packs]],
[[agentic-memory-and-observability]], [[mcp-and-context-flooding]].
