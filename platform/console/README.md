# Kubona Observatory Console

Sovereign discovery console: icon dock, living arena, native co-pilot. Next.js 16 + React 19 + Tailwind v4 + vitest.

## Run it (local test stack)

The console was built against the **Go core** — not the Python bridge (different shapes: SSE events, `session_id` fields).

```bash
# 1. Backend — Go orchestrator on :8080
go build -o /tmp/kubonad ./cmd/kubonad   # from platform/core
/tmp/kubonad -port 8080

# 2. Frontend — from this directory (dev token required: the bridge/core
#    fail closed without `Authorization: Bearer`; unset in production)
NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN=dev-token pnpm dev --port 3000
```

Open http://localhost:3000. Optional: local oMLX LLM on `:8787` powers chat/tribunal replies.

## Gates

```bash
pnpm run build   # production build (required green)
pnpm test        # vitest suite
pnpm lint        # eslint, zero warnings
```

## Test it

Ten adopted co-pilot scenarios (local simulation only, $0.00, no QPU):
[`wiki/concepts/local-simulation-test-scenarios.md`](../../wiki/concepts/local-simulation-test-scenarios.md).
Golden rule: if the Action Gate appears during testing, deny it or leave it pending.

## Layout

- `src/app/page.tsx` — shell (dock + stage + co-pilot wiring, sessions, gate state)
- `src/components/sovereign/` — Dock, ArenaHud, SwarmUniverse, ProofDagView, ActionGate, WidgetStage (+ tests)
- `src/components/CopilotDrawer.tsx` — chat/tribunal logic in co-pilot skin
- `src/components/surfaces/` — prior instruments (recorder, manifold, fleet, proofs, mesh…)
- `src/lib/api.ts` — backend client (Bearer attached only when the dev token env is set)
