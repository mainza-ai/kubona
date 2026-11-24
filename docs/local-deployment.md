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
open http://localhost:3000   # macOS; xdg-open on Linux
```

`docker compose --env-file .env` is deliberate: the compose file lives under
`platform/docker/`, so Compose's automatic `.env` lookup is not the repo root.

### First-run identity (console writes)

Reads and the zero-login local flow work immediately. In the supported
single-lab deployment (`KUBONA_SINGLE_LAB=1`) with a local LLM, campaign
creation and chat are open without a sign-in prompt; `GET /api/access` reports
`mode: "local_private"` and `anonymous_chat: true`. Cloud LLM providers, QPU
dispatch, settings, mandates, and lab administration still require auth.
Authenticated writes (LLM/quantum settings, mandates, rerun) need either:

- **Console login:** open **Labs** → bootstrap the lab's admin key (first
  write wins; the key stays in browser custody) → **Log in**. The Ed25519
  session cookie then authorizes writes. This is the supported local flow;
  no public token is baked into the console.
- **API/agent:** send `Authorization: Bearer $KUBONA_API_TOKEN` (the token
  from `.env`) on gated routes.

### Calibration refresh (QPU)

The fidelity gate refuses hardware dispatch when the contract is older than 24
hours. The state volume owns a writable copy; the image copy is only a
bootstrap. Refresh it from the live provider **without rebuilding**:

```bash
docker compose --env-file .env -f platform/docker/compose.prod.yml \
  run --rm --no-deps bridge \
  python -m kubona.fidelity.refresh --provider ibm-qiskit --device ibm-heron --write
```

Both the bridge (`KUBONA_FIDELITY_DEVICES`) and kubonad
(`KUBONA_CONTRACTS_DIR`) resolve the state-volume contract on boot. Restart the
two services after a refresh if a long-running process has already cached it.
A failed or unmeasurable refresh refuses; it never stamps a fresh timestamp
without live data. `kubonad` checks the shared contract on boot and hourly
(`KUBONA_CALIBRATION_CHECK_SECONDS`, default 3600), seals
`calibration_stale`/`calibration_resumed` ledger events, and alerts on the
transition; `/api/fleet` exposes the exact age and freshness per device.

### Autoresearch sandbox (opt-in, gVisor)

The default compose stack intentionally has **no Docker socket**: autoresearch
harnesses refuse (`docker_runner.available:false`) rather than executing code
on the host. The supported production path is the narrow `sandboxd` sidecar:
kubonad reaches a private UDS; only the sidecar reaches the Docker socket; the
sidecar launches the fixed runner image through **gVisor `runsc`** with
`--network none`, read-only rootfs, cap-drop ALL, no-new-privileges, seccomp,
non-root, and memory/PID/CPU caps. Its API has only `/health` and `/v1/run`;
there is no image, command, bind, runtime, or network override.

Prerequisites on the Linux host:

```bash
# Install gVisor and register its runtime in /etc/docker/daemon.json, then
# restart Docker. Verify:
docker info --format '{{json .Runtimes}}' | grep runsc
docker build -f platform/docker/Dockerfile.quantum-runner \
  -t kubona-quantum-runner:1.1.0 platform
sudo install -d -m 0700 /var/lib/kubona/sandbox
```

Run the opt-in profile with a host-visible scratch path (the Docker daemon
resolves bind-mount sources on the host, so a container-private path would
mount empty directories):

```bash
export KUBONA_SANDBOX_SCRATCH=/var/lib/kubona/sandbox
docker compose --env-file .env \
  -f platform/docker/compose.prod.yml \
  -f platform/docker/compose.sandbox.yml up -d --build
