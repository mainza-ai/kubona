# Kubona Lean 4 Comparator Sandbox (Q-31).
#
# Pinned to the vendored toolchain (submodules/math/lean/lean-toolchain):
#   leanprover/lean4:v4.34.1
#
# Why a tarball instead of a base image (investigated 2026-11-18):
# - Upstream Lean does NOT publish `leanprover/lean4` on Docker Hub: the
#   repository API returns 404, and the only community image
#   (`leanprovercommunity/lean4`) is a stale 2024 snapshot. The previous
#   `FROM leanprover/lean4:v4.34.1` could therefore never resolve.
# - Lean's official installation path is elan, and every GitHub release
#   publishes a checksummed Linux tarball. For a single-toolchain sandbox the
#   tarball is stricter: the image carries exactly one compiler, pinned by
#   SHA-256, with no registry or toolchain-manager indirection.
# - The version and both architecture checksums below MUST match the vendored
#   lean-toolchain; platform/tests/test_lean_sandbox_pin.py asserts they stay in
#   sync. BuildKit's TARGETARCH selects the x86_64 or aarch64 release asset, so
#   the image builds natively on both Intel and Apple Silicon hosts.
#
# Design notes (read before building):
# - The Lean *source* stays vendored in the repo and is mounted at runtime
#   (/workspace, see comparator_eval.verify_lean4_live sandbox path). This
#   image carries the toolchain only, so source changes need no rebuild.
# - First `lake exe` warms the build cache (dependency fetch + compile needs
#   NETWORK AT BUILD/WARM TIME). Runtime verification runs with --network none
#   (see comparator_eval.LeanSandboxConfig); warm the cache first:
#     scripts/build-lean-sandbox.sh --warm
# - Host prerequisite (Linux): vm.max_map_count must cover large proof
#   elaboration, e.g. `sysctl -w vm.max_map_count=1048576`. Containers cannot
#   raise it themselves. Docker Desktop (macOS) users: raise it in the VM.
# - Memory pressure inside Lean: MALLOC_ARENA_MAX=2 bounds glibc arena sprawl
#   next to the cgroup limit enforced at `docker run` (--memory/--memory-swap).
#
# Complies with the hardened sandbox profile: unprivileged user, read-only
# root at runtime (caller passes --read-only), no network at runtime.

FROM debian:bookworm-slim

ARG TARGETARCH
ARG LEAN_VERSION=4.34.1
# GitHub release asset digests for lean-4.34.1-linux{,_aarch64}.tar.zst.
ARG LEAN_LINUX_AMD64_SHA256=47bf4bbd78f70c2e9670598ab7124d92b6efb7330ff33e5fbb4030f6fd72e4e4
ARG LEAN_LINUX_ARM64_SHA256=fdb974c2cdb4627e090d5d4007b913e09d13c4868720fb5594e22808b3de9e37

LABEL maintainer="Kubona Systems Maintainer <team@kubona.org>"
LABEL org.opencontainers.image.title="kubona-lean"
LABEL org.opencontainers.image.description="Pinned Lean 4.34.1 toolchain sandbox for comparator proof verification"
LABEL org.opencontainers.image.version="${LEAN_VERSION}"
LABEL org.opencontainers.image.source="https://github.com/mainza-ai/kubona"

ENV MALLOC_ARENA_MAX=2 \
    PATH="/opt/lean/bin:${PATH}"

# git is required by Lake when it fetches package dependencies during the warm
# step; zstd unpacks the release artifact; libgmp10 is Lean's GMP runtime.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git libgmp10 zstd \
    && rm -rf /var/lib/apt/lists/*

RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) LEAN_ASSET_ARCH=""; LEAN_SHA256="${LEAN_LINUX_AMD64_SHA256}" ;; \
      arm64) LEAN_ASSET_ARCH="_aarch64"; LEAN_SHA256="${LEAN_LINUX_ARM64_SHA256}" ;; \
      *) echo "unsupported TARGETARCH='${TARGETARCH}' (need amd64 or arm64)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/lean.tar.zst \
        "https://github.com/leanprover/lean4/releases/download/v${LEAN_VERSION}/lean-${LEAN_VERSION}-linux${LEAN_ASSET_ARCH}.tar.zst"; \
    echo "${LEAN_SHA256}  /tmp/lean.tar.zst" | sha256sum -c -; \
    mkdir -p /opt/lean; \
    tar --zstd -xf /tmp/lean.tar.zst -C /opt/lean --strip-components=1; \
    rm /tmp/lean.tar.zst; \
    /opt/lean/bin/lean --version; \
    /opt/lean/bin/lake --version

# Unprivileged verification user (never run lake as root).
RUN useradd --create-home --shell /bin/bash lean \
    && mkdir -p /workspace \
    && chown lean:lean /workspace

USER lean
WORKDIR /workspace

ENTRYPOINT ["lake"]
CMD ["--version"]
