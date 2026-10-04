---
name: Agent Protocol Landscape & Standards Audit (Oct 2026)
type: source
tags: [audit, protocols, mcp, a2a, aaif, ap2, standards, governance]
date: 2026-10-03
updated: 2026-10-03
status: source
origin: first-party primary-source specification & repository audit (2026-10-03)
---

# Agent Protocol Landscape & Standards Audit — Oct 2026 (`agent-protocol-landscape-2026`)

**Evidence class.** First-party maintainer inspection of primary specifications, official
foundation announcements, and live repositories as of 2026-10-03. Anchors resolve against
published specification drafts, official GitHub repositories, and release registries.

## What this source covers

An exhaustive audit of the 2026 agentic protocol and standards ecosystem, evaluating
where [[mcp-and-context-flooding|MCP]] serves as a viable foundation, its operational
limits, and complementary standards for inter-agent collaboration, payment authorization,
and vendor quantum tooling.

### 1. Model Context Protocol (MCP) — Spec 2026-07-28 & Python SDK 2.x
- **Stateless Core (`SEP-2575`):** Protocol-level sessions and `Mcp-Session-Id` header
  eliminated from Streamable HTTP. Every request carries its protocol version and client
  capabilities in `_meta`. Cross-call state is handled via explicit server-minted handles.
- **`server/discover`:** Mandatory RPC for servers to advertise supported versions and capabilities.
- **Multi-Round-Trip Requests (MRTR, `SEP-2322`):** Servers return `resultType: "input_required"`,
  prompting the client to retry with `inputResponses`. Replaces server-initiated requests.
- **Tasks Extension (`SEP-2663`):** Experimental tasks moved out of the core protocol into
  the official extension `io.modelcontextprotocol/tasks` (`tasks/get` polling, `tasks/update`).
- **Deprecations:** Roots, Sampling, and Logging deprecated. SSE stream resumability removed.
- **Python SDK 2.x:** Rebuilt engine; `FastMCP` replaced by `MCPServer` (`mcp.server.mcpserver`).
  Speaks protocol version `2026-07-28`. Requires `httpx2>=2.10.0`. Validates `x-mcp-header`.

### 2. Linux Foundation / Agentic AI Foundation (AAIF)
- Formed 2025-12-09 under the Linux Foundation (`https://aaif.io`).
- Anchored by open-source contributions including MCP, `goose`, and `AGENTS.md`.
- Establishes vendor-neutral open governance for agent tool and capability interfaces.

### 3. Agent2Agent Protocol (A2A) & IBM ACP Consolidation
- Contributed by Google to the Linux Foundation; Apache-2.0; active repository (`a2aproject/A2A`).
- Focus: Interoperability between opaque agentic applications ("agents as agents, not just tools").
- **IBM ACP Absorption:** IBM's Agent Communication Protocol (`i-am-bee/acp`) officially merged
  into A2A under the Linux Foundation; legacy ACP repository archived.

### 4. Agent Payments & Commerce: AP2, ACP, MPP, x402
- **AP2 (Agent Payments Protocol):** Google & partners (Sept 2025; Apache-2.0; `google-agentic-commerce/AP2`).
  Solves authorization and intent via cryptographically signed mandates ("OAuth for payments").
  Explicitly designed as an extension for A2A and MCP.
- **ACP (Agentic Commerce Protocol):** OpenAI & Stripe (`agentic-commerce-protocol`). Standardizes
  buyer-to-merchant cart and checkout flows. *Distinct from IBM's legacy inter-agent ACP.*
- **MPP (Machine Payments Protocol):** Stripe & Tempo (`tempoxyz/mpp-specs`, `mpp.dev`). Revives HTTP 402
  with `WWW-Authenticate: Payment` and session streaming for programmatic machine access.
- **x402:** Moved from Coinbase to `x402-foundation/x402` (Apache-2.0). Internet-native settlement standard.

### 5. Vendor Tooling: IBM Qiskit MCP Servers
- Official repository `Qiskit/mcp-servers` (Apache-2.0, pushed 2026-09-25).
- Exposes 5 servers: `qiskit` (transpile/analyze), `qiskit-ibm-runtime` (cloud jobs),
  `qiskit-ibm-transpiler` (AI transpile), `qiskit-docs`, `qiskit-gym`.
- **Architectural limits:** Passes raw API tokens (`QISKIT_IBM_TOKEN`), contains zero cost
  ceilings or pre-flight balance estimation, zero classical anchors, and is vendor-locked to IBM.

### 6. Context Efficiency: Code Execution vs. Tool Definitions
- **Anthropic Engineering (2025-11-04):** Code execution via MCP reduces context overhead by up to
  **98.7%** by presenting servers as typed APIs rather than loading schemas into context.
- **Cloudflare Code Mode:** Exposing 2,500 endpoints as tool definitions consumes >2M tokens;
  code mode collapses this into a dynamic search/execute interface using ~1,000 tokens.

## Platform impact

- Establishes [[protocol-strategy]]: MCP is an edge adapter over a protocol-neutral capability core.
- Defines [[spend-mandates-and-delegation]]: AP2 for spend authorization, A2A for inter-agent delegation.
- Identifies Kubona's strategic role above vendor MCP suites ([[qiskit-mcp-servers]]).
