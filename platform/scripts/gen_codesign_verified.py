"""Generate the versioned verified-results JSON for the RS decoder.

Reads NOTHING by hand: every T-count is measured live from the builders
(emitter-speed only — no trajectory sims; the proofs already ran at
slice-landing and are cited, not re-run). Fails loudly on any drift from
the pinned values, so regeneration is a verification act, not an edit.

Output: platform/core/pkg/codesign/verified.json (served verbatim by Go).
"""

from __future__ import annotations

import json
import os
import random
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(REPO, "platform"))

from kubona.codesign import F2_12  # noqa: E402
from kubona.codesign.bm import (  # noqa: E402
    berlekamp_massey,
    build_bm12_walk,
    build_phase_scan,
    check_decode,
    locator_roots,
    mask_cancellation,
    multi_error_syndromes,
)
from kubona.codesign.eea12 import build_eea12  # noqa: E402
from kubona.codesign.kickmix import Builder  # noqa: E402
from kubona.codesign.rs12 import _emit_omega12, build_rs_decoder, rs12_layout  # noqa: E402


def check(name: str, got: int, want: int) -> int:
    if got != want:
        raise SystemExit(f"DRIFT: {name} measured {got}, pinned {want}: refusing")
    print(f"  {name}: {got} (pin holds)")
    return got


def main() -> None:
    print("measuring (emitter-speed, no sims)...")

    _, walk_meta = build_bm12_walk()
    walk_t = check("bm_walk_70r", walk_meta["ccx"], 2422140)

    _, eea_meta = build_eea12()
    eea_t = check("eea_36b", eea_meta["ccx"], 1193586)

    rng = random.Random(50)
    masks = [rng.randrange(4096) for _ in range(4095)]
    rep = mask_cancellation(masks, F2_12)
    canceled = set(rep["canceled"])
    positions = [b for b in range(1, 4096) if b not in canceled]
    assert len(positions) == 4025, len(positions)
    nus = {b: rep["residual"][b] for b in positions}
    _, scan_regs = build_phase_scan(F2_12, 36, positions, nus)
    scan_t = check("phase_scan", scan_regs["ccx"], 217350)

    L = rs12_layout(0)
    # 112,168 addressable AND peak-live (Builder is alloc-aware): the older
    # "110,132" prose figure (slice-60 docstring, pre-F2) is stale — corrected
    # wherever it appears, with this generator as the live source.
    qubits = check("qubits", L["nqubits"], 112168)
    b = Builder(L["nqubits"])
    sig = L["WB"][0][:432]
    n_omega = _emit_omega12(b, [0], sig, L["S"], L["OMEGA"], L["OMBP"])
    omega_t = check("omega", n_omega, 666 * 144)

    _, meta = build_rs_decoder(positions, nus)
    total_t = check("decoder_total", meta["ccx"], 3928980)
    # Chien 85995 is asserted in-composition (rs12.py); reaching this line
    # IS its live re-verification.
    print("  chien_scan: 85995 (in-composition assert passed)")

    doc = {
        "program": "paper-scale RS decoder (Gu et al. arXiv:2610.22367v1, RS thread)",
        "version": 1,
        "generated": "2026-10-20",
        "generator": "platform/scripts/gen_codesign_verified.py (emitter-speed; proofs ran at slice-landing)",
        "labels": [
            "measured on simulator (kickmix, Aer-local)",
            "paper gains Anthropic-claimed until independently rerun",
            "components are standalone pins; the composed total is authoritative (shared scratch + inlined framing differ — they do not sum)",
        ],
        "total": {
            "toffoli": total_t,
            "qubits": qubits,
            "pin": "test_codesign_slice60.py::test_rs_composition_end_to_end",
        },
        "components": [
            {"name": "bm_walk_70r", "toffoli": walk_t, "scope": "standalone",
             "pin": "test_codesign_slice58.py:77 (70 x 34602, MBU mults)"},
            {"name": "eea_36b", "toffoli": eea_t, "scope": "standalone",
             "pin": "test_codesign_slice59.py:111 (MBU mults, full radius)"},
            {"name": "chien_scan", "toffoli": 85995, "scope": "composed-inlined",
             "pin": "rs12.py:352 in-composition assert"},
            {"name": "phase_scan", "toffoli": scan_t, "scope": "standalone paper-scale",
             "pin": "test_codesign_slice51.py:82 (54T/pos x 4025)"},
            {"name": "omega", "toffoli": omega_t, "scope": "structural (666 MBU mults)",
             "pin": "rs12.py:189 + generator-measured return"},
        ],
        "proofs": [
            "slice60 end-to-end (FLAG=1, Chien marks exactly planted roots, composed phase == standalone phase, A*E+B*O=1, inputs preserved, temps clean)",
            "slice59/F1 EEA 60/60 converge + explicit-refusal flag",
            "slice58 10 full planted walks end-to-end (inputs preserved, temps clean)",
            "slice57 700 harvested round-sims (inputs preserved, temps clean)",
        ],
    }
    out = os.path.join(REPO, "core", "pkg", "codesign", "verified.json")
    with open(out, "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print(f"wrote {out}")


if __name__ == "__main__":
    main()
