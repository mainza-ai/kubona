---
type: entity
name: Agentic AI Foundation (AAIF)
date: 2026-10-03
updated: 2026-10-03
tags: [standards, linux-foundation, governance, mcp]
sources: [source-agent-protocol-landscape-2026]
status: tracked
---

# Agentic AI Foundation (AAIF)

An open-source directed foundation launched under the **Linux Foundation** on 2025-12-09
to steward vendor-neutral standards for autonomous AI agents (source-agent-protocol-landscape-2026).

## Scope & Anchor Projects

AAIF was formed to prevent fragmentation across the agent tooling and execution layers.
Key founding contributions include:
- **Model Context Protocol (MCP):** Universal standard connecting LLMs and AI agents to external
  tools, APIs, and data resources.
- **goose:** Open-source agent runtime framework.
- **AGENTS.md:** Open convention for structuring agent governance, operational rules, and project
  instructions (the exact format Kubona implements in `AGENTS.md`).

## Role in Kubona

Kubona tracks AAIF specifications to ensure its [[mcp-and-context-flooding|MCP edge adapters]]
and documentation adhere strictly to vendor-neutral standards, protecting against proprietary
lock-in from individual model providers.

Related: [[mcp-and-context-flooding]] · [[protocol-strategy]] · [[a2a]]
