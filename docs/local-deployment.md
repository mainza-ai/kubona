# Local private deployment

Kubona is local-first. The supported production shape is one operator running
the stack on their own machine or lab host, reachable only from that host
(loopback) or through an explicit private tunnel. There is **no required
domain, Cloudflare account, public reverse proxy, or public certificate**.

The only cloud egress is:

1. **QPU dispatch** from the bridge to the provider you configured.
2. **Opt-in cloud LLM** from kubonad (`KUBONA_LLM_CLOUD=1`).

Everything else (console, ledger, simulator, local LLM via Strata/oMLX/
Ollama/LM Studio, backup, provenance) works offline.

## 1. Prerequisites

- Docker Engine/Desktop with Compose v2 (verified with Docker 29.x).
- ~8 GB free disk for images/state; 4 GB RAM is the compose floor.
- A local LLM engine if you want LLM features (Strata is the compose default).
  Without one, simulator/ledger features still work; LLM routes fail closed.
- `openssl` (token generation) and `go` (only if you build/run `kubonad`
  locally for `keygen`).

## 2. First boot

```bash
cp .env.example .env
chmod 600 .env
# Required for writes:
#   KUBONA_API_TOKEN=$(openssl rand -hex 32)
#   KUBONA_SECRET_KEY=$(openssl rand -hex 32)  # seals UI-entered LLM keys at rest
# Reviewer key (production refuses an ephemeral signer):
cd platform/core && go run ./cmd/kubonad keygen
# paste the printed KUBONA_REVIEWER_PRIVKEY_HEX=... into .env
# Optional bridge-side cost-mandate key:
go run ./cmd/kubonad keygen --kind operator
cd ../..

docker compose --env-file .env -f platform/docker/compose.prod.yml up --build -d
curl -fsS http://127.0.0.1:8080/api/health
open http://127.0.0.1:3000   # macOS; xdg-open on Linux
```

`docker compose --env-file .env` is deliberate: the compose file lives under
`platform/docker/`, so Compose's automatic `.env` lookup is not the repo root.

### First-run identity (console writes)

Reads and the zero-login local flow work immediately. Authenticated writes
(LLM/quantum settings, mandates, rerun) need either:

- **Console login:** open **Labs** → bootstrap the lab's admin key (first
  write wins; the key stays in browser custody) → **Log in**. The Ed25519
  session cookie then authorizes writes. This is the supported local flow;
  no public token is baked into the console.
- **API/agent:** send `Authorization: Bearer $KUBONA_API_TOKEN` (the token
  from `.env`) on gated routes.

## 3. Prove it is private

Both published ports must be loopback-only:

```bash
# macOS
lsof -nP -iTCP:8080 -sTCP:LISTEN
lsof -nP -iTCP:3000 -sTCP:LISTEN
# Linux
ss -ltnp 'sport = :8080'
ss -ltnp 'sport = :3000'
```

The host address must be `127.0.0.1` (or `::1`), never `0.0.0.0`, `*`, or a
LAN address. The convenience check:

```bash
scripts/local-deployment-check.sh
```

If a LAN/public listener is required, treat it as a separate deployment
decision: add TLS (`--tls-cert/--tls-key`) or put the stack behind a
WireGuard/SSH tunnel. Never publish plaintext `8080`/`3000` on `0.0.0.0`.
The container-internal bind is `0.0.0.0` only so Docker healthchecks and the
loopback-published host port work; the host binding above is the boundary.

### Remote access (optional)

Single operator, SSH tunnel:

```bash
ssh -L 3000:127.0.0.1:3000 -L 8080:127.0.0.1:8080 user@host
```

Small lab, WireGuard: expose only the WireGuard UDP port and rate-limited
SSH; keep the host firewall default-deny for everything else. TLS is provided
by the tunnel.

## 4. Local LLM (default)

The compose stack points kubonad at Strata on the host:

- `STRATA_BASE_URL=http://host.docker.internal:47831/v1`
- `STRATA_MODEL=iq3_s` (override in `.env`)

Start Strata's serving panel, then verify from the console (LLM Settings →
models) or:

