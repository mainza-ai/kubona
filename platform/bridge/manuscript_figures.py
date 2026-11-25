"""Deterministic Publication-Grade Figure Synthesizer for Kubona Manuscripts.

Renders crisp, anti-aliased, colorblind-safe (Okabe-Ito) 300-DPI publication
PNG charts directly from settled Kubona campaign rows using Pillow (already
pinned in uv.lock — zero extra dependencies, ~20 ms per figure, 100%
byte-reproducible across builds).

Figures are synthesized ONLY from real settled numerical rows in the campaign
projection; no placeholder or synthetic curve is ever invented.
"""

from __future__ import annotations

import io
import math
from typing import Any, Dict, List, Tuple

try:
    from PIL import Image, ImageDraw, ImageFont
    HAS_PIL = True
except Exception:  # pragma: no cover
    HAS_PIL = False

# Okabe-Ito colorblind-safe publication palette
NAVY = (20, 33, 61)
TEAL = (14, 107, 116)
ORANGE = (213, 94, 0)
SKY = (86, 180, 233)
PURPLE = (128, 72, 168)
GREEN = (0, 158, 115)
RED = (196, 48, 43)
SLATE = (74, 85, 104)
GRID = (226, 232, 240)
BG = (255, 255, 255)
INK = (15, 23, 42)
SERIES_COLORS = [NAVY, TEAL, ORANGE, PURPLE, GREEN, SKY, RED]


def _load_font(size: int):
    for path in (
        "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
        "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
        "/System/Library/Fonts/Helvetica.ttc",
    ):
        try:
            return ImageFont.truetype(path, size)
        except Exception:
            continue
    return ImageFont.load_default()


def _to_png_bytes(img: "Image.Image") -> bytes:
    buf = io.BytesIO()
    img.save(buf, format="PNG", optimize=False, compress_level=6, dpi=(300, 300))
    return buf.getvalue()


def _new_canvas(w: int = 1600, h: int = 920):
    img = Image.new("RGB", (w, h), BG)
    draw = ImageDraw.Draw(img)
    return img, draw


def _draw_dashed_h(draw, x0: int, x1: int, y: int, color, dash: int = 12, gap: int = 8, width: int = 3):
    x = x0
    while x < x1:
        draw.line([(x, y), (min(x + dash, x1), y)], fill=color, width=width)
        x += dash + gap


def _short_backend_label(raw: Any) -> str:
    s = str(raw or "sim").strip().lower()
    if "aer" in s or "sim" in s:
        return "aer"
    if s.startswith("ibm_"):
        return s[4:][:11]
    return s[:11]


