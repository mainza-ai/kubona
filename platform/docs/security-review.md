# Kubona Gateway — Security Review Package (Phase 3)

> Re-baselined 2026-10-10 against the tripartite tree (Go `platform/core`,
> Python mesh + bridge, Next.js console). Rows marked *(new since draft)*
> postdate the original package; nothing below is aspirational — every
> "implemented" claim names code + tests.

Prepared for the third-party gateway security review (roadmap P3 gate). The review
checks the controls below against the threat model from wiki/concepts/security-and-pqc.
Suggested scoping yardstick: **OWASP ASVS Level 2** (recommended for most
applications; L1 is pentest-only theater for this architecture, L3 is
military-grade overkill pre-revenue) **plus a cryptography track** (ASVS V6/V11:
algorithm selection, key management, randomness, fail-secure defaults). Require a
**fix-reverification cycle** in the engagement terms.

## Threat model

1. **Credential theft / MCP compromise** — a compromised agent session can exfiltrate
   circuit IP and run up five/six-figure QPU bills (the documented 2026 attack class).
2. **Harvest-now-decrypt-later** — encrypted traffic today is stored for decryption
   once fault-tolerant hardware exists; the platform's own outputs (optimized circuits,
   algorithm results) are high-value targets.
3. **Privilege escalation** — an agent (or agent-created agent) gains capabilities its
   owner never granted (delegation forgery, scope creep, tool misuse).
4. **Tampering with evidence** — provenance/audit records modified to hide spend or
   falsify results.
5. **Denial / abuse of governance** — approvals bypassed, ceilings evaded, loops killed
   or spawned without authority.

## Controls implemented (map control -> code)

| # | Control | Where | Status |
|---|---|---|---|
| 1 | Bearer auth boundary on every gateway call | `kubona/mcp/gateway.py` (`require_auth`, `DevAuthProvider`) | implemented (dev); OAuth 2.1 IdP = Phase-3 hardening |
| 2 | **Per-call identity-aware authorization** (fine-grained scopes, no coarse roles) | `kubona/security/policy.py` (`TOOL_SCOPES`, `PolicyEngine.decide`) | implemented + tested |
| 3 | Agent delegation with **live revocation** (NIP-OA pattern) | `kubona/identity/registry.py` (`access()` checks per call) | implemented + tested |
| 4 | **Hybrid PQC transport** (X25519 + ML-KEM768, HKDF-combined, AES-256-GCM) | `kubona/security/hybrid_transport.py` | implemented + tested |
| 5 | Cryptographic agility (swappable suites) | transport as a swappable suite; suite id in handshake | implemented since Phase 0 |
| 6 | **Immutable audit** — every tool call/approval/spend as a content-addressed, hash-chained record | `kubona/mcp/gateway.py` (`_ledger.append("audit")`) + `kubona/provenance/ledger.py` | implemented + tested |
| 7 | Cost governance: pre-flight, hard ceilings, two-stage approvals | `kubona/cost/governance.py` | implemented + tested |
| 8 | Console surfaces are oversight-first; spend authorization is a signed mandate validated server-side, never an ambient bearer | `platform/console/src/lib/mandates.ts` + `platform/core/pkg/api/server.go` (`/api/mandates/verify`, bilateral TS/Go vector test) | implemented + tested |
| 9 | Sandboxed execution boundary (local simulators; optional deps fail closed) | provider trait `submit()`; plugins raise `ProviderError` without deps/credentials | implemented |
| 10 | Canary CI to catch provider drift before agents hit it | `kubona/canary/` (nightly CLI) | implemented + regression test |
| 11 | **Lab tenants + autonomy ladder (L0–L3)** with live grant/revoke at every dispatch entry; session≡tenant conflation removed | `platform/core/pkg/tenants/`, `jobs/admission.go`, A2A runTask checker | implemented + tested *(new since draft)* |
| 12 | **Pre-QPU admission**: autonomy → input-label confinement → fidelity floor → mandate/grant receipt, enforced inside `SubmitJob` on every entry path | `platform/core/pkg/jobs/admission.go`, `FidelityGate.EvaluateAdmissionFloorOnly` | implemented + tested *(new since draft)* |
| 13 | **UCAN 1.0 cross-lab grants** (mint/attenuate/verify, ledger CID revocation registry, did:key from mandate keys) + register/revoke endpoints + CLI | `platform/core/pkg/grants/`, `pkg/api/grants.go`, `cmd/kubonad/grants.go` | implemented + tested *(new since draft)* |
| 14 | **Confidentiality labels**: lattice codec + join, labels on ledger writes, label-gated export, Python ledger labels with tamper-bound hashes, pre-serialization query filter | `platform/core/pkg/ifc/`, `pkg/ledger/`, `kubona/provenance/ledger.py`, `mcp/servers/provenance.py` | implemented + tested *(new since draft)* |
| 15 | **MRTR approval retry** (form elicitation, HMAC state, single-use nonces) + burden-scaled approval packet card | `kubona/mcp/approval.py`, gateway `require_approval`, `ApprovalPacketCard.tsx` | implemented + tested *(new since draft)* |

