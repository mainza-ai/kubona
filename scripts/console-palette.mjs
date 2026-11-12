#!/usr/bin/env node
// Kubona console palette: every literal colour, alpha, shadow, radius and
// type size in the console's design tokens is DERIVED here from a small set
// of Kubona-owned parameters (OKLCH seeds, an alpha ladder, a radius rule,
// a modular type scale). Nothing is hand-picked hex.
//
// The derivation is also a gate: verifyPalette() checks WCAG contrast for
// every text/control pairing, colour-vision-deficiency separation for the
// chart series, and ladder ordering. A parameter change that breaks any
// check fails CI instead of shipping.
//
//   node scripts/console-palette.mjs --write   regenerate outputs
//   node scripts/console-palette.mjs --check   fail if outputs drift or a check fails
//
// Outputs (generated — never hand-edit):
//   platform/console/src/styles/palette.css
//   platform/console/src/styles/palette.generated.ts
//
// Colour science references (published methods, implemented from the papers):
//   OKLab/OKLCH — B. Ottosson, "A perceptual color space for image processing" (2020).
//   Relative luminance & contrast ratio — WCAG 2.2 §1.4.3 / §1.4.11.
//   CVD simulation — Machado, Oliveira & Fernandes, IEEE TVCG 15(6), 2009 (severity 1.0).
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const STYLES = path.join(ROOT, 'platform', 'console', 'src', 'styles');
export const PALETTE_CSS_PATH = path.join(STYLES, 'palette.css');
export const PALETTE_TS_PATH = path.join(STYLES, 'palette.generated.ts');

// ---------------------------------------------------------------------------
// Parameters — the only place design decisions live.
// ---------------------------------------------------------------------------

/** Neutral hue: a faint indigo "observatory ink", one hue for both themes. */
export const NEUTRAL_HUE = 284;

/** Tonal ladders as OKLCH lightness; chroma is the neutral chroma per theme. */
export const LADDER = {
  dark: {
    chroma: 0.011,
    canvas: 0.205, chrome: 0.226, surface: 0.252, raised: 0.274, elev: 0.298, float: 0.334,
    text: 0.968, label: 0.885,
    lit: 0.94, litHover: 0.975, onLit: 0.19,
    trackOn: 0.66, ink: 1, // overlays are white in dark
  },
  light: {
    chroma: 0.0065,
    canvas: 0.958, chrome: 0.918, surface: 1, raised: 1, elev: 0.968, float: 1,
    text: 0.215, label: 0.45,
    lit: 0.25, litHover: 0.315, onLit: 1,
    trackOn: 0.5, track: 0.885, ink: 0.205, // overlays are neutral ink in light
  },
};

/** Text alpha over the ladder (muted / faint). Verified >= 4.5:1 below. */
export const TEXT_ALPHA = { dark: { muted: 0.74, faint: 0.64 }, light: { muted: 0.74, faint: 0.67 } };

/**
 * One geometric alpha ladder for every overlay fill, hairline and shadow:
 * A[n] = 0.044 * 1.42^(n-1). Tokens pick a rung, never a free number.
 */
export const ALPHA = Array.from({ length: 10 }, (_, n) => Math.round(0.044 * 1.42 ** (n - 1) * 1000) / 1000);

/** Rung assignment for overlay tokens (same rung in both themes; ink differs). */
export const OVERLAY_RUNGS = {
  '--color-fill': 1, '--color-fill-hover': 2, '--color-fill-active': 3, '--color-fill-pressed': 4, '--color-fill-strong': 5,
  '--color-card': 1, '--color-tile': 2, '--color-tile-hover': 3,
  '--color-border': 2, '--color-border-strong': 4,
  '--row-hover': 0, '--row-active': 2, '--row-pressed': 3,
  '--scroll-thumb': 4, '--scroll-thumb-hover': 6,
  '--color-control': 3, '--color-control-hover': 4, '--color-control-pressed': 5,
  '--color-field': 3, '--color-field-hover': 4, '--color-field-ring': 4,
  '--color-raise': 2, '--color-raise-active': 3,
  '--color-menu-separator': 3,
};
/** Dark-only overlay rungs (light uses solid ladder values for these). */
export const DARK_ONLY_RUNGS = { '--color-track': 4 };
/** Light-theme controls are solid paper, not translucent ink. */
export const LIGHT_SOLID = { '--color-control': 1, '--color-control-hover': 0.984, '--color-control-pressed': 0.94 };

