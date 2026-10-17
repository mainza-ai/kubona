---
type: entity
name: Cloudroom
date: 2026-10-17
updated: 2026-10-17
tags: [platform, agents, sandbox, prior-art, cloud-agents, rust, electron]
sources: [source-frontier-repo-audit-2026]
status: reference
---

# Cloudroom

Open-source platform for running coding agents (Codex, Claude Code, Pi, Cursor,
OpenCode) each in its own sandbox, on your Mac or in the cloud, on your own
model logins. Two repos, both Apache-2.0:

- `davidondrej/cloudroom-core` — Rust runtime (one service per machine), HEAD
  `0735d68` (2026-10-15). → `https://github.com/davidondrej/cloudroom-core`
- `davidondrej/cloudroom-gui` — desktop app (Electron + Vite/React monorepo),
  HEAD `3e4b4bc` (2026-10-16). → `https://github.com/davidondrej/cloudroom-gui`

> Method note: read as prior art for *mechanisms*, never as supply
> ([[integration-topology-and-licence-discipline]]: reference class). Every
> claim below anchors to a live `path:line` in those two repos at the SHAs
> above. What Kubona already does differently is stated where it matters.

## What it is (3 pieces)

1. **Core** (Rust, `cloudroom-core`): owns processes, durable request queue,
   journal recording, replay/recovery, workspaces, sync, previews, Mac two-way
   access. Harnesses own their agent loop, tools, inference, retries,
   compaction (`docs/harnesses.md:3`). One package, one service (`AGENTS.md:3`).
2. **GUI** (`cloudroom-gui/gui`): pnpm + Turbo monorepo; Vite/React web UI
   (`apps/app`), thin Electron shell (`apps/desktop`), central server
   (`apps/server`, SQLite + Drizzle), per-machine executor (`apps/host-daemon`),
   scriptable CLI (`apps/cli`, bin `room-cli`), 39 `packages/` incl. wire
   contracts (`server-contract`, `host-daemon-contract`,
   `desktop-contract`).
3. **Web app** (closed, invite-only hosted): stores chat history +
   diagnostics; self-hosted cores keep history on local disk
   (`README.md:29`, releases note `core-v0.1.56`).

## Session + receipt model (the part most worth copying)

- **202 means saved, not finished.** Every `POST` returns
  `{session_id, receipt, saving}`; receipts move
  `accepted → delivered → completed | interrupted | failed | unknown |
  unknown_after_restart` (`docs/api.md:8-11`,
  `src/session/mod.rs:1205-1428`). Uncertain work is never resent.
- **`request_id` idempotency with content binding.** Every command carries a
  `request_id`; retrying it returns the same receipt and never repeats work;
  reusing an ID for *different* content returns `request_conflict`
  (`docs/api.md:9`, `src/session/mod.rs:562-574`). Session IDs derive from
  it (`cr_` + start request ID).
- **Single-turn durable queue.** Prompts arriving mid-turn queue in acceptance
  order; immediate dispatch only if idle + ready + empty + unpaused
  (`src/session/mod.rs:1188-1229`); `edit` / `cancel` / `reorder` operate on
  queued items with revision checks
  (`src/session/mod.rs:1066-1133`, `src/session/prompt.rs:48-87`).
- **`steer`:** inject guidance into the *running* turn where the harness
  supports it; otherwise it runs as the next turn
  (`docs/api.md:39`). Queued fallback is the same pattern used for
  child-completion notices.
- **Sleep:** 30 min idle → harness process stopped, state `sleeping`, history
  on disk; a running turn, queued work, busy children, pending interrupts, or
  *new background processes* keep it awake
  (`src/session/sleep.rs:4-6,66-93`). Next prompt / teleport / resume wakes
  the same native conversation. Restart relaunches only sessions with queued
  work or recent activity — a reboot never refills memory
  (`src/session/sleep.rs:55-61`).
- **Recovery preflight + explicit retry.** `GET …/recovery` returns
  `ready | empty | missing | unavailable` (read-only); a failed resume
  exposes `startup_error` and is retried only with a *new* `request_id`
  after cleanup + preflight — never replays completed/failed/uncertain
  prompts (`docs/session-lifecycle.md:44-50`).
- **Crash rule:** unfinished dispatched work becomes `unknown` /
  `unknown_after_restart` and is never resent; one auto-recovery attempt,
  re-armed only by a healthy completion
  (`src/session/mod.rs:1764-1766`, `docs/session-lifecycle.md:38-40`).
- **Rewind / compact / fork:** idle-only rewind to `before | last_turn_id`
  with optional replacement; compact only if the harness advertises the
  capability; side-chat fork copies a conversation slice into a new session
  with the same harness/model/guard (`docs/api.md:27`,
  `src/session/children.rs:167-238`).
