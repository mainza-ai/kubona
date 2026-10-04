---
type: entity
name: Agent Payments Protocol (AP2)
date: 2026-10-03
updated: 2026-10-03
tags: [standards, payments, mandates, authorization, security]
sources: [source-agent-protocol-landscape-2026]
status: adopted
---

# Agent Payments Protocol (AP2)

An open-source specification and SDK (Apache-2.0, released September 2025 by Google and industry
collaborators) designed to standardize user authorization and intent for agentic financial transactions
(source-agent-protocol-landscape-2026).

## The Mandate Mechanism

AP2 addresses the authorization crisis of agent commerce: proving that an autonomous agent had
explicit, bounded user permission to incur a financial obligation.
- **Signed Mandates:** Cryptographic credentials issued by a user authorizing a specific agent
  to spend funds under bounded conditions (maximum budget, specific merchant/provider, expiry timestamp).
- **Extension Architecture:** Designed specifically to plug into [[a2a|A2A]], [[mcp-and-context-flooding|MCP]],
  and Universal Commerce Protocol (UCP).

## Role in Kubona

Adopted within [[spend-mandates-and-delegation]]: AP2 provides the structured cryptographic
format for Kubona's [[cost-governance|two-stage approval gate]], ensuring that QPU expenditures
exceeding a tenant's autonomy ceiling carry verifiable, non-repudiable proof of human consent
before dispatching circuits to cloud providers.

Related: [[spend-mandates-and-delegation]] · [[cost-governance]] · [[x402]] · [[protocol-strategy]]
