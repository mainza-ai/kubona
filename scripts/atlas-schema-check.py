#!/usr/bin/env python3
"""Atlas live-schema drift check (manual — needs network, never CI).

Fetches the live API-reference pages and asserts the contract tokens our
integration is pinned to. A failure means the beta schema moved: update
platform/core + console payload builders, the wiki source page, and the
Q-38 probe before the next paid call.

Usage: python3 scripts/atlas-schema-check.py
Exit: 0 all pinned tokens present · 1 drift detected (prints missing).
"""

from __future__ import annotations

import sys
import urllib.request

BASE = "https://atlas-beta.worldlabs.ai/docs/api-reference"

# page -> tokens our code depends on (with the code site in comments).
CHECKS: dict[str, list[str]] = {
    # observatory_lens.go default branch (PinholeCamera shape)
    "atlas-chisel": ["extrinsics", "quaternion", "coordinateSystem", "intrinsics", "1280", "voxelSize", "returnDepth", "purgeContentOnOperationCompletion"],
    # observatory_lens.go generate refusal + flip-pair context
    "atlas-generate": ["contextFrames", "imageAsset", "depth", "targetCameras", "returnDepth", "promptUsed", "base64Outputs", "Idempotency-Key"],
    # Q-37 snapshot path (inline asset inputs)
    "images2-posed-rgbd": ["imageAsset", "assetId", "targetResolution", "refinePoses", "canonicalizeToFirstFrame", "Idempotency-Key"],
    # T2I probe (when the outage lifts)
    "atlas-text-to-image": ["aspectRatio", "enhancePrompt", "numSamples", "returnDepth"],
    # atlasMasked reference: schema shape only (gate lives on the guide)
    "atlas-masked": ["maskAsset", "targetCameras", "returnDepth", "enhancePrompt", "Idempotency-Key"],
    # masked stays gated: the guide carries the access/404 language
    "inpainting": ["requires access", "Access-dependent preview"],
    # P6 verifier + resolve rail (case-insensitive: slugs lowercase tokens)
    "operations": ["done", "webhookdelivery"],
    "webhooks": ["v1a", "webhook-id", "webhook-signature", "webhook-timestamp"],
}

PAGES = {
    "atlas-chisel": f"{BASE}/atlas-chisel",
    "atlas-generate": f"{BASE}/atlas-generate",
    "images2-posed-rgbd": f"{BASE}/images2-posed-rgbd",
    "atlas-text-to-image": f"{BASE}/atlas-text-to-image",
    "atlas-masked": f"{BASE}/atlas-masked",
    "inpainting": "https://atlas-beta.worldlabs.ai/docs/inpainting",
    "operations": "https://atlas-beta.worldlabs.ai/docs/operations",
    "webhooks": "https://atlas-beta.worldlabs.ai/docs/webhooks",
}


def fetch(url: str) -> str:
    req = urllib.request.Request(url, headers={"User-Agent": "kubona-schema-check/1.0"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return resp.read().decode(errors="replace")


def main() -> int:
    drifted = 0
    for key, url in PAGES.items():
        try:
            text = fetch(url)
        except Exception as e:  # noqa: BLE001 — reported, not fatal until the end
            print(f"FETCH-FAIL {key}: {e}")
            drifted += 1
            continue
        lowered = text.lower()
        missing = [t for t in CHECKS[key] if t.lower() not in lowered]
        if missing:
            print(f"DRIFT {key}: missing {missing} ({url})")
            drifted += 1
        else:
            print(f"OK {key} ({len(CHECKS[key])} tokens)")
    if drifted:
        print(f"\n{drifted} page(s) drifted or unreachable — review before paid calls.")
        return 1
    print("\nall pinned tokens present.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