- **Child threads:** managed (same sandbox, `cr_child_…`, parent gets a
  `notice_…` per finished child turn) vs. spawned in their own sandbox via a
  60 s app handshake with shared-sandbox fallback
  (`docs/api.md:25-26`, `src/session/thread.rs:299-424`); `detach` makes a
  child top-level (`docs/session-lifecycle.md:41`).
- **Teleport** (Mac ↔ cloud mid-task, both directions): staged
  `POST /v1/teleports → files/{i} (1 MB chunks, offset + sha256) → check
  (probe exact model) → activate | cancel`, single-record activation;
  manifest validated (exactly 1 native, kinds
  `native|context|project|attachment`, ≤4 GiB, no traversal)
  (`docs/api.md:81`, `src/session/teleport.rs:165-201,360-380,514-614`).
- **Automations:** deliberately thin — the agent records
  `{action,prompt,cron|at|delay}` and the *website* cron re-prompts; Core
  never parses cron (`src/session/automation.rs:1-85`).

## Security model (threat-argued, honestly bounded)

- **Dual users + containment.** Service `cloudroom` (creds + history) vs.
  worker `cloudroom-agent` (no admin, no sudo), `0700/0600` perms
  (`docs/setup.md:47-51`); spawn joins `cgroup.procs` with
  `PR_SET_NO_NEW_PRIVS` + `setgid/setuid` + zeroed ambient caps
  (`src/runtime/linux.rs:305-352`); per-workload freeze/kill, descendant
  wait (`src/runtime/linux.rs:63-88`). Containers without delegation fall
  back to `CLOUDROOM_WORKLOAD`-tagged signalling, explicitly weaker
  (`docs/storage.md:42-48`).
- **Token + loopback-first.** `CLOUDROOM_TOKEN` ≥32 chars, `0600` on disk,
  constant-time compare, `WWW-Authenticate: Bearer`, `Cache-Control:
  no-store`; localhost-only unless `ALLOW_NON_LOOPBACK_HTTP=1` (still
  plaintext — expects Tailscale/HTTPS proxy)
  (`src/config.rs:37-62`, `src/api/mod.rs:470-508`).
- **Journal-first history.** Local fsynced journal (`NNNN.record` + snapshot
  every 20 000 records) is primary; Postgres/website upload is
  gzip-per-session `PUT` with `If-None-Match:*`, `412` byte-compared,
  `Conflict` never overwrites; RLS owner-isolated tables, core never
  migrates (`src/session/outbox.rs:27-100`,
  `src/session/history.rs:127-226`,
  `docs/database/0001-session-records.sql:11-28`).
