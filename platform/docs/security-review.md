# Kubona Gateway — Security Review Package (Phase 3)

Prepared for the third-party gateway security review (roadmap P3 gate). The review
checks the controls below against the threat model from wiki/concepts/security-and-pqc.

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
| 8 | Read/approve-only console surface (no submission path) | `kubona/console_api.py` (no job endpoints) | implemented + tested |
| 9 | Sandboxed execution boundary (local simulators; optional deps fail closed) | provider trait `submit()`; plugins raise `ProviderError` without deps/credentials | implemented |
| 10 | Canary CI to catch provider drift before agents hit it | `kubona/canary/` (nightly CLI) | implemented + regression test |

## Review checklist (for the third-party pass)

- [ ] ML-KEM-768 usage matches FIPS 203 test vectors (KAT) — verify via the
      `cryptography` package's own test suite (v50 ships the KEM).
- [ ] Hybrid KDF domain separation: `_KEM_CONTEXT` binding reviewed; no secret
      material shared outside the two-party handshake.
- [ ] AES-GCM nonce policy: fresh 96-bit random nonce per message; collision risk
      quantified (negligible for expected volumes).
- [ ] `PolicyEngine.decide` is invoked on **every** gateway call path (no bypass via
      console REST API, which is separately scoped and has no submit tools).
- [ ] Delegation lifecycle: expiry enforced, revocation immediate, no cross-lab
      leakage (identity registry is per-store).
- [ ] Ledger: hash chain validates on every read path used by the console/provenance
      resource; audit writes can never break the call path (fail-open to in-memory log).
- [ ] Secret handling: dev tokens/keys are placeholders; real credentials never enter
      the browser or the console API (provider SDKs hold them server-side).
- [ ] CORS `*` on the console API is dev-only — tighten to the console origin in
      deployment (Phase-3 hardening item).
- [ ] Rate limiting on auth failures and approval actions (hardening item).

## Known gaps (deferred, documented)

- Real OAuth 2.1 IdP + short-lived scoped tokens (verifier stub stands in).
- TLS termination / ML-KEM on the wire at the HTTP gateway (the hybrid transport is
  demonstrated as a library; wiring it into the gateway HTTP layer is deployment scope).
- Hardware-security-module / key custody for provider credentials.
