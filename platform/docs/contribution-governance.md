# Contribution Governance — the Tool-Surface Security Boundary (Phase 4)

Deferred by design until a working core exists (\u00a79); this doc ships with the
plugin SDK. Target: first third-party provider integration merged (Phase-4 gate).

## The boundary

- **The MCP tool surface is security-sensitive** (credentials, spend, approvals).
  Contributions to the *core* tool surface follow a stricter bar than
  contributions to *providers* (which are data + an isolated plugin).
- **Providers are data + plugin.** A provider contribution touches
  `kubona/schema/providers/<name>.json` (capability data) and an isolated plugin
  module. It cannot modify gateway, cost-governance, provenance, or auth code.
- **Sandbox-first:** every provider PR must include a sandbox smoke-test result
  (see `kubona.sdk.register_provider(sandbox_test=True)`); real-hardware
  credentials are never part of the repo.

## Review process

1. Contributor runs `register_provider(...)` against their capability JSON +
   plugin; commits the generated schema file + plugin + a passing canary for
   their provider (or a documented "unreachable without credentials" flag).
2. Maintainer review checklist:
   - [ ] schema valid (pydantic), no pricing/TP misinformation
   - [ ] plugin imports cleanly without credentials; fails closed otherwise
   - [ ] no changes outside `schema/providers/` + the plugin module
   - [ ] canary result recorded in the ledger (kind="canary")
   - [ ] license of any vendored SDK is OSI-approved (open-source-first rule)
3. Merge requires the correctness-regression harness green on the canary tier
   (the \u00a711.5 release gate).

## Extensibility validation (Phase-4 gate)

A third-party provider merged through this SDK is the gate's proof: onboarding
is a data change, not a core rewrite.