```

If `runsc` is missing, `sandboxd` stays up but `/health` returns 503 and every
run refuses; kubonad reports the reason and autoresearch stays unavailable.
macOS Docker Desktop does not provide a gVisor boundary: use a dedicated
Linux VM/host or keep autoresearch unavailable. The sidecar is still
root-equivalent through the Docker socket; run it only on a trusted
local-private host.

### Local reset and isolated E2E

The live ledger contains labs and campaigns. Never point E2E at it: the browser
suite creates labs. Use the disposable launcher:

```bash
KUBONA_BRIDGE_SOCKET=/path/to/bridge.sock \
KUBONA_BRIDGE_TOKEN=... \
python3 scripts/e2e-isolated.py --build-console
```

It creates a temporary ledger/state directory, starts isolated API (`:18080`)
and console (`:13000`) processes, runs Playwright, and tears them down.

To reset a stopped local ledger, take a verified backup first:

```bash
python3 scripts/local-reset.py --db "$PWD/kubona.db" --yes
```

The script refuses while the daemon holds the ledger lock, writes a verified
`*.reset-backup-<UTC>` snapshot, removes the ledger/WAL/SHM files, and leaves
the backup in place. Restart `kubonad` to create a clean ledger.

### Reset a compose ledger

The supported one-command path is:

```bash
scripts/reset-compose-state.sh                 # dry run: prints the plan
scripts/reset-compose-state.sh --yes           # archive + reset
```

It takes a verified SQLite backup, copies the whole state volume to a new
archive volume, then removes only `kubona.db{,-wal,-shm}` and restarts. It
preserves the backup volume, `pqc_recipient_key.bin`, contracts, models, and
evals. **The DB holds sessions, labs, sealed provider keys, claims, and
campaigns**; export the publication evidence first
(`kubonad manuscript-export`) and re-enter sealed keys after the reset.

The manual equivalent follows.

`local-reset.py` is a host-path tool: it cannot reach a ledger inside the
`kubona-state` Docker volume and it cannot take the lock while the compose
daemon holds it. For the compose topology, back up first, stop only
`kubonad`, then remove the ledger files through the image (which has the
volumes mounted at the same paths):

```bash
COMPOSE="docker compose --env-file .env -f platform/docker/compose.prod.yml"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"

# 1. verified snapshot while the daemon is still running
docker exec kubona-prod-kubonad-1 kubonad backup \
  --db /home/kubona/.kubona/kubona.db \
  --out "/home/kubona/.kubona/kubona.reset-backup-$STAMP.db"

# 2. release the ledger lock (leaves preflight/backup containers untouched)
$COMPOSE stop kubonad

# 3. remove the ledger + WAL/SHM, keep every reset-backup-*.db
$COMPOSE run --rm --no-deps --entrypoint sh kubonad -c \
  'cd /home/kubona/.kubona && rm -f kubona.db kubona.db-wal kubona.db-shm'

# 4. start clean; the daemon recreates the schema and the default lab
$COMPOSE up -d
```

Then assert the declared posture: `scripts/local-deployment-check.sh
--expect-single-lab`. A fresh ledger holds one tenant, so `/api/access`
reports `mode:"local_private"` and `single_lab:true`.

> ⚠ `docker compose down -v` is **not** a reset procedure for a deployment
> you intend to keep: it removes the `kubona-backups` volume with the state
> volume. Move the backups off-host first, or use the four steps above.

Why this matters: `singleTenantStore()` counts distinct tenants in the
sessions table, not a config flag. E2E labs created against a live stack (or
a restored multi-lab ledger) silently flip a `KUBONA_SINGLE_LAB=1` deployment
to `multi_lab`, which re-requires operator auth for reads/campaign creation
and disables anonymous local chat.

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
scripts/local-deployment-check.sh --expect-single-lab
```

`--expect-single-lab` also fails when the daemon reports `multi_lab`; use it
whenever the deployment claims the single-lab posture (it is what the compose
default declares). Without the flag the mode is printed but not asserted.

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

## 6b. Agent MCP layer (unified stdio + standard Streamable HTTP) & External Skills

The **7 MCP servers (49 tools)** (`compiler`, `control`, `cost`, `execution`,
`provenance`, `science`, `world`) can be accessed either via the **unified
profiled stdio server** or over **standard MCP Streamable HTTP (`POST /mcp`)**
served by the `mcp` compose service on `127.0.0.1:8123`:

```bash
# Unified stdio (profiled to prevent context flooding)
cd platform
.venv/bin/python -m kubona.run serve-mcp --profile science
# Profiles: all | read-only | science | governance | ops | world

# Standalone single-server stdio (still supported)
.venv/bin/python -m kubona.mcp.servers.compiler
# execution, cost, provenance, control, science, world — same pattern
```

The compose `mcp` service runs `kubona.run serve-gateway` on `127.0.0.1:8123`
with OAuth2 bearer auth, DNS-rebinding protection, `PolicyEngine` scope checks,
MRTR paid-execution approval gates, and audit JSONL:
- **Standard MCP Streamable HTTP:** `POST http://127.0.0.1:8123/mcp?profile=science`
  (JSON-RPC 2.0, `stateless_http=True`, `json_response=True`, Bearer auth required;
  unknown profiles fail closed with `400`).
