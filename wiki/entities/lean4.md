---
name: Lean 4
type: entity
tags: [entity, tool, formal-verification, theorem-prover, programming-language]
date: 2026-10-08
updated: 2026-11-18
status: adopted
sources: [source-openai-math-corpus]
---

# Lean 4

**Lean 4** is an interactive theorem prover and functional programming language developed by Leonardo de Moura and the Lean community. It is based on the calculus of inductive constructions (CIC) and features dependent type theory, meta-programming, and an optimizing ahead-of-time compiler.

- **Toolchain in `openai/math`:** `leanprover/lean4:v4.34.1`
- **Package manager:** Lake (`lakefile.lean`)
- **Mathematical library:** Mathlib4 (and specialized external repos)
- **Role in Kubona:** Formal verification backend for automated proof validation and epistemic certainty in scientific discovery.

---

## Technical Characteristics

1. **Axiom Discipline:**
   The standard Lean 4 axiomatic foundation consists of:
   - `propext`: Propositional extensionality.
   - `Quot.sound`: Quotients.
   - `Classical.choice`: Axiom of choice.
   Proofs that rely on these three axioms are considered fully classical and sound. The use of `sorryAx` (unproven gaps) or arbitrary ad-hoc axioms is strictly tracked and rejected by verification tools like [[comparator]].

2. **Lake Package Management:**
   In `openai/math/lean/lakefile.lean`, Lake coordinates over 20 external community and research libraries, compiling them into a unified verification library (`OAI`).

3. **Memory and Scalability:**
   Large formal libraries can exhaust system memory mappings. Lean 4 provides options such as `-DMMAP=OFF` and `GLIBC_TUNABLES=glibc.malloc.mmap_max=0:glibc.malloc.arena_max=1` to operate reliably in containerized sandbox environments.

---

## Role in Kubona's Architecture

Within Kubona's [[epistemic-curvature]] and [[science-discovery]] stack:
- **Sandbox Execution:** Lean 4 runs inside isolated container sandboxes ([[secrets-and-workload-isolation]]), invoked by autonomous agents to verify lemmas before claim publication.
- **Epistemic Weight:** Claims accompanied by a verified Lean 4 proof achieve an epistemic confidence $\rho = 1.0$ on the [[claim-credibility-wavefunctions|credibility curve]], serving as immutable anchor points for scientific discovery campaigns.

See also [[comparator]], [[openai-math]], [[formal-proof-verification-lean-comparator]], [[claim-credibility-wavefunctions]].

> ⚑ Correction (2026-11-18): the earlier recipe used
> `FROM leanprover/lean4:v4.34.1`. Upstream does **not** publish an official
> `leanprover/lean4` Docker image — the Docker Hub repository returns 404, and
> the only community image (`leanprovercommunity/lean4`) is a stale 2024
> snapshot — so that base could never resolve. The image now installs the
> checksummed `lean-4.34.1-linux{,_aarch64}.tar.zst` release asset (SHA-256
> pinned per architecture; BuildKit `TARGETARCH` selects it, after the first
> build attempt caught an x86_64 binary on this Apple Silicon host).
> `scripts/build-lean-sandbox.sh` reads the vendored `lean-toolchain`, refuses a
> Dockerfile/version mismatch, and verifies `lean --version` and `lake --version`
> in the built image. See
> https://github.com/leanprover/lean4/releases/tag/v4.34.1 and
> https://lean-lang.org/install/manual/ (the official path is elan; no official
> container is documented).

## Resource containment (Q-31, closed 2026-10-10)

Sandbox execution above is now a mechanism, not an aspiration
(`platform/kubona/eval/comparator_eval.py`, `platform/docker/Dockerfile.lean`):

- **Image:** `kubona-lean:4.34.1` built on `debian:bookworm-slim` from the
  checksummed `lean-4.34.1-linux.tar.zst` GitHub release asset, unprivileged
  user, `MALLOC_ARENA_MAX=2`; source stays vendored and is mounted at runtime
  (image carries the toolchain only). Build cache must be warmed at
  image-build time — runtime runs `--network none`, so an unwarmed first build
  would fail offline.
- **Caps on every run** (`LeanSandboxConfig`, unit-tested flag-for-flag):
  `--read-only`, `--network none`, `--cap-drop ALL`, `--memory/--memory-swap 4g`,
  `--cpus 2.0`, `--pids-limit 256`, `--tmpfs /tmp`, 600s timeout.
- **Precedence:** containerized lake → host lake → fail-closed AST containment
  with the reason recorded. Infra breakdowns (daemon/image/runtime) fall back;
  genuine proof failures NEVER degrade into a passing AST result (masked-failure
  guard on docker-level markers) — verified by test.
- **Host prerequisites (operator):** raise `vm.max_map_count` on Linux hosts
  (containers cannot raise it themselves); image build + one warm run before
  any production verification.
- **Build evidence (2026-11-18):** `kubona-lean:4.34.1` built on arm64 from the
  `linux_aarch64` release asset — `Lean (version 4.34.1,
  aarch64-unknown-linux-gnu, commit 5045d005…)`, `Lake version
  5.0.0-src+5045d00`; the hardened `--network none --read-only --cap-drop ALL`
  run reports the same Lean version. Manifest
  `sha256:e5effcbce273f8fc835b7608831b8f43f8fc32ce541bc72cab3b953e2a5e3a06`.
  The mathlib-scale warm and a live challenge have not run.
