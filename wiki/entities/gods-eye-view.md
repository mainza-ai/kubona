---
type: entity
name: God's Eye View (bilawalsidhu)
date: 2026-09-27
updated: 2026-09-27
tags: [prior-art, observability, policy, boundaries]
sources: [source-frontier-repo-audit-2026]
status: reference
---

# God's Eye View — policies that a script can check

**https://github.com/bilawalsidhu/gods-eye-view** · MIT · HEAD verified 2026-09-24.
Integration class: **Blueprint** ([[integration-topology-and-licence-discipline]]).

An OSINT-style monitoring app, valuable to Kubona for exactly one reason: its architectural rules
are **machine-checked**, not just written down.

- **Boundaries enforced in CI**: `scripts/check-import-directions.mjs` and
  `scripts/check-package-boundaries.mjs`, documented in `docs/CODE-BOUNDARIES.md` — a layer that
  imports the wrong way fails a build, so the diagram cannot rot.
- **Policy lives with the layer, not in a prompt**: one policy module per domain —
  `src/layers/{traffic,vessels,launches,flights,radio,localAdsb}/policy.js`.
- **User-owned endpoints, guarded**: `server/providers/local-receivers.js` admits user-supplied
  local endpoints from an **env list**, applies an `isLocalIpv4` guard (:146, :295), and lets the
  feed's own clock decide staleness (:376) — see `docs/LOCAL-RECEIVERS.md`. The trust decision is
  enumerated, and freshness is asserted by the source that owns it
  ([[agentic-memory-and-observability]], [[confidentiality-lattice-provenance]]).

The transfer is structural: Kubona's equivalent is a `check-import-directions`-style gate over
`platform/kubona/` layers plus one `policy.py` per domain, so
[[workflows-and-agent-authoring]] and [[multi-agent-research-collective]] boundaries stay true
after the author moves on.