```bash
docker compose --env-file .env -f platform/docker/compose.prod.yml exec kubonad \
  curl -fsS http://host.docker.internal:47831/v1/models
```

If Strata is not running, LLM features refuse rather than inventing output.
The deterministic simulator, ledger, QPU bridge, and backup paths still work.

## 5. Cloud LLM (optional, BYOK)

Cloud LLM is an explicit exception, not a default.

1. Put your provider key and model in `.env` (`NVIDIA_API_KEY`/`NVIDIA_MODEL`,
   `OPENAI_API_KEY`/`OPENAI_MODEL`, or `ANTHROPIC_*`).
2. Set `KUBONA_LLM_CLOUD=1`. Production refuses **any enabled cloud role**
   without this flag, not just the active provider.
3. Turn **off** Sovereign Air-Gap in LLM Settings (fresh installs default it
   on), or the provider is blocked by design.
4. Assign the `codesign` role to that provider/model in LLM Settings, or set
   `KUBONA_LLM_CODESIGN_PROVIDER`/`_MODEL` in `.env`.
5. Supply a **runtime vendor rate card** (`KUBONA_RATE_CARD_PATH` or
   `KUBONA_RATE_CARD_URL`). The embedded card is a bootstrap; production
   cloud pricing refuses it. Run `kubonad rates-audit --db ...` to verify the
   models actually assigned are live and priced.
6. Restart the stack.

Persisted LLM settings override env defaults: the console **LLM Settings**
is authoritative per user. Strata/oMLX/Ollama/LM Studio are local and need no
cloud opt-in; cloud providers are BYOK and require `KUBONA_LLM_CLOUD=1` in
production. A host showing `active=nvidia`/air-gap off is a development
override, not the shipped default (fresh compose volumes start on Strata).

Billing reality: the provider bills **your API key**. Kubona is not the payer
and cannot see the real invoice. Kubona records an **estimate** (runtime
rate card) and enforces:

- `KUBONA_CODESIGN_RUN_BUDGET_USD` (default `0.50`) per rerun,
- optional `KUBONA_LLM_BUDGET_USD` global guardrail,
- per-call max tokens, step cap, and a 10-minute run timeout.

It does **not** debit the QPU session/tenant cap for a BYOK cloud LLM bill.
The provider dashboard is authoritative.

## 6. QPU cloud (optional)

Only the bridge needs QPU credentials. Set `KUBONA_IBM_QISKIT_TOKEN` (and
optionally instance/backend) in `.env`; the bridge is the only component that
talks to the QPU vendor. Without credentials, simulators still work and the
fleet reports the backend as credentialless (fail-closed at submit).
Air-gap mode disables QPU cloud as well as cloud LLM.

## 6b. Agent MCP layer (stdio)

The 6 MCP servers are **stdio processes spawned by the agent host**, not
long-running compose services. On the same machine, run them from the repo
venv (or install the package):

```bash
cd platform
.venv/bin/python -m kubona.mcp.servers.compiler
# execution, cost, provenance, control, world — same pattern
```

They use local SQLite/ledger state and the same contracts; no TCP control
plane is exposed. The compose stack is the human/operator plane. If an agent
runs in a container, give it the repo/venv and the state volume, or point it
at the Go API instead. This is the supported local topology; a containerized
stdio broker is not required and is not shipped.

The `cost` server's `codesign_rerun` tool forwards to `POST
/api/codesign/rerun`. Set `KUBONA_API_URL` (default `http://127.0.0.1:8080`)
and `KUBONA_API_TOKEN`; without the token it refuses (no anonymous spend).

## 7. Data, secrets, and backup

- State: the `kubona-state` volume mounts at `/home/kubona/.kubona` (SQLite
  ledger, models, provenance spool).
- Secrets: `.env` (chmod 600) for a local install; Docker secrets/OpenBao for
  a shared host. Never commit `.env`; it is gitignored. `kubonad keygen`
  prints the reviewer/operator seeds; write them to `.env`, not shell history.
- Backup/restore, scheduled `backup` profile, FK audit/quarantine, and
  retention: [`docs/backup-restore.md`](backup-restore.md).