- **Disk guard with hysteresis.** Warn ≤5 GB, block + freeze <2 GB, resume
  >2.5 GB; failed measurement blocks; after 30 s still-paused, kill the
  heaviest disk-writing process group (never the harness's own), then thaw
  (`docs/storage.md:5-14`, `src/workspace/storage.rs:31-51,264-272`).
  Surfaced on `/v1/health` + `/v1/dashboard`; `ready` 503s when blocked.
- **Command Guard is labelled accident-prevention, NOT a sandbox.**
  Regex blocklist (`rm -rf /`, `mkfs`, `dd of=/dev/*`, fork bombs…)
  injected as harness hooks; a guard error *allows* the command
  (`docs/harnesses.md:94-108`). The honesty of the label is the lesson.
- **Secrets broker (user→agent, never reverse).** Agent requests over an
  agent-only Unix socket gated by `SO_PEERCRED uid` + ancestor-PID→session
  binding (`src/secrets/mod.rs:107-121`); answers are exact-set,
  single-line ≤16 KiB values; the CLI writes dotenv atomically (`0600`,
  tmp + rename) preserving comments — secret *values* never enter API
  history (`src/secrets/mod.rs:217-229,310-332,366-463`).
- **Accepted risk, on the record.** ADR-0001 accepts the core service's
  elevated privileges as a deliberate trade-off
  (`adr/0001-accept-privileged-core-service.md:5`).

## Observability (redaction by construction)

- Fixed diagnostic taxonomy (`api`, `api_summary`, `agent_start/exit`,
  `auth_health`, `computer_use`, `history_upload`, `resources`,
  `storage_stale`, …) — never prompts, paths, bodies, or raw URLs
  (`docs/observability.md:5-19`); 1 024-record queues, drop-counted,
  8 MiB rotated local logs, 5-minute uploads.
- Raw harness stderr is the *only* raw exception: last 16 KiB, `0600` files,
  never in SSE/history/API, inspect with `jq -a` (never `-r`)
  (`docs/observability.md:43-61`).
- CPU-minute rollups name process *names* (npm package for Node), never
  arguments or paths; first CPU sample is `unknown`, failed samples are
  `null`, never zero (`docs/observability.md:14-15`).

## GUI patterns worth stealing

- **Teleport cards:** context-overflow offers model + reasoning picker
  (`TeleportCheckCard.tsx`); stepwise Cloud→Local progress
  (`TeleportNotice.tsx`).
- **Inline context readout:** `%` + compact token count in the composer,
  from `thread/contextWindowUsage/updated` events
  (`packages/thread-view/src/thread-context-window-usage.ts`).
- **Tools hub:** skills + plugins + automations unified with registry search
  and paged grid (`ToolsView.tsx`, `lib/skills-registry.ts`).
- **3-tier permission selector** (Manual / Auto / Full) with icons at
  project + machine scope (`permission-mode-options.ts`).
- **Persistent drawer, not modals; banner cards** for prompt-stack, queue,
  and usage-limit nudges (`PromptStackCard`).
- **Contract packages** (`server-contract`, `host-daemon-contract`) enforce
  server-owns-policy / daemon-owns-execution; wire change bumps
  `HOST_DAEMON_PROTOCOL_VERSION` (`gui/AGENTS.md`).
- **Test discipline:** never mock the DB (in-memory + real migrations);
  desktop integration tests only in test environments (`gui/AGENTS.md`).

## What this means for Kubona

**Adopt (highest leverage first):**

1. **Content-bound idempotency keys** on every mutation (jobs, launch,
   tribunal, loops): same-ID-same-content returns the receipt, same-ID
   *different* content 409s. Kubona's ghost-session 404s (G7) stop
   mis-attribution; Cloudroom's keys stop *double-execution* — the adjacent
   hole. Touches `platform/core/pkg/api` + ledger receipts.
2. **202-saved vs done + steer/queue ops** for chat (`edit/cancel/reorder`
   queued, `steer` into the running turn). Kubona queues implicitly; making
   the queue first-class is the console's missing control surface
   ([[workflows-and-agent-authoring]], [[agent-ux-control-plane]]).
3. **Sleep with a background-process guard + drain gate** for autoresearch
   loops (Cloudroom's `sleep.rs:66-93,99-128`): reclaim idle harnesses
   without killing productive background work; refuse new work while
   draining. Directly answers the loop load/chaos follow-up in
   [[implementation-gaps]].
4. **Secrets discipline upgrade:** exact-set single-line answers, CLI-side
   atomic dotenv write, secret values never in event history; peercred-style
   caller binding where the OS allows. Extends [[secrets-and-workload-isolation]]
   and the Bao runbook without changing the lease model.
5. **Disk guard for the sandbox + bridge:** warn/block/resume thresholds
   with hysteresis, `ready` 503 when blocked, heaviest-writer kill that
   never targets the harness. Kubona has no disk guard; a runaway job or
   PySCF scratch fill is currently an unmodelled outage.
6. **Label the sandbox honestly** (Command-Guard precedent): accident
   prevention ≠ containment. Kubona's sandbox claims should carry the same
   explicit boundary statement.
7. **Single-record teleport pattern** for session migration
   (check → chunked validated upload → one activating record): the shape
   Kubona needs for lab-to-lab handoff and checkpoint resume, without
   adopting Cloudroom's Mac-specific halves.
8. **Observability redaction by construction** (fixed taxonomy, no prompts /
   paths / bodies; 16 KiB `0600` stderr capture): the FlightRecorder's next
   hardening step ([[agentic-memory-and-observability]]).
9. **Console UX:** inline context-% in the composer, overflow-teleport-style
   cards (for Kubona: overflow → compact / new-session / escalate-model),
   Tools hub unifying skills + packs + automations, drawer-over-modals.

**Do not copy:** the privileged-service trade-off (ADR-0001 — Kubona's
single-writer + bearer + limiter posture is stricter; keep it); the
container fallback's weaker containment (document, don't inherit);
harness-neutral output (Cloudroom admits `data` still differs per harness —
same honesty Kubona applies to emulation labels); automations' unparsed
cron (Kubona's scheduler, when built, should parse + validate, not
pass through); Pi credential-sync complexity (per-provider conflict rules
Kubona has no reason to inherit).

**Integration topology:** reference class, not submodule/sidecar
([[integration-topology-and-licence-discipline]]). Licence (Apache-2.0)
is compatible, but the Rust service + Electron shell do not belong inside
the Go core; mine the patterns (receipts, sleep, disk guard, secrets
broker, teleport record) as independent implementations.

Related: [[integration-topology-and-licence-discipline]] · [[risks]] ·
[[secrets-and-workload-isolation]] · [[workflows-and-agent-authoring]] ·
[[agent-ux-control-plane]] · [[agentic-memory-and-observability]] ·
[[implementation-gaps]] · [[protocol-strategy]] · [[user-identity-and-profiles]]
