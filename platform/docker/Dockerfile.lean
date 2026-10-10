# Kubona Lean 4 Comparator Sandbox (Q-31).
#
# Pinned to the vendored toolchain (submodules/math/lean/lean-toolchain):
#   leanprover/lean4:v4.34.1
#
# Design notes (read before building):
# - The Lean *source* stays vendored in the repo and is mounted at runtime
#   (/workspace, see comparator_eval.verify_lean4_live sandbox path). This
#   image carries the toolchain only, so source changes need no rebuild.
# - First `lake exe` warms the build cache (dependency fetch + compile needs
#   NETWORK AT BUILD/WARM TIME). Runtime verification runs with --network none
#   (see comparator_eval.LeanSandboxConfig); warm the cache first:
#     docker build -f platform/docker/Dockerfile.lean -t kubona-lean:4.34.1 .
#     docker run --rm -v $PWD/submodules/math/lean:/workspace:rw -w /workspace \
#       kubona-lean:4.34.1 lake exe comparator --help
# - Host prerequisite (Linux): vm.max_map_count must cover large proof
#   elaboration, e.g. `sysctl -w vm.max_map_count=1048576`. Containers cannot
#   raise it themselves. Docker Desktop (macOS) users: raise it in the VM.
# - Memory pressure inside Lean: MALLOC_ARENA_MAX=2 bounds glibc arena sprawl
#   next to the cgroup limit enforced at `docker run` (--memory/--memory-swap).
#
# Complies with the hardened sandbox profile: unprivileged user, read-only
# root at runtime (caller passes --read-only), no network at runtime.

FROM leanprover/lean4:v4.34.1

LABEL maintainer="Kubona Systems Maintainer <team@kubona.org>"
LABEL org.opencontainers.image.title="kubona-lean"
LABEL org.opencontainers.image.description="Pinned Lean 4.34.1 toolchain sandbox for comparator proof verification"
LABEL org.opencontainers.image.version="4.34.1"
LABEL org.opencontainers.image.source="https://github.com/mainza-ai/kubona"

ENV MALLOC_ARENA_MAX=2 \
    ELAN_HOME=/home/lean/.elan \
    PATH="/home/lean/.elan/bin:${PATH}"

# Unprivileged verification user (never run lake as root).
RUN useradd --create-home --shell /bin/bash lean \
    && mkdir -p /workspace \
    && chown lean:lean /workspace

USER lean
WORKDIR /workspace

ENTRYPOINT ["lake"]
CMD ["--version"]
