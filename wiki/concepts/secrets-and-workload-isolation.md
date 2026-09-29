---
name: Secrets and Workload Isolation
type: concept
tags: [frontier, security, secrets, sandbox, supply-chain, governance]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026, source-architecture-paper]
---

# Secrets and Workload Isolation

**Definition.** The layer that makes autonomous research *allowed to exist*: cloud QPU
credentials never enter an agent's context, agent-authored code never runs in the platform
process, and every privileged action is attributable to a signed identity. Two rails —
**secrets** and **workloads** — with one invariant: *the agent that wants a capability is never
the component that holds it.*

## Why this is a frontier requirement, not hygiene

An autonomous loop that can compile, submit, and spend on real hardware is a credential-theft
target by construction. The risk is demonstrated locally, in this repo's own working copies:
`dev-docs/gather_insights_from/mainza-consciousness/.git/config` carries a `ghp_…` **plaintext
PAT in the remote URL** — readable by any tool that reads `.git/config`, echoed into logs and
IDE panes, and inherited by any clone made from that URL ((repo-audit-2026); logged in
`wiki/concepts/risks.md`). That clone is a gitignored, throwaway reference copy, so nothing was
published, and re-checking it against the API on 2026-09-27 returned **HTTP 401**: the token is
already dead, which is luck, not design. The live case is one layer down and worse in kind:
`credential.helper=store` globally leaves a **live** `gho_…` OAuth token (`repo`, `workflow`) in
plaintext at `~/.git-credentials`, left over from an older `gh` login that the keyring no longer
uses. Neither is a Kubona bug; both are the exact failure the platform must make impossible for
itself and for its operators — a helper that writes secrets into a file is the same defect as an
agent that pastes a token into a config. Kubona must be *structurally unable* to do this to itself:
credentials unreachable from agent context, not merely "not sent", and never persisted by an
ambient tool outside the vault.

## The secrets rail: OpenBao as sidecar

`openbao` (MPL-2.0, Go) ships the engines needed, all in-tree under
`internal/builtin/logical/`: **kv** (versioned credentials), **transit** (encryption-as-a
service — sign/verify without the key leaving the box), **pki** + `pkiext` (mTLS identity for
every component), **kubernetes** and role-based auth (short-lived, workload-scoped), and
`internal/audit` writing hash-chained audit trails — the same append-only philosophy as the
Kubona ledger ([[provenance-and-reproducibility]]).

| Property | Consequence for Kubona |
|---|---|
| Transit: keys never exit | a provider key is used by *API call*, never read by an agent |
| Short-lived workload roles | an agent's token expires with its job, not with a human's patience |
| Namespaces | per-tenant isolation for the multi-tenant plan in [[qflow-studio]] |
| Agent with in-memory cache | repeated submissions don't re-auth; cache TTL bounds blast radius |
| `paperclip` plugin = secret-provider adapter (MIT; `doc/files/plugin-manifest.md`) | the shape Kubona adopts: a plugin supplies context and/or credentials, and its manifest declares which |

Topology: **sidecar + external binary** (`bao` CLI never vendored), MCP-exposed as
`secret_lease(scope, ttl)` returning single-use leases. Default-deny; grants are signed policy
objects. See [[user-identity-and-profiles]] for identity, [[security-and-pqc]] for posture.

## The workload rail

Agent-authored code is **untrusted input that happens to be written by our own agent.**

1. **Declarative** (no code): parameter sweeps, pipeline specs, domain-pack workflows →
   interpreted, never executed. Preferred default.
2. **Constrained Python**: import-whitelisted execution with CPU/wall/memory caps, following
   `milimoquantum/backend/app/quantum/sandbox.py:104` `ALLOWED_MODULES` and
   `execute_with_custom_function` (`:147`) — proven in this family (Blueprint: unlicensed,
   (repo-audit-2026)).
3. **Full sandbox** (container/gVisor, egress denied) for third-party skills, with the runtime
   declared by a **toolset manifest**: `runner-images` ships that pattern at
   `images/ubuntu/toolsets/toolset-2404.json` (language versions, pinned tools, arch). Kubona
   emits one per job, so *environment* is provenance rather than folklore.

Two cheap habits from `reverse-skill` (MIT): a **route-before-act contract** ("先路由后动手" —
route → declare a PRIMARY path → `case-init` writes `scope.md` → *no ACT while auth is not
granted* → named lead + specialist roles → tools only from a declared tool-index), and
**`case-guard`, which exits 2 and whose `-Force` flag explicitly cannot bypass it**. That is the
property autonomous action needs: an escape hatch documented as not an escape hatch.

For inbound *feeds*, adopt `gods-eye-view`'s `local-receivers` discipline: user-owned endpoints
only from an env allow-list, `isLocalIpv4` validation, connect/response timeouts, body caps,
staleness windows, and an error enum that never leaks a host or port. User instruments become
first-class providers under [[provider-capability-schema]] without becoming an SSRF surface.

## Supply chain and skills

`reverse-skill` keeps a `skill-supply-chain` policy doc; `gods-eye-view` keeps
`tools/skill-evidence-audit/` with ~30 verification scripts; `paperclip` ships a **low-trust
preset** *and an adversarial eval suite* for its own plugin boundary
(`evals/paperclip-plugin-braintrust`). Kubona adopts all three: skills pinned by hash,
capability manifests audited mechanically, and low-trust mode exercised in CI — an untested
hardening mode is theatrical hardening. See [[mcp-and-context-flooding]] for the context-side
analogue.

## Gate tests

(a) No MCP response body may contain a lease secret or provider key (redaction test over
recorded fixtures). (b) A sandboxed snippet importing a non-whitelisted module fails with an
audited refusal; egress denied by default. (c) The `case-guard` analogue exits non-zero with the
grant unset, even when forced. (d) A CI job runs the low-trust mode. (e) A repo scanner fails
the build on credential-bearing remote URLs — the direct response to the finding above.
