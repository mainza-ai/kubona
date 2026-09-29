---
type: entity
name: Cloudflare
date: 2026-08-16
updated: 2026-09-27
tags: [vendor, economy, infrastructure]
sources: [source-cloudflare-economy]
status: tracked
---

# Cloudflare

The vendor that shipped the first pragmatic infrastructure for the **agent resource
economy** (source-cloudflare-economy).

## What Cloudflare published

| Product | Function |
|---|---|
| **AI Crawl Control** | Visibility and control over which AI crawlers touch site content |
| **Pay Per Crawl** | Direct monetization of crawler access (payment intent up front, or HTTP 402 quote) |
| **Monetization Gateway** | Pay-per-access generalized to *any* resource: page, dataset, API, MCP tool call, file, search index |
| **x402 rail** | See [[x402]]. Request → price quote (often sub-cent) → agent pays → retry with proof → edge verifies before origin |
| **AI Index** | Making sites agent-consumable via MCP, `llm.txt`, search APIs, bulk data APIs |

## Thesis it substantiates

- The old human-web bargain (crawler → human visit → monetization) is breaking; the agent
  web monetizes **useful resources**, not attention.
- Sub-cent micropayments work because machines tolerate per-request fees and agents gain
  wallets + identities.
- This is the external validation for Kubona's [[x402]] adoption and algorithm-library
  metering (see [[agent-resource-economy]]).