---
type: entity
name: OpenBao
date: 2026-09-27
updated: 2026-09-27
tags: [secrets, infrastructure, sidecar, mpl-2.0]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# OpenBao — the secrets sidecar

**https://github.com/openbao/openbao** · **MPL-2.0**. Integration class:
**Sidecar + external binary** — its own process, its own storage, reached over the network; never
linked into the Apache-2.0 core ([[integration-topology-and-licence-discipline]]).

The value is that the hard parts are already engines, not modules we would rewrite:
`internal/builtin/logical/{kv,transit,pki,kubernetes,database,ssh,totp}` plus `internal/audit`.
Provider tokens for cloud QPU access, per-agent credentials and rotation policies are all
ordinary workloads there ([[secrets-and-workload-isolation]], [[security-and-pqc]]).

> **Licence note — the rule that shapes the topology.** MPL-2.0 is *file-level* weak copyleft:
> a modified MPL file must be shared under MPL, while unmodified files and separately-licensed
> consumers are unaffected. Vendoring OpenBao code into Kubona would be legal but would tag those
> files MPL; running it as a sidecar tags nothing. Sidecar is therefore the licence-clean choice,
> and it is also the operationally correct one (audit log, separate blast radius, independent
> restart) — see [[integration-topology-and-licence-discipline]].

The `ghp_…` incident in [[mainza-consciousness]] is the counter-example this integration exists to
make impossible: a live PAT lived in a *remote URL*, which is exactly what a vault with scoped,
expiring tokens and an audit trail exists to prevent.