- Scheduled encrypted backups (optional profile):
  ```bash
  # add KUBONA_BACKUP_AGE_RECIPIENT=age1... to .env
  docker compose --env-file .env -f platform/docker/compose.prod.yml \
    --profile backup up -d
  ```
  Check freshness with `kubonad backup-status --dir /backups --max-age 26h`.
- Existing DB: production refuses to start on FK orphans. Back up, stop the
  daemon, then:
  ```bash
  kubonad fk-audit --db /path/to/kubona.db
  kubonad fk-repair-plan --db /path/to/kubona.db
  kubonad fk-quarantine --db /path/to/kubona.db --apply
  kubonad fk-audit --db /path/to/kubona.db
  ```
  For compose, run these against the state volume while the stack is stopped.

## 8. Rate cards

The active card is runtime data (`KUBONA_RATE_CARD_PATH`/`URL`; embedded
bootstrap fallback), effective 2026-10-30 with a 30-day window. It becomes
stale on 2026-11-29. `kubonad` refuses to boot on a stale configured card
(fail-closed), and pricing/lens/cloud-LLM paths refuse at call time rather
than invent a rate. Production cloud pricing requires a runtime card; local
providers are free. Update procedure and the current model table:
[`docs/rate-cards.md`](rate-cards.md).

## 8b. Preflight doctor

`kubona doctor` reports the local deployment state without printing secrets:

```bash
cd platform
.venv/bin/python -m kubona.run doctor --env-file ../.env
# add --strict to exit non-zero on warnings (rate card, FK orphans, reviewer key)
```

It checks the state root/ledger, provider inventory, event taxonomy, rate-card
freshness, FK orphan count on `~/.kubona/kubona.db`, configured backup
freshness/last failure, reviewer/operator/QPU key presence, effective
LLM/air-gap posture, bridge configuration, and daemon readiness. A stopped
daemon is reported, not treated as a failure; an absent backup directory is
reported as optional, while a configured-but-empty/stale/failed backup set is
a `--strict` failure.

## 9. Diagnostics (local by default)

- Logs: `docker compose ... logs -f kubonad console bridge`.
- Metrics: `GET /metrics` with `Authorization: Bearer $KUBONA_API_TOKEN`.
- Health/readiness: `/api/health`, `/api/ready`.
- No telemetry leaves the host by default. Sentry/OTLP are not release
  requirements and are not enabled in this compose file.

## 10. Update / rollback

```bash
git pull
docker compose --env-file .env -f platform/docker/compose.prod.yml up --build -d
```

Back up first. The daemon takes a verified pre-migration snapshot when it
opens an older schema, and refuses the upgrade if that snapshot fails. To
roll back, stop the daemon, restore the pre-migration snapshot with
`kubonad restore`, then check out the previous commit and rebuild. Never
restore a backup over a running daemon.

## 11. Acceptance checklist

- [ ] `docker compose ... config` is clean.
- [ ] `kubona doctor --env-file .env` reports no stale rate card or FK orphans.
- [ ] `/api/health` is 200 and `/api/ready` is 200 on a fresh volume.
- [ ] Console loads at `http://127.0.0.1:3000`.
- [ ] `ss`/`lsof` shows loopback only for 8080/3000.
- [ ] A request from another host fails.
- [ ] Writes without `KUBONA_API_TOKEN` fail closed (503).
- [ ] `kubonad restore --from ... --db ...` dry run is clean; `--apply` on a
      scratch path verifies and leaves a preservation directory.
- [ ] `kubonad backup-status --dir /backups --max-age 26h` is fresh with the
      `backup` profile enabled.
- [ ] Local LLM answers; cloud LLM refuses without `KUBONA_LLM_CLOUD=1`.
- [ ] `POST /api/codesign/rerun` with bearer returns a sealed receipt;
      anonymous callers get 401 and no LLM egress.
- [ ] QPU dispatch works with credentials, or reports credentialless.
- [ ] `curl /api/health` works with the network unplugged (except configured
      local LLM/QPU).
