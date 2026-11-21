---
name: kubona-connect
description: Connect external AI agents to Kubona over MCP Streamable HTTP (/mcp), unified stdio (serve-mcp), legacy /rpc, Go HTTP API (:8080), or A2A (/api/a2a/v1), including authentication and client configs.
---

# kubona-connect: Transport, Auth & Client Setup

Version 2026-11-20. Use this skill when configuring an AI agent host (Claude
Code, Claude Desktop, Cursor, Codex, Antigravity, or custom HTTP/A2A clients)
to connect to Kubona.

## Transport Matrix
| Transport | Endpoint / Command | Protocol | Auth | Best For |
|---|---|---|---|---|
| **MCP Streamable HTTP** (recommended) | `http://127.0.0.1:8123/mcp?profile=science` | Standard MCP JSON-RPC 2.0 (`stateless_http`, `json_response`) | `Authorization: Bearer <token>` | Shared Docker stack, multi-agent setups, Claude Code / Cursor / Antigravity |
| **MCP Unified stdio** | `platform/.venv/bin/python -m kubona.run serve-mcp --profile science` | Standard MCP over stdio | `KUBONA_MCP_TOKEN` or `KUBONA_API_TOKEN` (or `--no-auth` local dev) | Local single-agent IDE sessions |
| **Legacy HTTP RPC** | `POST http://127.0.0.1:8123/rpc` | `{server, tool, args}` or `{envelope}` (X25519+ML-KEM) | `Authorization: Bearer <token>` | Custom scripts or post-quantum hybrid envelope callers |
| **Go HTTP + SSE API** | `http://127.0.0.1:8080/api/...` | REST JSON + SSE `/api/events/stream` (`Last-Event-ID`) | Reads public (rate-limited); writes require `Bearer $KUBONA_API_TOKEN` | Full 188-path OpenAPI surface, console parity, streaming |
| **A2A Delegation** | `POST http://127.0.0.1:8080/api/a2a/v1` | Agent-to-Agent + x402 payment proof | `X-Kubona-A2A-Token` / Bearer + `GET /api/a2a/v1/challenge` | Inter-agent delegated paid tasks |

## Context-Flooding Profiles
Pass `?profile=<name>` on `/mcp` or `--profile <name>` on `serve-mcp`:
- `read-only` — strictly non-mutating inspection, cost estimates, anchors, and memory recall.
- `science` — `compiler/*`, `execution/*`, `science/*`, cost estimates/sessions, provenance.
- `governance` — `cost/*`, `control/*`, `provenance/*`, session autonomy, cancel/close.
- `ops` — `control/*`, job status/cancel, provider registry, burn rate, provenance read.
- `world` — `world/*` Observatory tools + provenance read.
- `all` — all 49 tools across all 7 servers (`compiler`, `control`, `cost`, `execution`, `provenance`, `science`, `world`).

Unknown profile names fail closed with HTTP `400`.

## Generate Client Configs (Zero Secret Leakage)
Run from the repo root to emit copy-pasteable configs with `${KUBONA_API_TOKEN}` placeholders:
```sh
platform/.venv/bin/python -m kubona.run mcp-config --client claude-code --transport http --profile science
platform/.venv/bin/python -m kubona.run mcp-config --client cursor --transport http --profile science
platform/.venv/bin/python -m kubona.run mcp-config --client codex --transport http --profile science
platform/.venv/bin/python -m kubona.run mcp-config --client antigravity --transport http --profile science
```

## Runtime Discovery Endpoints (`kubonad :8080`)
- `GET /llms.txt` — plain-text agent entrypoint with live links.
- `GET /api/openapi.json` — full OpenAPI 3.1 contract for all modeled routes.
- `GET /.well-known/agent.json` — A2A Agent Card advertising `/api/a2a/v1` and capabilities.
- `GET /health` on `:8123` — minimal gateway liveness probe (`{"ok": true, "service": "kubona-http-gateway"}`).

## MCP-Native Onboarding Primitives
Every unified MCP connection exposes:
- **Resources:** `kubona://skills`, `kubona://skills/{name}`, `kubona://contracts/error-catalog`, `kubona://contracts/mcp-catalog`, `kubona://contracts/experiment-kinds`, `kubona://contracts/rate-cards`.
- **Prompts:** `vqe-with-anchor`, `hardware-readiness-check`, `cost-governed-submit`, `audit-session-provenance`, `debug-failed-job`.

## Auth Traps
- Missing or invalid `Authorization: Bearer <token>` on `/mcp` or `/rpc` returns `401` (`unauthorized`).
- On `kubonad :8080`, writes require `KUBONA_API_TOKEN`; reads stay public behind rate limiters (`429` `rate_limited`).
- Never pass a client-forged `caller` argument to MCP tools — `KubonaGateway` overwrites `caller` server-side from the verified token (`platform/kubona/mcp/gateway.py:121`).
- In Docker Compose (`platform/docker/compose.prod.yml`), the `mcp` service reaches `kubonad` via `KUBONA_CORE_URL=http://kubonad:8080`; local stdio reaches `http://127.0.0.1:8080`.