## Review checklist (for the third-party pass)

- [ ] ML-KEM-768 usage matches FIPS 203 test vectors (KAT) — verify via the
      `cryptography` package's own test suite (v50 ships the KEM).
- [ ] Hybrid KDF domain separation: `_KEM_CONTEXT` binding reviewed; no secret
      material shared outside the two-party handshake.
- [ ] AES-GCM nonce policy: fresh 96-bit random nonce per message; collision risk
      quantified (negligible for expected volumes).
- [ ] `PolicyEngine.decide` is invoked on **every** gateway call path (no bypass via
  console REST API; the Go rail enforces autonomy/admission independently of the
  Python policy engine — verify both, and their parity).
- [ ] Delegation lifecycle: expiry enforced, revocation immediate, no cross-lab
  leakage (identity registry is per-store); UCAN chains verify to trusted lab
  admin DIDs with ledger revocation consulted at dispatch.
- [ ] Delegation lifecycle: expiry enforced, revocation immediate, no cross-lab
      leakage (identity registry is per-store).
- [ ] Ledger: hash chain validates on every read path used by the console/provenance
      resource; audit writes can never break the call path (fail-open to in-memory log).
- [ ] Secret handling: dev tokens/keys are placeholders; real credentials never enter
      the browser or the console API (provider SDKs hold them server-side).
- [ ] CORS `*` on the console API **and** the Go gateway is dev-only — tighten to the console origin in
  deployment (Phase-3 hardening item; live today in `console_api.py:853` and `pkg/api/server.go:206`).
- [ ] Rate limiting on auth failures and approval actions (hardening item).

## Known gaps (deferred, documented)

- Real OAuth 2.1 IdP + short-lived scoped tokens (verifier stub stands in).
- TLS termination / ML-KEM on the wire at the HTTP gateway (the hybrid transport is
  demonstrated as a library; wiring it into the gateway HTTP layer is deployment scope).
- Hardware-security-module / key custody for provider credentials.

## Outreach brief (models, ranked)

1. **Boutique audit firm, 2–3 week time-box** (Trail-of-Bits pattern): scoping call →
   threat model → code review → **fix verification included in terms** (highest-ROI
   clause — negotiate it upfront). Send: this package + architecture one-pager +
   bilateral mandate vector + gate suite (`go test ./...`, `pytest`, build).
2. **Academic security lab**: slower, cheaper, deeper on the novel parts (IFC lattice,
   mandate/grant semantics, UCAN revocation model).
3. **Quantum-provider security team**: narrow scope to the provider edge (credential
   handling, job dispatch) — providers welcome reviews of integrations that drive
   hardware usage.

Time outreach to land **after** the current implementation batch merges, so reviewers
read the real system (this package is versioned against tree state — re-baseline it
before sending).
