# Live gVisor sandbox proof — 2026-11-09

**Scope:** prove the `sandboxd` + gVisor boundary runs hostile benchmark code
when a Linux host exposes the `runsc` Docker runtime. macOS Docker Desktop
still has no `runsc`; this proof used a Linux VM, which is the supported
default shape for the opt-in sandbox profile.

## Environment

- Host VM: Lima Ubuntu 26.04 arm64, 4 CPU / 4 GiB.
- Docker Engine `29.9.3` (rootful daemon used for the live run; the Lima
  rootless daemon rejects `runsc` with a systemd cgroup error).
- gVisor release `20261026.0` (`runsc` plus `gvisor-bin/`), downloaded from
  `https://storage.googleapis.com/gvisor/releases/release/latest/aarch64/`.
- Image: `python:3.12-slim`.
- Binary: Linux/arm64 build of `kubonad`, run as
  `kubonad sandboxd --socket /tmp/sandbox.sock --image python:3.12-slim --runtime runsc --scratch /tmp/kubona-sandboxd --max-concurrent 1`.

## Health

`curl --unix-socket /tmp/sandbox.sock http://localhost/health` returned:

```json
{"available":true,"image":"python:3.12-slim","reason":"","runtime":"runsc"}
```

## Hostile-run result

The benchmark attempted to open a TCP connection, print its UID, and read the
container hostname. `sandboxd` returned:

```json
{"stdout":"hello-from-gvisor 1000\nNETWORK_BLOCKED OSError\na4316870b819\n","stderr":"...","exit_code":0,"duration_ms":5583,"sandboxed":true,"isolation_tag":"gvisor-runsc:python:3.12-slim (net=none, readonly, cap-drop=ALL)"}
```

The run executed as UID `1000`, had no network route, and the container was
read-only with `cap-drop=ALL`, no-new-privileges, and the fixed seccomp
profile. The sandboxd socket is `0600` and owned by the daemon UID, so only the
daemon can submit requests.

## What this closes and what it does not

- **Closes the live-execution question for a Linux host/VM.** The topology and
  the isolation flags are no longer only unit-tested.
- **Does not make macOS Docker Desktop a gVisor host.** Use a Linux VM/host or
  keep autoresearch disabled; the API continues to report unavailable rather
  than silently running untrusted code under a weaker runtime.
- The VM proof used a rootful Docker daemon for `runsc`; the compose profile
  documents the same requirement.
