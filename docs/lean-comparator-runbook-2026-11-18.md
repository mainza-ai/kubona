# Lean comparator sandbox — build, warm, and live verification

**Date:** 2026-11-18 · **Status:** verified live (kernel proof + API-attested)

This runbook is the operational path for the Kubona formal-verification boundary.
It records the pins, the two host requirements that are easy to get wrong, and the
exact commands that produced a `live_kernel_proven` attestation.

## Pins

| Component | Pin | Why |
|---|---|---|
| Lean | `v4.34.1` (release tarball, SHA-256 per arch) | Matches `submodules/math/lean/lean-toolchain` |
| Comparator | `v4.34.0` commit `d03acab…` | Merged into Lean core at v4.35.0; this is the last 4.34-line tag |
| `lean4export` | `v4.34.0` commit `076e8e5…` | Must match the target Lean version |
| landrun | `v0.1.17` (SHA-256 per arch) | Comparator's inner Landlock sandbox |
| Base image | `debian:trixie-slim` | landrun's release binary needs glibc ≥ 2.38 |

Upstream publishes **no** `leanprover/lean4` Docker image (Docker Hub returns
404); the image therefore installs the checksummed GitHub release tarball.
`platform/tests/test_lean_sandbox_pin.py` keeps the toolchain, both Lean
checksums, landrun, comparator/`lean4export` commits, image tag, and script in
sync.

## Host requirements

1. **A standard Linux kernel with Landlock + landrun.** Docker Desktop's
   linuxkit kernel reports Landlock ABI 8 and a minimal `landlock_*` probe works,
   but landrun v0.1.14/v0.1.17 apply an effectively deny-all ruleset there.
   Use a Linux host, CI, or Colima/Lima.
2. **A mount the compiler can write through.** For macOS VMs use Colima with
   `--mount-type virtiofs`; sshfs drops the Docker API tunnel under Lean's write
   volume. Run the container as the checkout-owner UID with `HOME=/tmp`
   (`sandbox_docker_cmd` does this; `KUBONA_SANDBOX_USER=uid:gid` overrides a
   VM-remapped mount).

Disk: the warm cache is multi-GB; the image is ~5 GB. The live run used the
existing `LeanSandboxConfig` caps (4 GB / 2 CPU / `--read-only` /
`--network none` / `--cap-drop ALL` / 256 PIDs / tmpfs `/tmp`).

## Build

```bash
scripts/build-lean-sandbox.sh
# verifies the pinned compiler and Lake inside the image
```

The script reads `submodules/math/lean/lean-toolchain`, refuses a
Dockerfile/version mismatch, and fails if the image reports another Lean
version.

## Warm (network required, once)

```bash
scripts/build-lean-sandbox.sh --warm
```

This runs `lake update` and `lake exe cache get` (8,908 Mathlib cache files in
the recorded run) as the checkout owner. Comparator builds the challenge and
solution modules on demand afterwards; a full `lake build` of the corpus is not
required.

## Live verification

```bash
scripts/comparator-verify.sh \
  --challenge ThreeStateSupercritical \
  --session <session-id> \
  --out /tmp/attestation.json
```

The CLI runs `lake env comparator ComparatorChallenges/<Name>.json` inside the
pinned image, refuses anything weaker than `live_kernel_proven`, and signs the
canonical payload. Post the attestation to
`POST /api/sessions/<session-id>/formal-verifications`; the daemon verifies it
against `KUBONA_COMPARATOR_PUBKEY_HEX`.

## Recorded evidence (2026-11-18)

- Challenge: `ThreeStateSupercritical` (theorems
  `OAI.ThreeState.poisson_supercritical`, `OAI.ThreeState.regular_supercritical`).
- Build: 8,950 targets; both environments exported.
- Verdict: `Lean default kernel accepts the solution` → `Your solution is okay!` ·
  `live_kernel_proven` · axioms `propext`, `Quot.sound`, `Classical.choice` ·
  `sorryAx` count 0.
- Attestation: proof hash `4762a59cac58…`; API accepted it (HTTP 201, event 780).

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `pull access denied` for `leanprover/lean4` | No official image exists | Use this image (tarball build) |
| landrun `permission denied` on every path | linuxkit kernel | Run on a standard Linux kernel / Colima |
| `unexpected EOF` from Docker during the build | sshfs mount under heavy writes | Colima `--mount-type virtiofs` |
| `permission denied` writing `.lake` | container UID ≠ checkout owner | Set `KUBONA_SANDBOX_USER=uid:gid` |
| `lake exe comparator` unknown target | comparator is an external binary | Use `lake env comparator <json>` |
