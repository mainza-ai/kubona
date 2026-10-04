---
type: entity
name: Agent2Agent Protocol (A2A)
date: 2026-10-03
updated: 2026-10-03
tags: [standards, linux-foundation, inter-agent, delegation, a2a]
sources: [source-agent-protocol-landscape-2026]
status: adopted
---

# Agent2Agent Protocol (A2A)

An open protocol under the **Linux Foundation** (originally contributed by Google; Apache-2.0)
enabling communication and task collaboration between autonomous, opaque AI agent applications
(source-agent-protocol-landscape-2026).

## Architecture & Distinctives

While [[mcp-and-context-flooding|MCP]] standardizes agent-to-tool connections, A2A addresses
agent-to-agent collaboration:
- **Opaque Boundaries:** Agents collaborate without exposing internal prompts, memories, tool sets,
  or execution runtimes.
- **Agent Cards:** Standardized endpoint metadata advertising supported capabilities, input/output
  modalities, and trust domains.
- **State Machine:** Formally tracks asynchronous multi-turn tasks (`submitted`, `working`,
  `input-required`, `completed`, `failed`).
- **Consolidation of IBM ACP:** In August 2025, IBM's Agent Communication Protocol (ACP) was
  officially folded into A2A under the Linux Foundation; legacy ACP repositories are archived.

## Role in Kubona

A2A is adopted as Kubona's designated inter-agent delegation protocol (see [[protocol-strategy]]
and [[spend-mandates-and-delegation]]). It governs interactions when research agents spawn or
collaborate with subagents across [[workflows-and-agent-authoring|workflow boundaries]], enforcing
autonomy ceilings without leaking tenant keys.

Related: [[protocol-strategy]] · [[spend-mandates-and-delegation]] · [[workflows-and-agent-authoring]] · [[aaif]]