- **Legacy HTTP RPC:** `POST http://127.0.0.1:8123/rpc` (`{server, tool, args}`
  or post-quantum X25519+ML-KEM `{envelope}`).
- **MCP-native onboarding:** Server `instructions`, read-only Resources
  (`kubona://skills`, `kubona://skills/{name}`, `kubona://contracts/error-catalog`,
  `kubona://contracts/mcp-catalog`, `kubona://contracts/experiment-kinds`,
  `kubona://contracts/rate-cards`), and 5 guided Prompts (`vqe-with-anchor`,
  `hardware-readiness-check`, `cost-governed-submit`,
  `audit-session-provenance`, `debug-failed-job`).

Generate copy-pasteable client configs (zero secret leakage — placeholders only)
and install the 6 external agent skills (`skills/kubona*`):

```bash
# Generate MCP client config for your host
platform/.venv/bin/python -m kubona.run mcp-config --client claude-code --transport http --profile science
# Supported clients: claude-code | claude-desktop | cursor | codex | antigravity

# Install external SKILL.md suite into your agent host
scripts/install-skills.sh claude        # or: cursor | codex | antigravity | project
```

`kubonad` (`127.0.0.1:8080`) also serves runtime discovery for HTTP and A2A
agents: `GET /llms.txt`, `GET /api/openapi.json`, and `GET /.well-known/agent.json`,
plus bearer-gated `POST /api/chemistry/anchor` for live PySCF classical anchors.
In `compose.prod.yml`, `mcp` reaches `kubonad` via `KUBONA_CORE_URL=http://kubonad:8080`
(with `KUBONA_ALLOWED_HOSTS=${KUBONA_ALLOWED_HOSTS:-kubonad}` on `kubonad`),
mounts `../../skills:/app/skills:ro`, and forwards `KUBONA_API_TOKEN` so
`science` / `world` / `codesign_rerun` fail closed without an operator token.

## 7. Data, secrets, and backup

- State: the `kubona-state` volume mounts at `/home/kubona/.kubona` (SQLite
  ledger, models, provenance spool).
- Secrets: `.env` (chmod 600) for a local install; Docker secrets/OpenBao for
  a shared host. Never commit `.env`; it is gitignored. `kubonad keygen`
  prints the reviewer/operator seeds; write them to `.env`, not shell history.
  Set `KUBONA_SECRET_KEY` (32-byte hex/base64) before entering provider keys
  in the console: production refuses new plaintext provider-key writes and
  `kubonad preflight` fails if a stored key is plaintext. Env-only provider
  credentials remain valid without it. To migrate an existing plaintext
  ledger, stop the daemon and run `kubonad seal-secrets --db /abs/kubona.db`
  with the same key; it re-seals stored provider credentials and prints only
  counts.
- Backup/restore, scheduled `backup` profile, FK audit/quarantine, and
  retention: [`docs/backup-restore.md`](backup-restore.md). `scripts/backup-restore-check.sh`
  accepts either a backup directory or an explicit backup file path and verifies
  any `.sha256` sidecar before restoring into a scratch ledger. Run
  `kubonad restore-drill --db /abs/kubona.db` before upgrades.
- **Restart-orphan `sleeping` loops on campaign delete:** If a campaign refuses
  deletion with `live loops` because a loop was left in `sleeping` state across
  a daemon restart, click **Settle sleeping loops & delete** in the Console
  confirm dialog (or `POST /api/sessions/{id}/loops/settle`) to archive the
  orphan loop with a signed `loop_settled` receipt before deleting. For the
  routine off-host encrypted drill, point a genuinely separate mount at
  `--offhost-dir` and run:
  ```bash
  kubonad offhost-drill --db /abs/kubona.db --offhost-dir /mnt/offhost \
    --age-recipient age1... --age-identity /secure/age-identity.txt
  ```
  It encrypts the verified backup, decrypts the off-host copy, restores it to
  scratch, and compares row counts. The operator owns the off-host mount and
  key custody.
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

