"""MCP layer: six federated servers + gateway (K5 role clarification).

Role: this package is the MCP *edge* for external MCP clients —
compiler, execution, cost-governance, eval/provenance, control servers
speak stdio (`python -m kubona.mcp.servers.<name>`) and Streamable HTTP
(`kubona serve-gateway`), per MCP spec 2026-07-28. It is NOT the
console's backend (the Next.js console talks only to Go :8080) and NOT
the job execution path (the Python bridge serves gRPC :50051; the Go
runner dispatches). Capability logic lives in protocol-neutral core
modules; these servers are thin adapters (see [[protocol-strategy]]).
"""
__version__ = "0.1.0"