/** Focus ring alpha rung (verified >= 3:1 non-text contrast). */
export const RING_RUNG = { dark: 8, light: 9 };

/** Accent tints (OKLCH hue). Graphite is the neutral lit accent. */
export const TINT_HUES = { sky: 252, sage: 156, rose: 18, amber: 72, teal: 192, lavender: 292, orchid: 332 };
export const TINT = {
  dark: { l: 0.79, c: 0.105, hoverL: 0.86, hoverC: 0.085, onL: 0.22, onC: 0.045 },
  light: { l: 0.505, c: 0.135, hoverL: 0.43, hoverC: 0.12 },
  trackOn: { l: 0.6, c: 0.15 },
};

/** Semantic states (OKLCH hue). */
export const SEMANTIC_HUES = { danger: 22, success: 158, warning: 78, info: 250 };
export const SEMANTIC = { dark: { l: 0.775, c: 0.105 }, light: { l: 0.5, c: 0.13 } };

/** Soft hues for badges/tags/code chips. */
export const SOFT_HUES = { sky: 246, mint: 170, lemon: 94, peach: 52, rose: 4, lilac: 296 };
export const SOFT = {
  dark: { l: 0.805, c: 0.085, wash: 14, edge: 28, ink: 60 },
  light: { l: 0.6, c: 0.135, wash: 12, edge: 32, ink: 76 },
  washRaised: 60,
};

/**
 * Categorical chart series. Hues alternate warm/cool and lightness alternates
 * so adjacent series stay separable under protan/deutan/tritan simulation.
 */
export const CHART_HUES = [255, 48, 172, 92, 338, 132, 298, 212];
export const CHART = {
  dark: { lights: [0.7, 0.63], c: 0.125, otherL: 0.63 },
  light: { lights: [0.53, 0.6], c: 0.13, otherL: 0.54 },
};

/** Elevation: y = 2^e px, blur = 3y, alpha grows linearly with e. */
export const ELEVATION = {
  levels: { raised: 0, float: 3, overlay: 4 },
  alpha: { dark: (e) => 0.17 + 0.05 * e, light: (e) => 0.05 + 0.025 * e },
};

/** Radius rule: r = nearest even px of (height x 0.3), for the element's height. */
export const RADIUS_RATIO = 0.3;
export const RADIUS_HEIGHTS = {
  '--radius-inset': 20,          // glyph plate / keycap inside a control
  '--radius-control-compact': 28, // .k-button-compact
  '--radius-control': 32,         // .k-button-regular
  '--radius-row': 40,             // .k-list-row min block size
  '--radius-control-large': 46,
};

/** Modular type scale: s(k) = 13 * 1.2^(k/2) px (minor third, half steps). */
export const TYPE_BASE = 13;
export const TYPE_RATIO = 1.2;
export const TYPE_STEPS = {
  '--text-caption': -2, '--text-label': -1, '--text-body': 0, '--text-page-body': 0,
  '--text-page-title': 1, '--text-title': 6, '--text-headline': 8, '--text-display': 10,
};

// ---------------------------------------------------------------------------
// Colour math
// ---------------------------------------------------------------------------

const cbrt = Math.cbrt;