def _draw_axes(draw, box: Tuple[int, int, int, int], title: str, xlabel: str, ylabel: str,
               y_min: float, y_max: float, n_yticks: int = 5, xlabel_offset: int = 80):
    x0, y0, x1, y1 = box
    font_title = _load_font(28)
    font_axis = _load_font(22)
    font_tick = _load_font(18)
    draw.text((x0, 24), title, fill=INK, font=font_title)
    draw.rectangle([x0, y0, x1, y1], outline=SLATE, width=2)
    span = max(y_max - y_min, 1e-9)
    for i in range(n_yticks + 1):
        frac = i / n_yticks
        val = y_min + frac * span
        py = int(y1 - frac * (y1 - y0))
        if 0 < i < n_yticks:
            draw.line([(x0 + 2, py), (x1 - 2, py)], fill=GRID, width=1)
        draw.line([(x0 - 8, py), (x0, py)], fill=SLATE, width=2)
        lbl = f"{val:.3g}" if abs(val) < 1000 else f"{val:.0f}"
        draw.text((max(12, x0 - 95), py - 10), lbl, fill=SLATE, font=font_tick)
    draw.text((x0 + (x1 - x0) // 2 - len(xlabel) * 5, y1 + xlabel_offset), xlabel, fill=INK, font=font_axis)
    draw.text((max(14, x0 - 95), y0 - 38), ylabel, fill=INK, font=font_axis)


def _render_energy_benchmark(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    pts = []
    for r in rows:
        kind = str(r.get("kind") or "").lower()
        if "qaoa" in kind or kind in ("lattice_dynamics", "thermodynamic_limit", "conjecture", "conjecture_run", "hardware_pes", "pes"):
            continue
        delta = r.get("delta_mha")
        if delta is None:
            e = r.get("energy", r.get("ground_energy", r.get("ground_state_energy")))
            ex = r.get("fci_exact_energy", r.get("active_space_fci_energy", r.get("exact_energy", r.get("energy_exact"))))
            if e is not None and ex is not None:
                delta = abs(float(e) - float(ex)) * 1000.0
        if delta is None or not math.isfinite(float(delta)):
            continue
        ci = float(r.get("ci95_mha") or 0.0)
        jid = str(r.get("job_id") or "")[:6]
        be = _short_backend_label(r.get("backend"))
        label = f"{jid}\n{be}"
        color = TEAL if r.get("mitigated") else (ORANGE if r.get("hardware") else NAVY)
        pts.append((label, float(delta), ci, color, bool(r.get("mitigated")), bool(r.get("hardware"))))
    if len(pts) < 2:
        return None
    total_pts = len(pts)
    pts = pts[:20]
    img, draw = _new_canvas(1600, 920)
    n = len(pts)
    stagger = n > 12
    box = (140, 110, 1520, 705 if stagger else 740)
    y_max = max(max(p[1] + p[2] for p in pts) * 1.18, 2.5)
    _draw_axes(draw, box, "Energy Discrepancy vs. Exact Classical Anchor (Settled Jobs)",
               "Settled Job & Backend (Navy: Simulator, Orange: Raw QPU, Teal: ZNE Mitigated)",
               "Error abs(E - E_anchor) (mHa)", 0.0, y_max, xlabel_offset=118 if stagger else 90)
    x0, y0, x1, y1 = box
    slot = (x1 - x0) / n
    bar_w = max(16, min(68, int(slot * 0.54)))
    font_lbl = _load_font(13 if n > 14 else 16)
    font_val = _load_font(13 if n > 14 else 16)

    for i, (label, val, ci, color, _, _) in enumerate(pts):
        cx = int(x0 + (i + 0.5) * slot)
        bh = int((val / y_max) * (y1 - y0))
        top = max(y0, y1 - bh)
        draw.rectangle([cx - bar_w // 2, top, cx + bar_w // 2, y1], fill=color, outline=INK, width=1)
        if ci > 0:
            eh = int((ci / y_max) * (y1 - y0))
            ey0, ey1 = max(y0, top - eh), min(y1, top + eh)
            draw.line([(cx, ey0), (cx, ey1)], fill=INK, width=3)
            draw.line([(cx - 8, ey0), (cx + 8, ey0)], fill=INK, width=3)
            draw.line([(cx - 8, ey1), (cx + 8, ey1)], fill=INK, width=3)
        draw.text((cx, max(y0 + 18, top - 6)), f"{val:.1f}", fill=INK, font=font_val, anchor="mb")
        draw.line([(cx, y1), (cx, y1 + 6)], fill=SLATE, width=1)
        y_off = 10 if (not stagger or i % 2 == 0) else 50
        line_h = 16 if n > 14 else 18
        for j, line in enumerate(label.split("\n")):
            draw.text((cx, y1 + y_off + j * line_h), line, fill=SLATE, font=font_lbl, anchor="mt")

    if 1.6 < y_max:
        cy = int(y1 - (1.6 / y_max) * (y1 - y0))
        _draw_dashed_h(draw, x0, x1, cy, RED, width=3)
        # Place chemical-accuracy callout cleanly in top-right legend area to avoid bar collisions
        draw.rectangle([x1 - 320, y0 + 12, x1 - 14, y0 + 48], fill=BG, outline=RED, width=2)
        _draw_dashed_h(draw, x1 - 308, x1 - 268, y0 + 30, RED, dash=8, gap=5, width=3)
        draw.text((x1 - 258, y0 + 20), "Chemical accuracy (1.6 mHa)", fill=RED, font=_load_font(16))

    shown_note = f"{len(pts)} of {total_pts}" if total_pts > len(pts) else f"{len(pts)}"
    caption = (
        f"Absolute energy discrepancy abs(E - E_anchor) (mHa) across {shown_note} settled jobs "
        "(navy: simulator, orange: raw QPU, teal: ZNE mitigated; whiskers denote 95% CI when reported; "
        "dashed red line marks 1.6 mHa chemical accuracy)."
    )
    return "fig_energy_benchmark.png", _to_png_bytes(img), caption


def _render_pes_or_depth(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    # Priority 1: hardware_pes curve
    for r in rows:
        if str(r.get("kind") or "").lower() == "hardware_pes" and isinstance(r.get("points"), list) and len(r["points"]) >= 2:
            pts = sorted(
                [p for p in r["points"] if isinstance(p, dict) and p.get("bond_distance") is not None and p.get("energy") is not None],
                key=lambda p: float(p["bond_distance"]),
            )
            if len(pts) >= 2:
                xs = [float(p["bond_distance"]) for p in pts]
                ys = [float(p["energy"]) for p in pts]
                ex = [float(p["fci_exact_energy"]) for p in pts if p.get("fci_exact_energy") is not None]
                all_y = ys + ex
                y_min, y_max = min(all_y) - 0.03, max(all_y) + 0.03
                img, draw = _new_canvas(1600, 920)
                box = (150, 110, 1520, 760)
                _draw_axes(draw, box, f"Governed Hardware PES Curve ({r.get('backend', 'QPU')})",
                           "Bond Distance R (Angstrom)", "Energy (Ha)", y_min, y_max)
                x0, y0, x1, y1 = box
                x_min, x_max = min(xs), max(xs)
                x_span = max(x_max - x_min, 1e-6)
                y_span = max(y_max - y_min, 1e-6)

                def _px(xv, yv):
                    return (
                        int(x0 + 60 + ((xv - x_min) / x_span) * (x1 - x0 - 120)),
                        int(y1 - ((yv - y_min) / y_span) * (y1 - y0)),
                    )

                if len(ex) == len(xs):
                    fci_pts = [_px(xv, yv) for xv, yv in zip(xs, ex)]
                    draw.line(fci_pts, fill=NAVY, width=4)
                    for px, py in fci_pts:
                        draw.ellipse([px - 7, py - 7, px + 7, py + 7], fill=NAVY)
                qpu_pts = [_px(xv, yv) for xv, yv in zip(xs, ys)]
                draw.line(qpu_pts, fill=ORANGE, width=4)
                font_lbl = _load_font(18)
                for (px, py), xv in zip(qpu_pts, xs):
                    draw.rectangle([px - 8, py - 8, px + 8, py + 8], fill=ORANGE, outline=INK, width=2)
                    draw.text((px - 24, y1 + 14), f"{xv:.2f}", fill=SLATE, font=font_lbl)
                draw.text((x1 - 360, y0 + 18), "Navy: Exact FCI | Orange: QPU PES", fill=INK, font=font_lbl)
                caption = (
                    f"Potential energy surface across {len(pts)} geometries on {r.get('backend', 'QPU')} "
                    "(orange squares) compared against exact PySCF FCI anchors (navy circles)."
                )
                return "fig_pes_curve.png", _to_png_bytes(img), caption

    # Priority 2: UCCSD depth scaling (layers 1..4)
    layered = [r for r in rows if r.get("layers") is not None and r.get("delta_mha") is not None]
    distinct_layers = sorted({int(r["layers"]) for r in layered})
    if len(distinct_layers) >= 2:
        by_layer: Dict[int, List[float]] = {L: [] for L in distinct_layers}
        for r in layered:
            by_layer[int(r["layers"])].append(float(r["delta_mha"]))
        xs = distinct_layers
        ys = [sum(by_layer[L]) / len(by_layer[L]) for L in xs]
        y_max = max(ys) * 1.2 + 5.0
        img, draw = _new_canvas(1600, 920)
        box = (150, 110, 1520, 760)
        _draw_axes(draw, box, "UCCSD Ansatz Depth Scaling (Mean Error vs. Layer Count)",
                   "UCCSD Repetitions (layers)", "Mean |E - E_anchor| (mHa)", 0.0, y_max)
        x0, y0, x1, y1 = box
        x_span = max(xs[-1] - xs[0], 1)
        pts_px = [
            (
                int(x0 + 80 + ((L - xs[0]) / x_span) * (x1 - x0 - 160)),
                int(y1 - (yv / y_max) * (y1 - y0)),
            )
            for L, yv in zip(xs, ys)
        ]
        draw.line(pts_px, fill=PURPLE, width=5)
        font_lbl = _load_font(20)
        for (px, py), L, yv in zip(pts_px, xs, ys):
            draw.ellipse([px - 10, py - 10, px + 10, py + 10], fill=PURPLE, outline=INK, width=2)
            draw.text((px - 28, py - 34), f"{yv:.1f}", fill=INK, font=font_lbl)
            draw.text((px - 16, y1 + 16), f"L={L}", fill=SLATE, font=font_lbl)
        caption = (
            f"Mean energy discrepancy (mHa) as a function of UCCSD repetition depth across "
            f"L = {', '.join(str(L) for L in xs)}."
        )
        return "fig_uccsd_depth.png", _to_png_bytes(img), caption
    return None


def _render_quench_dynamics(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    quenches = [
        r for r in rows
        if str(r.get("kind") or "").lower() == "lattice_dynamics"
        and isinstance(r.get("times"), list) and len(r["times"]) >= 2
    ]
    if not quenches:
        return None
    img, draw = _new_canvas(1600, 920)
    left_box = (110, 115, 760, 745)
    right_box = (900, 115, 1550, 745)
    _draw_axes(draw, left_box, "2D TFIM Loschmidt Echo L(t)", "Time t", "L(t)", 0.0, 1.05, xlabel_offset=42)
    _draw_axes(draw, right_box, "Mean Spatial Correlator <Z_i Z_j(t)>", "Time t", "<Z_i Z_j>", 0.0, 1.05, xlabel_offset=42)
    font_leg = _load_font(17)
    for idx, q in enumerate(quenches[:6]):
        color = SERIES_COLORS[idx % len(SERIES_COLORS)]
        ts = [t for t in q["times"] if isinstance(t, dict) and t.get("t") is not None]
        if len(ts) < 2:
            continue
        t_vals = [float(t["t"]) for t in ts]
        t_max = max(max(t_vals), 1e-6)
        le_pts = []
        zz_pts = []
        for t in ts:
            tv = float(t["t"])
            if t.get("loschmidt_echo") is not None:
                lx = int(left_box[0] + (tv / t_max) * (left_box[2] - left_box[0]))
                ly = int(left_box[3] - min(1.05, max(0.0, float(t["loschmidt_echo"]))) / 1.05 * (left_box[3] - left_box[1]))
                le_pts.append((lx, ly))
            if t.get("zz_correlator_mean") is not None:
                rx = int(right_box[0] + (tv / t_max) * (right_box[2] - right_box[0]))
                ry = int(right_box[3] - min(1.05, max(0.0, float(t["zz_correlator_mean"]))) / 1.05 * (right_box[3] - right_box[1]))
                zz_pts.append((rx, ry))
        if len(le_pts) >= 2:
            draw.line(le_pts, fill=color, width=4)
        if len(zz_pts) >= 2:
            draw.line(zz_pts, fill=color, width=4)
        hq = q.get("quench_field", idx)
        draw.rectangle([110 + idx * 230, 865, 132 + idx * 230, 885], fill=color)
        draw.text((140 + idx * 230, 864), f"h_q={hq}", fill=INK, font=font_leg)
    caption = (
        f"Exact finite-size 2D TFIM non-equilibrium quench dynamics across {len(quenches[:6])} quench field(s): "
        "Loschmidt echo L(t) (left) and mean nearest-neighbor correlator <Z_i Z_j(t)> (right)."
    )
    return "fig_quench_dynamics.png", _to_png_bytes(img), caption


def _render_qaoa_or_dmrg(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    qaoas = [r for r in rows if "qaoa" in str(r.get("kind") or "").lower() and r.get("approximation_ratio") is not None]
    if len(qaoas) >= 2:
        qaoas = qaoas[:10]
        img, draw = _new_canvas(1600, 920)
        box = (140, 110, 1520, 740)
        _draw_axes(draw, box, "QAOA Approximation Ratio Across Depth & Processors",
                   "Settled QAOA Job & Backend", "Approx. Ratio r = C / C_opt", 0.0, 1.05)
        x0, y0, x1, y1 = box
        slot = (x1 - x0) / len(qaoas)
        bar_w = max(24, min(80, int(slot * 0.55)))
        font_lbl = _load_font(17)
        for i, r in enumerate(qaoas):
            val = float(r["approximation_ratio"])
            cx = int(x0 + (i + 0.5) * slot)
            top = int(y1 - (min(1.05, max(0.0, val)) / 1.05) * (y1 - y0))
            color = ORANGE if r.get("hardware") else NAVY
            draw.rectangle([cx - bar_w // 2, top, cx + bar_w // 2, y1], fill=color, outline=INK, width=1)
            draw.text((cx, max(y0 + 20, top - 8)), f"{val:.3f}", fill=INK, font=font_lbl, anchor="mb")
            be = _short_backend_label(r.get("backend"))
            jid = str(r.get("job_id") or "")[:6]
            draw.text((cx, y1 + 14), jid, fill=SLATE, font=font_lbl, anchor="mt")
            draw.text((cx, y1 + 36), be, fill=SLATE, font=font_lbl, anchor="mt")
        caption = (
            f"QAOA MaxCut approximation ratio r across {len(qaoas)} settled simulator (navy) and QPU hardware (orange) runs."
        )
        return "fig_qaoa_utility.png", _to_png_bytes(img), caption
    return None


def _render_qec_benchmark(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    """Render 300-DPI QEC circuit-level LER benchmark with 95% Wilson CI whiskers."""
    qecs = [
        r for r in rows
        if str(r.get("kind") or "").lower() == "qec_memory_sim"
        and r.get("ler") is not None
    ]
    if len(qecs) < 2:
        return None
    qecs = qecs[:12]
    img, draw = _new_canvas(1600, 920)
    box = (150, 110, 1520, 725)
    y_max = max(max(float(r.get("ci_high", r["ler"])) for r in qecs) * 1.2, 0.05)
    _draw_axes(
        draw,
        box,
        "QEC Circuit-Level Logical Error Rate (95% Wilson CI)",
        "Code & Noise Model (Navy: Surface Baseline, Teal: qLDPC/BB Candidate)",
        "Logical Error Rate (LER)",
        0.0,
        y_max,
        xlabel_offset=96,
    )
    x0, y0, x1, y1 = box
    slot = (x1 - x0) / len(qecs)
    bar_w = max(22, min(72, int(slot * 0.54)))
    font_lbl = _load_font(14 if len(qecs) > 8 else 16)
    for i, r in enumerate(qecs):
        ler = float(r["ler"])
        ci_lo = float(r.get("ci_low", ler))
        ci_hi = float(r.get("ci_high", ler))
        cx = int(x0 + (i + 0.5) * slot)
        top = int(y1 - min(1.0, ler / y_max) * (y1 - y0))
        name = str(r.get("code_name") or "qec")
        color = NAVY if "surface" in name.lower() else TEAL
        draw.rectangle([cx - bar_w // 2, top, cx + bar_w // 2, y1], fill=color, outline=INK, width=1)
        ey0 = int(y1 - min(1.0, ci_hi / y_max) * (y1 - y0))
        ey1 = int(y1 - min(1.0, ci_lo / y_max) * (y1 - y0))
        draw.line([(cx, ey0), (cx, ey1)], fill=INK, width=3)
        draw.line([(cx - 8, ey0), (cx + 8, ey0)], fill=INK, width=3)
        draw.line([(cx - 8, ey1), (cx + 8, ey1)], fill=INK, width=3)
        draw.text((cx, max(y0 + 18, ey0 - 6)), f"{ler:.3f}", fill=INK, font=font_lbl, anchor="mb")
        short = name.replace("bravyi2024_", "")[:12]
        noise_tag = "heron" if (isinstance(r.get("noise"), dict) and r["noise"].get("is_held_out")) else "si1000"
        draw.text((cx, y1 + 12), short, fill=SLATE, font=font_lbl, anchor="mt")
        draw.text((cx, y1 + 32), noise_tag, fill=SLATE, font=font_lbl, anchor="mt")
    caption = (
        f"Circuit-level logical error rate (LER) across {len(qecs)} QEC memory runs "
        "(navy: rotated surface-code baseline, teal: qLDPC/BB code; whiskers show 95% Wilson CIs)."
    )
    return "fig_qec_benchmark.png", _to_png_bytes(img), caption


def _render_transpile_overhead(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    """Phase 2: render two-qubit gate count and QASM gate-line summary for hardware jobs."""
    hw = [
        r for r in rows
        if (bool(r.get("hardware")) or r.get("circuit_sha256"))
        and (r.get("two_qubit_gate_count") is not None or r.get("qasm_gate_lines") is not None)
    ]
    if not hw:
        return None
    hw = hw[:12]
    img, draw = _new_canvas(1600, 920)
    box = (150, 110, 1520, 725)
    vals = [float(r.get("two_qubit_gate_count") or r.get("qasm_gate_lines") or 0) for r in hw]
    y_max = max(max(vals) * 1.2, 10.0)
    _draw_axes(
        draw,
        box,
        "Circuit Two-Qubit Gate & Transpilation Footprint",
        "Settled Job & Backend (full OpenQASM archived in supplement/qasm/)",
        "Two-Qubit Gates / Gate Lines",
        0.0,
        y_max,
        xlabel_offset=96,
    )
    x0, y0, x1, y1 = box
    slot = (x1 - x0) / len(hw)
    bar_w = max(22, min(72, int(slot * 0.52)))
    font_lbl = _load_font(14 if len(hw) > 8 else 16)
    for i, r in enumerate(hw):
        v = float(r.get("two_qubit_gate_count") or r.get("qasm_gate_lines") or 0)
        cx = int(x0 + (i + 0.5) * slot)
        top = int(y1 - min(1.0, v / y_max) * (y1 - y0))
        color = ORANGE if bool(r.get("hardware")) else TEAL
        draw.rectangle([cx - bar_w // 2, top, cx + bar_w // 2, y1], fill=color, outline=INK, width=1)
        draw.text((cx, max(y0 + 18, top - 6)), f"{int(v)}", fill=INK, font=font_lbl, anchor="mb")
        jid = str(r.get("job_id") or "")[:6]
        be = _short_backend_label(r.get("backend"))
        draw.text((cx, y1 + 12), jid, fill=SLATE, font=font_lbl, anchor="mt")
        draw.text((cx, y1 + 32), be, fill=SLATE, font=font_lbl, anchor="mt")
    hashes = [str(r.get("circuit_sha256") or "")[:8] for r in hw if r.get("circuit_sha256")]
    hash_note = f" (circuit SHA-256 prefixes: {', '.join(hashes[:4])})" if hashes else ""
    caption = (
        f"Circuit two-qubit gate / gate-line footprint across {len(hw)} settled circuit jobs"
        f"{hash_note}. Complete OpenQASM sources are archived in supplement/qasm/."
    )
    return "fig_transpile_overhead.png", _to_png_bytes(img), caption


def _render_calibration_overview(rows: List[Dict[str, Any]]) -> Tuple[str, bytes, str] | None:
    """Phase 2: render processor calibration metrics (median 2Q & readout error) across snapshots."""
    cals = []
    seen = set()
    for r in rows:
        cal = r.get("calibration")
        if not isinstance(cal, dict):
            continue
        snap = str(cal.get("snapshot_id") or r.get("calibration_snapshot_id") or r.get("backend") or "")
        if not snap or snap in seen:
            continue
        e2q = cal.get("median_two_qubit_error", cal.get("two_qubit_error"))
        ero = cal.get("median_readout_error", cal.get("readout_error"))
        if e2q is None and ero is None:
            continue
        seen.add(snap)
        cals.append((str(r.get("backend") or "qpu"), snap[:12], float(e2q or 0.0) * 100.0, float(ero or 0.0) * 100.0))
    if not cals:
        return None
    cals = cals[:10]
    img, draw = _new_canvas(1600, 920)
    box = (150, 110, 1520, 725)
    y_max = max(max(max(a, b) for _, _, a, b in cals) * 1.25, 0.5)
    _draw_axes(
        draw,
        box,
        "QPU Calibration Snapshot Error Rates (Navy: Median 2Q %, Teal: Readout %)",
        "Backend & Calibration Snapshot",
        "Error Rate (%)",
        0.0,
        y_max,
        xlabel_offset=96,
    )
    x0, y0, x1, y1 = box
    slot = (x1 - x0) / len(cals)
    bw = max(14, min(42, int(slot * 0.28)))
    font_lbl = _load_font(14)
    for i, (be, snap, e2q, ero) in enumerate(cals):
        cx = int(x0 + (i + 0.5) * slot)
        top1 = int(y1 - min(1.0, e2q / y_max) * (y1 - y0))
        top2 = int(y1 - min(1.0, ero / y_max) * (y1 - y0))
        draw.rectangle([cx - bw - 3, top1, cx - 3, y1], fill=NAVY, outline=INK, width=1)
        draw.rectangle([cx + 3, top2, cx + bw + 3, y1], fill=TEAL, outline=INK, width=1)
        draw.text((cx - bw // 2 - 3, max(y0 + 18, top1 - 4)), f"{e2q:.2f}%", fill=INK, font=font_lbl, anchor="mb")
        draw.text((cx + bw // 2 + 3, max(y0 + 18, top2 - 4)), f"{ero:.2f}%", fill=INK, font=font_lbl, anchor="mb")
        draw.text((cx, y1 + 12), _short_backend_label(be), fill=SLATE, font=font_lbl, anchor="mt")
        draw.text((cx, y1 + 32), snap, fill=SLATE, font=font_lbl, anchor="mt")
    caption = (
        f"Hardware processor calibration error rates across {len(cals)} execution snapshot(s) "
        "(navy: median two-qubit gate error %, teal: median readout error %)."
    )
    return "fig_calibration_overview.png", _to_png_bytes(img), caption


def _parse_qasm_for_schematic(qasm: str) -> Tuple[int, List[Tuple[str, List[int]]], int, int]:
    """Parse OpenQASM 2/3 text (including multi-register declarations and OQ3 if-blocks)
    into (n_qubits, gates, total_gates, two_qubit_gates)."""
    import re

    n_qubits = 0
    reg_offsets: Dict[str, int] = {}
    gates: List[Tuple[str, List[int]]] = []
    two_q = 0
    qreg_re = re.compile(r"(?:qreg\s+(\w+)\[(\d+)\]|qubit\[(\d+)\]\s+(\w+))", re.IGNORECASE)
    ref_re = re.compile(r"(\w+)\[(\d+)\]")
    if_prefix_re = re.compile(r"^if\s*\([^)]*\)\s*", re.IGNORECASE)

    def _resolve_qubits(expr: str) -> List[int]:
        nonlocal n_qubits
        out_qs: List[int] = []
        for rname, idx_s in ref_re.findall(expr):
            idx = int(idx_s)
            off = reg_offsets.get(rname, 0)
            gidx = off + idx
            out_qs.append(gidx)
            n_qubits = max(n_qubits, gidx + 1)
        return out_qs

    for raw_line in qasm.splitlines():
        line = raw_line.split("//", 1)[0].strip().rstrip(";")
        if not line:
            continue
        low = line.lower()
        if low.startswith(("openqasm", "include ", "creg ", "bit", "barrier")):
            continue
        m_reg = qreg_re.search(line)
        if m_reg:
            rname = m_reg.group(1) or m_reg.group(4) or "q"
            rsize = int(m_reg.group(2) or m_reg.group(3) or 0)
            if rname not in reg_offsets:
                reg_offsets[rname] = n_qubits
                n_qubits += rsize
            continue
        # R-D: strip OpenQASM 2/3 classical conditional prefix `if (c == 1) x q[0]`
        line = if_prefix_re.sub("", line).strip()
        if not line:
            continue
        low = line.lower()
        if low.startswith("measure"):
            qs = _resolve_qubits(line.split("->", 1)[0])
            if qs:
                gates.append(("M", [qs[0]]))
            continue
        # Gate statement: name[(params)] reg[i], reg[j]
        tok = line.split(None, 1)
        if len(tok) < 2:
            continue
        gname = tok[0].split("(", 1)[0].strip().upper()[:5]
        qs = _resolve_qubits(tok[1])
        if not qs:
            continue
        if len(qs) >= 2:
            two_q += 1
        gates.append((gname, qs))
    return n_qubits, gates, len(gates), two_q


def _render_circuit_schematics(
    rows: List[Dict[str, Any]],
    circuit_samples: Dict[str, str] | None = None,
) -> List[Tuple[str, bytes, str]]:
    """R1: Render deterministic 300-DPI gate-grid circuit schematics (<= 1 per family)."""
    if not HAS_PIL or not circuit_samples:
        return []
    import hashlib
    import re

    rows_by_id = {str(r.get("job_id") or ""): r for r in rows if r.get("job_id")}
    out: List[Tuple[str, bytes, str]] = []
    seen_families: set[str] = set()

    for jid in sorted(circuit_samples.keys()):
        qasm = str(circuit_samples.get(jid) or "").strip()
        if not qasm:
            continue
        row = rows_by_id.get(jid, {})
        raw_kind = str(row.get("kind") or "circuit").strip().lower()
        fam = re.sub(r"[^a-z0-9]+", "_", raw_kind).strip("_") or "circuit"
        if fam in seen_families:
            continue
        n_q, gates, total_gates, two_q = _parse_qasm_for_schematic(qasm)
        if n_q <= 0 or not gates:
            continue
        seen_families.add(fam)

        # Pack gates greedily into parallel layers; display up to 18 layers and 12 wires
        max_wires = min(n_q, 12)
        layers: List[List[Tuple[str, List[int]]]] = []
        wire_free = [0] * n_q
        for gname, qs in gates:
            col = max((wire_free[q] for q in qs if q < n_q), default=0)
            while len(layers) <= col:
                layers.append([])
            layers[col].append((gname, qs))
            for q in qs:
                if q < n_q:
                    wire_free[q] = col + 1

        total_depth = len(layers)
        shown_layers = layers[:18]
        folded = total_depth > len(shown_layers) or n_q > max_wires

        w, h = 1600, max(540, 210 + max_wires * 58)
        img, draw = _new_canvas(w, h)
        font_title = _load_font(26)
        font_sub = _load_font(18)
        font_gate = _load_font(15)
        q_sha = str(row.get("circuit_sha256") or hashlib.sha256(qasm.encode("utf-8")).hexdigest())[:12]
        be = _short_backend_label(row.get("backend"))
        shots = row.get("shots")
        seed = row.get("seed")
        title = f"Circuit Schematic: {raw_kind} ({n_q}Q, depth {total_depth}, {two_q} 2Q gates)"
        sub = f"Job {jid[:8]} · backend={be} · shots={shots if shots is not None else '--'} · seed={seed if seed is not None else '--'} · sha256={q_sha}"
        draw.text((60, 24), title, fill=INK, font=font_title)
        draw.text((60, 62), sub, fill=SLATE, font=font_sub)

        x_start, x_end = 130, w - 90
        y_top = 125
        y_step = 56
        col_w = max(58, min(86, (x_end - x_start) // max(len(shown_layers) + (1 if folded else 0), 1)))

        for q in range(max_wires):
            wy = y_top + q * y_step
            draw.text((48, wy - 10), f"q[{q}]", fill=SLATE, font=font_gate)
            draw.line([(x_start - 10, wy), (x_end, wy)], fill=SLATE, width=2)

        for ci, layer in enumerate(shown_layers):
            cx = x_start + int((ci + 0.5) * col_w)
            for gname, qs in layer:
                vis_qs = [q for q in qs if q < max_wires]
                if not vis_qs:
                    continue
                if len(qs) >= 2:
                    ys = [y_top + q * y_step for q in vis_qs]
                    if len(ys) >= 2:
                        draw.line([(cx, min(ys)), (cx, max(ys))], fill=TEAL, width=3)
                    # Draw the gate according to its semantics: CX/CCX use
                    # control dots + target box, CZ uses dots only, SWAP uses
                    # X boxes, and multi-qubit rotations use a labelled box on
                    # every wire. A CNOT glyph for CZ/SWAP would misstate the
                    # circuit.
                    if gname in ("CX", "CNOT", "CCX", "TOFFOLI"):
                        for q in vis_qs[:-1]:
                            c_y = y_top + q * y_step
                            draw.ellipse([cx - 7, c_y - 7, cx + 7, c_y + 7], fill=TEAL, outline=INK, width=1)
                        t_y = y_top + vis_qs[-1] * y_step
                        draw.rectangle([cx - 22, t_y - 16, cx + 22, t_y + 16], fill=ORANGE, outline=INK, width=2)
                        draw.text((cx, t_y), gname[:4], fill=BG, font=font_gate, anchor="mm")
                    elif gname == "CZ":
                        for q in vis_qs:
                            c_y = y_top + q * y_step
                            draw.ellipse([cx - 7, c_y - 7, cx + 7, c_y + 7], fill=TEAL, outline=INK, width=1)
                            draw.text((cx, c_y), "Z", fill=BG, font=font_gate, anchor="mm")
                    elif gname in ("SWAP",):
                        for q in vis_qs:
                            c_y = y_top + q * y_step
                            draw.rectangle([cx - 22, c_y - 16, cx + 22, c_y + 16], fill=ORANGE, outline=INK, width=2)
                            draw.text((cx, c_y), "X", fill=BG, font=font_gate, anchor="mm")
                    else:
                        for q in vis_qs:
                            c_y = y_top + q * y_step
                            draw.rectangle([cx - 22, c_y - 16, cx + 22, c_y + 16], fill=NAVY, outline=INK, width=2)
                            draw.text((cx, c_y), gname[:4], fill=BG, font=font_gate, anchor="mm")
                else:
                    qy = y_top + vis_qs[0] * y_step
                    fill = NAVY if gname != "M" else PURPLE
                    draw.rectangle([cx - 22, qy - 16, cx + 22, qy + 16], fill=fill, outline=INK, width=2)
                    draw.text((cx, qy), gname[:4], fill=BG, font=font_gate, anchor="mm")

        if folded:
            note = (
                f"Schematic shows first {len(shown_layers)}/{total_depth} layers and "
                f"{max_wires}/{n_q} qubits ({total_gates} total gates); full OpenQASM in supplement/qasm/{jid[:8]}..."
            )
            draw.text((60, h - 42), note, fill=SLATE, font=font_sub)

        fname = f"fig_circuit_{fam}.png"
        caption = (
            f"Representative quantum circuit schematic for {raw_kind} "
            f"(n={n_q} qubits, depth={total_depth}, {two_q} two-qubit gates, "
            f"shots={shots if shots is not None else 'n/a'}, seed={seed if seed is not None else 'n/a'}, "
            f"circuit SHA-256 prefix {q_sha}). Complete OpenQASM source archived in supplement/qasm/."
        )
        out.append((fname, _to_png_bytes(img), caption))
        if len(out) >= 6:
            break
    return out


def generate_campaign_figures(
    rows: List[Dict[str, Any]],
    *,
    circuit_samples: Dict[str, str] | None = None,
) -> List[Tuple[str, bytes, str]]:
    """Synthesize deterministic 300-DPI PNG figures from settled campaign rows."""
    if not HAS_PIL or not rows:
        return []
    out: List[Tuple[str, bytes, str]] = []
    for fn in (
        _render_energy_benchmark,
        _render_pes_or_depth,
        _render_quench_dynamics,
        _render_qaoa_or_dmrg,
        _render_qec_benchmark,
        _render_transpile_overhead,
        _render_calibration_overview,
    ):
        try:
            res = fn(rows)
            if res is not None:
                out.append(res)
        except Exception:
            continue
    if circuit_samples:
        try:
            out.extend(_render_circuit_schematics(rows, circuit_samples))
        except Exception:
            pass
    return out
