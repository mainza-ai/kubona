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
# Comparator (investigated 2026-11-18):
# - `comparator` was merged into Lean core at v4.35.0 (`lake comparator`). This
#   corpus is pinned to v4.34.1, so the sandbox installs the archived
#   `leanprover/comparator` at tag v4.34.0 (the last 4.34-line release) and
#   pins its `lean4export` dependency to the matching v4.34.0 commit.
# - The README requires `landrun` (inner Landlock sandbox) and `lean4export` on
#   PATH; both are installed below. `comparator` passes `--best-effort` to
#   landrun, so kernels older than Landlock ABI v9 still work.
# - The Debian trixie base is required: the landrun release binary needs
#   glibc >= 2.38.
#
# Design notes (read before building):
# - The Lean *source* stays vendored in the repo and is mounted at runtime
#   (/workspace, see comparator_eval.verify_lean4_live sandbox path). This
#   image carries the toolchain and comparator tools only.
# - First `lake exe cache get` warms the build cache (network required, run it
#   through scripts/build-lean-sandbox.sh --warm). Runtime verification runs
#   with --network none.
# - Host prerequisite (Linux): vm.max_map_count must cover large proof
#   elaboration, e.g. `sysctl -w vm.max_map_count=1048576`. Containers cannot
#   raise it themselves. Docker Desktop (macOS) users: raise it in the VM.
# - Memory pressure inside Lean: MALLOC_ARENA_MAX=2 bounds glibc arena sprawl
#   next to the cgroup limit enforced at `docker run` (--memory/--memory-swap).
#
# Complies with the hardened sandbox profile: unprivileged user, read-only
# root at runtime (caller passes --read-only), no network at runtime.

FROM debian:trixie-slim

ARG TARGETARCH
ARG LEAN_VERSION=4.34.1
# GitHub release asset digests for lean-4.34.1-linux{,_aarch64}.tar.zst.
ARG LEAN_LINUX_AMD64_SHA256=47bf4bbd78f70c2e9670598ab7124d92b6efb7330ff33e5fbb4030f6fd72e4e4
ARG LEAN_LINUX_ARM64_SHA256=fdb974c2cdb4627e090d5d4007b913e09d13c4868720fb5594e22808b3de9e37
# landrun v0.1.17 release asset digests (Linux binaries).
ARG LANDRUN_VERSION=0.1.17
ARG LANDRUN_AMD64_SHA256=6ada66a06669e8994e174a7271af2db636308e55a0d6ec896cc7d326b46727f6
ARG LANDRUN_ARM64_SHA256=365d8c8656c732b14d70e621d972cb733e490621fb5b9c0876376bcf457dbb5d
# Archived comparator tag v4.34.0 and the lean4export v4.34.0 commit it must use.
ARG COMPARATOR_COMMIT=d03acab154d269c06e60e4de7e4cc85deebff94b
ARG LEAN4EXPORT_COMMIT=076e8e57707e813375e8f9da8bf989799ace9680

LABEL maintainer="Kubona Systems Maintainer <team@kubona.org>"
LABEL org.opencontainers.image.title="kubona-lean"
LABEL org.opencontainers.image.description="Pinned Lean 4.34.1 comparator sandbox for proof verification"
LABEL org.opencontainers.image.version="${LEAN_VERSION}"
LABEL org.opencontainers.image.source="https://github.com/mainza-ai/kubona"

ENV MALLOC_ARENA_MAX=2 \
    PATH="/opt/lean/bin:/usr/local/bin:${PATH}"

# git is required by Lake when it fetches package dependencies during the warm
# step; zstd unpacks the release artifact; libgmp10 is Lean's GMP runtime.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl gcc git libc6-dev libgmp-dev libgmp10 zstd \
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

# Inner Landlock sandbox used by comparator for adversarial solution builds.
RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) LANDRUN_ASSET="landrun-linux-amd64"; LANDRUN_SHA256="${LANDRUN_AMD64_SHA256}" ;; \
      arm64) LANDRUN_ASSET="landrun-linux-arm64"; LANDRUN_SHA256="${LANDRUN_ARM64_SHA256}" ;; \
      *) echo "unsupported TARGETARCH='${TARGETARCH}' (need amd64 or arm64)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/landrun \
        "https://github.com/Zouuup/landrun/releases/download/v${LANDRUN_VERSION}/${LANDRUN_ASSET}"; \
    echo "${LANDRUN_SHA256}  /usr/local/bin/landrun" | sha256sum -c -; \
    chmod 0755 /usr/local/bin/landrun; \
    landrun --version

# Build the archived comparator (v4.34.0) and its matching lean4export, then
# keep only the binaries. `lake update` regenerates comparator's manifest from
# the pin, so the build cannot silently follow lean4export master.
RUN set -eu; \
    git clone --filter=blob:none https://github.com/leanprover/comparator /opt/comparator-src; \
    cd /opt/comparator-src; \
    git checkout "${COMPARATOR_COMMIT}"; \
    sed -i "s|rev = \"master\"|rev = \"${LEAN4EXPORT_COMMIT}\"|" lakefile.toml; \
    lake update; \
    lake build lean4export comparator; \
    cp .lake/build/bin/comparator /usr/local/bin/comparator; \
    cp .lake/packages/lean4export/.lake/build/bin/lean4export /usr/local/bin/lean4export; \
    cd /; \
    rm -rf /opt/comparator-src; \
    test -x /usr/local/bin/comparator; \
    test -x /usr/local/bin/lean4export

# Unprivileged verification user (never run lake as root).
RUN useradd --create-home --shell /bin/bash lean \
    && mkdir -p /workspace \
    && chown lean:lean /workspace

USER lean
WORKDIR /workspace

ENTRYPOINT ["lake"]
CMD ["--version"]