export function oklabToLinear([L, a, b]) {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

export function linearToOklab([r, g, b]) {
  const l = cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

const oklchToOklab = (L, C, h) => [L, C * Math.cos((h * Math.PI) / 180), C * Math.sin((h * Math.PI) / 180)];
const inGamut = (rgb) => rgb.every((v) => v >= -1e-6 && v <= 1 + 1e-6);

/** OKLCH -> linear sRGB, reducing chroma (hue and lightness kept) until in gamut. */
export function oklchToLinear(L, C, h) {
  let rgb = oklabToLinear(oklchToOklab(L, C, h));
  if (inGamut(rgb)) return rgb;
  let lo = 0;
  let hi = C;
  for (let i = 0; i < 32; i += 1) {
    const mid = (lo + hi) / 2;
    if (inGamut(oklabToLinear(oklchToOklab(L, mid, h)))) lo = mid;
    else hi = mid;
  }
  rgb = oklabToLinear(oklchToOklab(L, lo, h));
  return rgb.map((v) => Math.min(1, Math.max(0, v)));
}

const encode = (v) => (v <= 0.0031308 ? 12.92 * v : 1.055 * v ** (1 / 2.4) - 0.055);
const decode = (v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);

export function linearToHex(rgb) {
  return `#${rgb.map((v) => Math.round(Math.min(1, Math.max(0, encode(v))) * 255).toString(16).padStart(2, '0')).join('')}`;
}

export function hexToLinear(hex) {
  const n = hex.replace('#', '');
  return [0, 2, 4].map((i) => decode(parseInt(n.slice(i, i + 2), 16) / 255));
}

export const oklch = (L, C, h) => linearToHex(oklchToLinear(L, C, h));

export function hexToRgbTriplet(hex) {
  const n = hex.replace('#', '');
  return [0, 2, 4].map((i) => parseInt(n.slice(i, i + 2), 16));
}

export function luminance(hex) {
  const [r, g, b] = hexToLinear(hex);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a, b) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

/** Source-over composite of `fg` at `alpha` onto opaque `bg` (sRGB-encoded, as browsers blend). */
export function composite(fg, alpha, bg) {
  const f = hexToRgbTriplet(fg);
  const b = hexToRgbTriplet(bg);
  return `#${f.map((v, i) => Math.round(v * alpha + b[i] * (1 - alpha)).toString(16).padStart(2, '0')).join('')}`;
}

/** CSS `color-mix(in oklab, a p%, b)` for opaque colours. */
export function mixOklab(a, pct, b) {
  const A = linearToOklab(hexToLinear(a));
  const B = linearToOklab(hexToLinear(b));
  const t = pct / 100;
  return linearToHex(oklabToLinear(A.map((v, i) => v * t + B[i] * (1 - t))));
}

const MACHADO = {
  protan: [[0.152286, 1.052583, -0.204868], [0.114503, 0.786281, 0.099216], [-0.003882, -0.048116, 1.051998]],
  deutan: [[0.367322, 0.860646, -0.227968], [0.280085, 0.672501, 0.047413], [-0.01182, 0.04294, 0.968881]],
  tritan: [[1.255528, -0.076749, -0.178779], [-0.078411, 0.930809, 0.147602], [0.004733, 0.691367, 0.3039]],
};

export function simulateCvd(hex, kind) {
  const rgb = hexToLinear(hex);
  const M = MACHADO[kind];
  return linearToHex(M.map((row) => Math.min(1, Math.max(0, row[0] * rgb[0] + row[1] * rgb[1] + row[2] * rgb[2]))));
}

/** Euclidean distance in OKLab (perceptual difference; ~0.02 is a just-noticeable step). */
export function deltaOk(a, b) {
  const A = linearToOklab(hexToLinear(a));
  const B = linearToOklab(hexToLinear(b));
  return Math.hypot(A[0] - B[0], A[1] - B[1], A[2] - B[2]);
}

const rgba = (hex, alpha) => {
  const [r, g, b] = hexToRgbTriplet(hex);
  return `rgb(${r} ${g} ${b} / ${alpha})`;
};

const evenPx = (v) => 2 * Math.round(v / 2);

// ---------------------------------------------------------------------------
// Derivation
// ---------------------------------------------------------------------------

function deriveTheme(theme) {
  const P = LADDER[theme];
  const n = (L, C = P.chroma) => oklch(L, C, NEUTRAL_HUE);
  const ink = n(P.ink, theme === 'dark' ? 0 : P.chroma);
  const t = {};

  t['--color-canvas'] = n(P.canvas);
  t['--color-page'] = t['--color-canvas'];
  t['--color-chrome'] = n(P.chrome);
  t['--color-surface'] = n(P.surface, P.surface === 1 ? 0 : P.chroma);
  t['--color-surface-elev'] = n(P.elev);
  t['--color-raised'] = n(P.raised, P.raised === 1 ? 0 : P.chroma);
  t['--color-float'] = n(P.float, P.float === 1 ? 0 : P.chroma);

  for (const [token, rung] of Object.entries(OVERLAY_RUNGS)) t[token] = rgba(ink, ALPHA[rung]);
  if (theme === 'dark') {
    for (const [token, rung] of Object.entries(DARK_ONLY_RUNGS)) t[token] = rgba(ink, ALPHA[rung]);
  } else {
    for (const [token, L] of Object.entries(LIGHT_SOLID)) t[token] = n(L, L === 1 ? 0 : P.chroma);
    t['--color-track'] = n(P.track);
    t['--color-card'] = rgba(t['--color-surface'], 0.94);
  }
  t['--row-rim'] = `inset 0 0 0 1px ${rgba(ink, ALPHA[1])}`;

  t['--color-text'] = n(P.text);
  t['--color-label-secondary'] = n(P.label);
  t['--color-muted'] = rgba(t['--color-text'], TEXT_ALPHA[theme].muted);
  t['--color-faint'] = rgba(t['--color-text'], TEXT_ALPHA[theme].faint);
  t['--color-on-control'] = theme === 'dark' ? 'var(--color-text)' : t['--color-text'];
  t['--color-on-control-strong'] = t['--color-on-control'];
  t['--color-track-on'] = n(P.trackOn);

  t['--color-lit'] = n(P.lit);
  t['--color-lit-hover'] = n(P.litHover);
  t['--color-on-lit'] = n(P.onLit, P.onLit === 1 ? 0 : P.chroma);
  t['--color-lit-soft'] = rgba(t['--color-lit'], theme === 'dark' ? ALPHA[5] : ALPHA[4]);
  t['--color-ring'] = rgba(ink, ALPHA[RING_RUNG[theme]]);

  const tint = TINT[theme];
  t['--color-tint-graphite'] = n(theme === 'dark' ? LADDER.dark.trackOn : LADDER.light.trackOn + 0.04);
  for (const [name, h] of Object.entries(TINT_HUES)) t[`--color-tint-${name}`] = oklch(tint.l, tint.c, h);

  const sem = SEMANTIC[theme];
  for (const [name, h] of Object.entries(SEMANTIC_HUES)) t[`--color-${name}`] = oklch(sem.l, sem.c, h);

  const soft = SOFT[theme];
  for (const [name, h] of Object.entries(SOFT_HUES)) t[`--color-soft-${name}`] = oklch(soft.l, soft.c, h);
  t['--soft-wash'] = `${soft.wash}%`;
  t['--soft-edge'] = `${soft.edge}%`;
  t['--soft-ink'] = `${soft.ink}%`;

  const chart = CHART[theme];
  CHART_HUES.forEach((h, i) => {
    t[`--color-chart-${i + 1}`] = oklch(chart.lights[i % 2], chart.c, h);
  });
  t['--color-chart-other'] = n(chart.otherL, theme === 'dark' ? 0.012 : 0.01);

  const shadowInk = theme === 'dark' ? '#000000' : ink;
  const a = ELEVATION.alpha[theme];
  const drop = (e, spread = '') => {
    const y = 2 ** e;
    return `0 ${y}px ${3 * y}px${spread} ${rgba(shadowInk, Math.round(a(e) * 1000) / 1000)}`;
  };
  const hairline = (rung) => `0 0 0 0.5px ${rgba(ink, ALPHA[rung])}`;
  const L = ELEVATION.levels;
  t['--shadow-card'] = theme === 'dark' ? 'none' : `${hairline(2)}, ${drop(L.raised)}`;
  t['--shadow-raised'] = `${drop(L.raised)}, ${hairline(theme === 'dark' ? 1 : 2)}`;
  t['--shadow-float'] = `${drop(L.float)}, ${hairline(2)}`;
  t['--shadow-overlay'] = `${drop(L.overlay)}, ${hairline(2)}`;
  t['--shadow-menu'] = `0 0 0 1px ${rgba(ink, ALPHA[2])}, ${drop(L.overlay, ' -12px')}`;
  t['--shine'] = theme === 'dark'
    ? `inset 0 1px 0 0 ${rgba(ink, ALPHA[4])}, inset 0 -1px 0 0 ${rgba(ink, ALPHA[0])}, inset 0 0 0 1px ${rgba(ink, ALPHA[1])}`
    : `inset 0 1px 0 0 ${rgba('#ffffff', ALPHA[7])}`;
  if (theme === 'light') {
    t['--segment-thumb'] = rgba(t['--color-surface'], 0.9);
    t['--shadow-segment-thumb'] = `${hairline(2)}, ${drop(L.raised)}`;
  }
  return t;
}

function deriveTintOverrides() {
  const out = {};
  for (const [name, h] of Object.entries(TINT_HUES)) {
    out[name] = {
      dark: {
        '--color-accent': `var(--color-tint-${name})`,
        '--color-track-on': oklch(TINT.trackOn.l, TINT.trackOn.c, h),
        '--color-accent-hover': oklch(TINT.dark.hoverL, TINT.dark.hoverC, h),
        '--color-on-accent': oklch(TINT.dark.onL, TINT.dark.onC, h),
      },
      light: {
        '--color-accent': `var(--color-tint-${name})`,
        '--color-accent-hover': oklch(TINT.light.hoverL, TINT.light.hoverC, h),
        '--color-on-accent': '#ffffff',
      },
    };
  }
  return out;
}

function deriveScale() {
  const radii = { '--radius-swatch': '2px' };
  for (const [token, h] of Object.entries(RADIUS_HEIGHTS)) radii[token] = `${evenPx(h * RADIUS_RATIO)}px`;
  const large = parseInt(radii['--radius-control-large'], 10);
  radii['--radius-card'] = `${large + 2}px`;
  radii['--radius-panel'] = `${large + 6}px`;
  radii['--radius-capsule'] = '999px';
  const type = {};
  for (const [token, k] of Object.entries(TYPE_STEPS)) type[token] = `${Math.round(TYPE_BASE * TYPE_RATIO ** (k / 2))}px`;
  return { ...radii, ...type, '--wash-raised': `${SOFT.washRaised}%` };
}

export function derivePalette() {
  return { dark: deriveTheme('dark'), light: deriveTheme('light'), tints: deriveTintOverrides(), scale: deriveScale() };
}

// ---------------------------------------------------------------------------
// Verification — every pairing the console relies on.
// ---------------------------------------------------------------------------

const alphaOf = (v) => {
  const m = /\/ ([\d.]+)\)$/.exec(v);
  return m ? Number(m[1]) : 1;
};
const baseOf = (v) => {
  const m = /^rgb\((\d+) (\d+) (\d+)/.exec(v);
  return m ? `#${m.slice(1, 4).map((x) => Number(x).toString(16).padStart(2, '0')).join('')}` : v;
};
/** Resolve a token value to an opaque hex over `bg`. */
const over = (v, bg) => (v.startsWith('#') ? v : composite(baseOf(v), alphaOf(v), bg));

export function verifyPalette(p = derivePalette()) {
  const failures = [];
  const need = (ok, msg) => { if (!ok) failures.push(msg); };
  const ratio = (fg, bg, min, label) => {
    const r = contrast(fg, bg);
    need(r >= min, `${label}: ${r.toFixed(2)}:1 < ${min}:1 (${fg} on ${bg})`);
  };

  for (const theme of ['dark', 'light']) {
    const t = p[theme];
    const bases = ['--color-canvas', '--color-chrome', '--color-surface', '--color-raised'].map((k) => [k, t[k]]);

    // Ladder order: dark climbs in lightness toward the viewer, light descends from paper.
    const Lof = (hex) => linearToOklab(hexToLinear(hex))[0];
    const order = ['--color-canvas', '--color-chrome', '--color-surface', '--color-raised', '--color-surface-elev', '--color-float'];
    if (theme === 'dark') {
      for (let i = 1; i < order.length; i += 1) need(Lof(t[order[i]]) > Lof(t[order[i - 1]]), `dark ladder not ascending at ${order[i]}`);
    } else {
      need(Lof(t['--color-chrome']) < Lof(t['--color-canvas']) && Lof(t['--color-canvas']) < Lof(t['--color-surface']), 'light ladder order chrome < canvas < surface broken');
    }

    for (const [bk, bg] of bases) {
      ratio(t['--color-text'], bg, 7, `${theme} text on ${bk}`);
      ratio(t['--color-label-secondary'], bg, 4.5, `${theme} label-secondary on ${bk}`);
      ratio(over(t['--color-muted'], bg), bg, 4.5, `${theme} muted on ${bk}`);
      ratio(over(t['--color-faint'], bg), bg, 4.5, `${theme} faint on ${bk}`);
      ratio(over(t['--color-ring'], bg), bg, 3, `${theme} focus ring on ${bk}`);
    }
    const surface = t['--color-surface'];
    const raised = t['--color-raised'];
    ratio(t['--color-on-lit'], t['--color-lit'], 4.5, `${theme} on-lit on lit`);
    ratio(t['--color-on-lit'], t['--color-lit-hover'], 4.5, `${theme} on-lit on lit-hover`);
    ratio(t['--color-lit'], surface, 3, `${theme} lit accent vs surface`);
    ratio(t['--color-track-on'], surface, 3, `${theme} track-on vs surface`);
    for (const name of Object.keys(SEMANTIC_HUES)) {
      ratio(t[`--color-${name}`], surface, 4.5, `${theme} ${name} text on surface`);
      ratio(t[`--color-${name}`], raised, 4.5, `${theme} ${name} text on raised`);
    }
    const soft = SOFT[theme];
    const softPairs = [
      ...Object.keys(SOFT_HUES).map((name) => t[`--color-soft-${name}`]),
      ...Object.keys(SEMANTIC_HUES).map((name) => t[`--color-${name}`]),
    ];
    for (const hue of softPairs) {
      const wash = mixOklab(hue, soft.wash, surface);
      const inkHex = mixOklab(hue, soft.ink, t['--color-text']);
      ratio(inkHex, wash, 4.5, `${theme} soft ink on wash (${hue})`);
    }
    for (const name of Object.keys(TINT_HUES)) ratio(t[`--color-tint-${name}`], surface, 3, `${theme} tint ${name} vs surface`);

    // Chart: every series >= 3:1 against the plot surfaces; all pairs distinct
    // to normal vision; adjacent pairs distinct under each CVD simulation.
    const series = CHART_HUES.map((_, i) => t[`--color-chart-${i + 1}`]);
    for (const [bk, bg] of [['surface', surface], ['raised', raised]]) {
      series.forEach((c, i) => ratio(c, bg, 3, `${theme} chart-${i + 1} vs ${bk}`));
      ratio(t['--color-chart-other'], bg, 3, `${theme} chart-other vs ${bk}`);
    }
    for (let i = 0; i < series.length; i += 1) {
      for (let j = i + 1; j < series.length; j += 1) {
        const d = deltaOk(series[i], series[j]);
        need(d >= 0.08, `${theme} chart-${i + 1}/chart-${j + 1} too similar (ΔOK ${d.toFixed(3)} < 0.08)`);
      }
    }
    for (const kind of Object.keys(MACHADO)) {
      for (let i = 0; i + 1 < series.length; i += 1) {
        const d = deltaOk(simulateCvd(series[i], kind), simulateCvd(series[i + 1], kind));
        need(d >= 0.05, `${theme} chart-${i + 1}/chart-${i + 2} not separable under ${kind} (ΔOK ${d.toFixed(3)} < 0.05)`);
      }
    }
  }

  for (const [name, o] of Object.entries(p.tints)) {
    const darkAccent = p.dark[`--color-tint-${name}`];
    const lightAccent = p.light[`--color-tint-${name}`];
    ratio(o.dark['--color-on-accent'], darkAccent, 4.5, `dark ${name} on-accent on accent`);
    ratio(o.dark['--color-on-accent'], o.dark['--color-accent-hover'], 4.5, `dark ${name} on-accent on hover`);
    ratio(o.light['--color-on-accent'], lightAccent, 4.5, `light ${name} on-accent on accent`);
    ratio(o.light['--color-on-accent'], o.light['--color-accent-hover'], 4.5, `light ${name} on-accent on hover`);
    ratio(o.dark['--color-track-on'], p.dark['--color-surface'], 3, `dark ${name} track-on vs surface`);
    ratio(o.dark['--color-track-on'], p.light['--color-surface'], 3, `light ${name} track-on vs surface`);
  }
  return failures;
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

const HEADER = `GENERATED by scripts/console-palette.mjs — do not edit.
   Every value is derived from Kubona-owned parameters in that script and
   verified for WCAG contrast and CVD chart separation. Regenerate with:
   node scripts/console-palette.mjs --write`;

const block = (selector, tokens) =>
  `${selector} {\n${Object.entries(tokens).map(([k, v]) => `  ${k}: ${v};`).join('\n')}\n}\n`;

export function renderPaletteCss(p = derivePalette()) {
  const parts = [`/* ${HEADER} */\n`, block(':root', { ...p.scale, ...p.dark }), block(':root[data-theme="light"]', p.light)];
  for (const [name, o] of Object.entries(p.tints)) parts.push(block(`:root[data-tint="${name}"]`, o.dark));
  for (const [name, o] of Object.entries(p.tints)) parts.push(block(`:root[data-tint="${name}"][data-theme="light"]`, o.light));
  return parts.join('\n');
}

export function renderPaletteTs(p = derivePalette()) {
  const pick = (t) => Object.fromEntries(Object.entries(t).filter(([, v]) => typeof v === 'string' && (v.startsWith('#') || v.startsWith('rgb('))));
  const data = { dark: pick(p.dark), light: pick(p.light) };
  return `// ${HEADER.replace(/\n {3}/g, '\n// ')}\n\n/** Resolved literal palette, for canvas/minimap fallbacks and tests. */\nexport const PALETTE = ${JSON.stringify(data, null, 2)} as const;\n`;
}

function main() {
  const mode = process.argv[2];
  const failures = verifyPalette();
  if (failures.length) {
    console.error(`palette verification failed (${failures.length}):\n  ${failures.join('\n  ')}`);
    process.exit(1);
  }
  const css = renderPaletteCss();
  const ts = renderPaletteTs();
  if (mode === '--write') {
    fs.writeFileSync(PALETTE_CSS_PATH, css);
    fs.writeFileSync(PALETTE_TS_PATH, ts);
    console.log(`palette: verified and wrote ${path.relative(ROOT, PALETTE_CSS_PATH)} + ${path.relative(ROOT, PALETTE_TS_PATH)}`);
    return;
  }
  if (mode === '--check') {
    const drift = [
      [PALETTE_CSS_PATH, css],
      [PALETTE_TS_PATH, ts],
    ].filter(([file, want]) => !fs.existsSync(file) || fs.readFileSync(file, 'utf8') !== want);
    if (drift.length) {
      console.error(`palette outputs drifted from the derivation: ${drift.map(([f]) => path.relative(ROOT, f)).join(', ')}\nrun: node scripts/console-palette.mjs --write`);
      process.exit(1);
    }
    console.log('palette: verified, outputs in sync');
    return;
  }
  console.error('usage: console-palette.mjs --write | --check');
  process.exit(2);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
