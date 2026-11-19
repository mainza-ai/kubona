"""Pin consistency for the Lean comparator sandbox (Q-31).

Upstream publishes no official `leanprover/lean4` Docker image (the Docker Hub
repository 404s; only a stale community image exists). The sandbox therefore
installs the checksummed GitHub release tarball. These assertions keep the
vendored toolchain, the Dockerfile pin, the checksum, and the runtime image tag
from drifting apart.
"""

from pathlib import Path
import re

from kubona.eval.comparator_eval import LeanSandboxConfig

ROOT = Path(__file__).resolve().parents[2]
TOOLCHAIN = ROOT / "submodules" / "math" / "lean" / "lean-toolchain"
DOCKERFILE = ROOT / "platform" / "docker" / "Dockerfile.lean"
BUILD_SCRIPT = ROOT / "scripts" / "build-lean-sandbox.sh"


def _pinned_version() -> str:
    value = TOOLCHAIN.read_text().strip()
    match = re.fullmatch(r"leanprover/lean4:v(.+)", value)
    assert match, f"unexpected lean-toolchain contents: {value!r}"
    return match.group(1)


def test_dockerfile_pins_the_vendored_toolchain():
    version = _pinned_version()
    text = DOCKERFILE.read_text()
    arg = re.search(r"^ARG LEAN_VERSION=(\S+)$", text, re.MULTILINE)
    assert arg, "Dockerfile must pin ARG LEAN_VERSION"
    assert arg.group(1) == version, (
        f"Dockerfile pins {arg.group(1)} but lean-toolchain is {version}"
    )


def test_dockerfile_does_not_use_the_nonexistent_hub_image():
    text = DOCKERFILE.read_text()
    assert not re.search(r"^FROM\s+leanprover/lean4", text, re.MULTILINE), (
        "leanprover/lean4 is not a published image; install the release tarball"
    )
    version = _pinned_version()
    assert "lean-${LEAN_VERSION}-linux${LEAN_ASSET_ARCH}.tar.zst" in text or (
        f"lean-{version}-linux.tar.zst" in text
    ), "Dockerfile must download the pinned Linux release tarball"
    assert "sha256sum -c -" in text, "the downloaded artifact must be checksum-verified"


def test_release_checksums_are_pinned_per_architecture():
    text = DOCKERFILE.read_text()
    amd64 = re.search(r"^ARG LEAN_LINUX_AMD64_SHA256=([0-9a-f]{64})$", text, re.MULTILINE)
    arm64 = re.search(r"^ARG LEAN_LINUX_ARM64_SHA256=([0-9a-f]{64})$", text, re.MULTILINE)
    assert amd64, "Dockerfile must pin the x86_64 release digest"
    assert arm64, "Dockerfile must pin the aarch64 release digest"
    # GitHub release asset digests for lean-4.34.1-linux{,_aarch64}.tar.zst.
    assert amd64.group(1) == "47bf4bbd78f70c2e9670598ab7124d92b6efb7330ff33e5fbb4030f6fd72e4e4"
    assert arm64.group(1) == "fdb974c2cdb4627e090d5d4007b913e09d13c4868720fb5594e22808b3de9e37"


def test_dockerfile_selects_the_asset_by_target_arch():
    text = DOCKERFILE.read_text()
    assert "ARG TARGETARCH" in text
    assert 'arm64) LEAN_ASSET_ARCH="_aarch64"' in text
    assert 'amd64) LEAN_ASSET_ARCH=""' in text
    assert "unsupported TARGETARCH" in text, "unknown architectures must fail the build"
    assert "linux${LEAN_ASSET_ARCH}.tar.zst" in text


def test_runtime_image_tag_matches_the_pin():
    version = _pinned_version()
    assert LeanSandboxConfig().image == f"kubona-lean:{version}"


def test_comparator_and_landrun_are_pinned():
    text = DOCKERFILE.read_text()
    assert "ARG LANDRUN_VERSION=0.1.17" in text
    assert "LANDRUN_AMD64_SHA256=6ada66a06669e8994e174a7271af2db636308e55a0d6ec896cc7d326b46727f6" in text
    assert "LANDRUN_ARM64_SHA256=365d8c8656c732b14d70e621d972cb733e490621fb5b9c0876376bcf457dbb5d" in text
    assert "ARG COMPARATOR_COMMIT=d03acab154d269c06e60e4de7e4cc85deebff94b" in text
    assert "ARG LEAN4EXPORT_COMMIT=076e8e57707e813375e8f9da8bf989799ace9680" in text
    assert "lake build lean4export comparator" in text
    assert "sed -i" in text and "lakefile.toml" in text, (
        "the comparator manifest must be regenerated from the pinned lean4export rev"
    )

def test_build_script_reads_the_pin_and_verifies_the_image():
    text = BUILD_SCRIPT.read_text()
    assert "lean-toolchain" in text
    assert '"--build-arg" "LEAN_VERSION=$VERSION"' in text or "LEAN_VERSION=$VERSION" in text
    assert "--entrypoint lean" in text, "the script must verify the installed compiler"
    assert "--entrypoint lake" in text, "the script must verify Lake is installed"