`kubonad preflight` is the stack-start gate (compose runs it before kubonad).
It checks the same rate-card/ledger/FK/backup conditions. Set
`KUBONA_ALERT_WEBHOOK_URL` to receive a JSON alert POST when preflight or a
scheduled backup fails, when the disk guard escalates to blocked, or when a
job settlement is skipped because vendor usage is missing/invalid. Alerts are
best-effort and never block the monetary/state path. Set `KUBONA_STRICT_CONTRACTS=1` to
turn console response-contract drift into a visible failure instead of an
advisory banner.

## 9. Diagnostics (local by default)

- Logs: `docker compose ... logs -f kubonad console bridge`.
- Metrics: `GET /metrics` with `Authorization: Bearer $KUBONA_API_TOKEN`.
  Includes HTTP counters, Go heap/GC/RSS, cgroup memory limit/use, bridge RSS
  (`kubona_bridge_rss_bytes`, fresh <60 s), and memory-store item/vector
  counts.
- Health/readiness: `/api/health`, `/api/ready`.
- Profiler: start with `KUBONA_PPROF=1`; `/debug/pprof/` is **bearer-only**
  and off by default. Disable after diagnosis.
- Memory/retention: `kubonad memory-status --db <abs> --json`,
  `kubonad memory-graph --db <abs> --tenant default`, and
  `kubonad memory-feedback --db <abs> --id <id> --useful`. Retention is on in
  compose (`KUBONA_RETENTION_ENABLED=1`, chat 90 d / tombstones 30 d); run
  `kubonad vacuum --db <abs>` once to enable incremental auto-vacuum on an
  existing database.
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
- [ ] `scripts/local-deployment-check.sh --expect-single-lab` passes: loopback
      only, health/ready green, console reachable, and `/api/access` reports
      `mode:"local_private"` / `single_lab:true`. A `multi_lab` report on a
      declared single-lab host means the ledger is not fresh (see "Reset a
      compose ledger").
- [ ] Console loads at `http://localhost:3000` (same host as the API so the session cookie stays same-site).
- [ ] `ss`/`lsof` shows loopback only for 8080/3000.
- [ ] A request from another host fails.
- [ ] Writes without `KUBONA_API_TOKEN` fail closed (503).
- [ ] `kubonad restore --from ... --db ...` dry run is clean; `--apply` on a
      scratch path verifies and leaves a preservation directory.
- [ ] `kubonad backup-status --dir /backups --max-age 26h` is fresh **inside
      the `backup`/`preflight` container**. The `kubonad` service does not
      mount `kubona-backups`, so running it there reports a false "no backup
      files found".
- [ ] `kubonad verify --db <newest backup>` is clean, and one real restore of a
      backup file (not only `restore-drill`) has been exercised into a scratch
      path. `restore-drill` proves the live ledger round-trips; it does not
      prove the scheduled backup file is readable.
- [ ] `scripts/backup-restore-check.sh` restores the newest scheduled backup
      file into a scratch ledger and re-verifies the chain (run it inside the
      `backup` image/container; see `docs/backup-restore.md`).
- [ ] Publishing works: a session with a measured result accepts
      `POST /api/sessions/{id}/manuscripts {"build":true}`, the returned
      `main.pdf` starts with `%PDF-` and its downloaded SHA-256 equals the
      artifact's recorded hash, and a rebuild returns the same `build_hash`
      (the contract is enforced in `platform/core/pkg/api/manuscripts.go` and
      exercised by the Console `PublicationPanel`).
- [ ] `/metrics` (operator bearer) shows `kubona_jobs`, `kubona_manuscripts`,
      `kubona_bridge_configured`, and `kubona_backup_age_seconds`.
- [ ] Backups are encrypted off-host (`KUBONA_BACKUP_AGE_RECIPIENT` set,
      `KUBONA_BACKUP_REQUIRE_ENCRYPTION=1`) **or** a signed residual-risk
      waiver records plaintext same-host backups.
- [ ] `preflight` is run with `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1` once the
      backup profile is live, so a stale backup blocks startup instead of
      warning.
- [ ] Local LLM answers; cloud LLM refuses without `KUBONA_LLM_CLOUD=1`.
- [ ] `POST /api/codesign/rerun` with bearer returns a sealed receipt;
      anonymous callers get 401 and no LLM egress.
- [ ] QPU dispatch works with credentials, or reports credentialless.
- [ ] `curl /api/health` works with the network unplugged (except configured
      local LLM/QPU).
