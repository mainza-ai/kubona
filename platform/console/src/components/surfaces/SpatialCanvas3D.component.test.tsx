import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import {
  SpatialCanvas3D,
  WEBGL_UNAVAILABLE_MESSAGE,
  roundHoverReading,
  hoverReadingChanged,
  initialPixelRatioCap,
  touchProbeHint,
} from './SpatialCanvas3D';
import type { CampaignState } from '@/types';

// Migrated from react-test-renderer (SpatialCanvas3D.test.tsx). The old suite
// asserted against a textOf(tree.toJSON()) walk, which cannot tell chrome the
// operator sees from props that were merely serialized. These assertions read
// the rendered container in a real Chromium page (WebGL is available there, so
// the live HUD mounts; the anchorless branch is still checked explicitly).
type Call = { method: string; path: string };

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

// No @/lib/api mock: the ledger/vault reads the observatory performs on mount
// are answered at the fetch boundary, so the real wrapper + contract path runs.
function installLedgerFetch() {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ method, path });
    if (method === 'GET' && path === '/api/vault/predictions/latest') {
      return ok({ ok: true, sealed: null, evaluation: null });
    }
    if (method === 'GET' && path === '/api/vault/predictions/calibration') {
      return ok({ ok: true, authors: {}, evaluations: 0 });
    }
    if (method === 'GET' && path === '/api/vault/predictions/history') {
      return ok({ ok: true, count: 0, predictions: [] });
    }
    if (method === 'POST' && path === '/api/ledger/export') {
      return ok({ ok: true, events: [] });
    }
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${method} ${path}` }),
      text: async () => `unexpected ${method} ${path}`,
    };
  });
  return calls;
}

const campaign = {
  session_id: 'sess-cost-test',
  target_system: 'H2',
  classical_anchor: { anchor_computed: true, fci_exact_energy: -1.137 },
  epistemics: { p_correlated: 0.92 },
} as unknown as CampaignState;

const anchorless = {
  session_id: 'sess-anchorless',
  target_system: 'H2',
  // p_correlated present but no computed anchor: must never look computed.
  epistemics: { p_correlated: 0.92 },
} as unknown as CampaignState;

afterEach(() => {
  vi.unstubAllGlobals();
});

// The live HUD assertions read chrome that only mounts once the GPU renderer
// starts; without a context the component correctly renders the fail-closed
// panel instead. Headless Chromium ships SwiftShader, so this precondition
// holds here — a runner with WebGL disabled fails with the reason instead of a
// confusing text mismatch.
function assertWebgl() {
  const gl = document.createElement('canvas').getContext('webgl2');
  expect(gl, 'no WebGL2 context: the observatory HUD cannot mount in this browser').not.toBeNull();
}

describe('observatory cost honesty (F-14) in a real browser', () => {
  beforeAll(assertWebgl);

  it('idle HUD shows no price — only the on-demand descriptor', async () => {
    installLedgerFetch();
    const screen = await render(<SpatialCanvas3D campaign={campaign} />);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('On-demand generative lens');
    expect(text).not.toContain('0.50');
    expect(text).not.toContain('500 cr');
    expect(text).not.toContain('Reserved $');
  });

  it('result card shows the actual reserved amount from dispatch', async () => {
    installLedgerFetch();
    const screen = await render(
      <SpatialCanvas3D campaign={campaign} lensImageUrl="https://cdn.example/f.png" lensCostUsd={0.5} />,
    );
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Reserved $0.50 against session cap');
  });

  it('result card shows no reserved line when the dispatch reported none', async () => {
    installLedgerFetch();
    const screen = await render(
      <SpatialCanvas3D campaign={campaign} lensImageUrl="https://cdn.example/f.png" />,
    );
    const text = screen.container.textContent ?? '';
    // The synthetic-lens watermark still renders; the money line must not.
    expect(text).toContain('[SYNTHETIC LENS — WORLD LABS ATLAS]');
    expect(text).not.toContain('Reserved $');
  });
});

describe('observatory anchor honesty (H1) in a real browser', () => {
  beforeAll(assertWebgl);

  it('anchorless campaign hides the 3D terrain instead of inventing a reference', async () => {
    installLedgerFetch();
    const screen = await render(<SpatialCanvas3D campaign={anchorless} />);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Observatory requires a computed classical anchor.');
    expect(text).not.toContain('UNVERIFIED terrain — illustrative');
    expect(text).not.toContain('ρ = 0.92');
  });

  it('no sealed prediction means no synthetic ghost plane', async () => {
    const calls = installLedgerFetch();
    const screen = await render(<SpatialCanvas3D campaign={campaign} />);
    // The real ledger read happens on mount; an empty answer leaves the ghost
    // unbound rather than fabricating one.
    expect(calls.some((c) => c.method === 'GET' && c.path === '/api/vault/predictions/latest')).toBe(true);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('No sealed prediction');
    expect(text).not.toContain('Vault Ghost (illustrative)');
    expect(text).not.toContain('ghost is an illustrative placeholder');
  });

  it('anchored campaign shows the computed rho and no UNVERIFIED probe tag', async () => {
    installLedgerFetch();
    const screen = await render(<SpatialCanvas3D campaign={campaign} />);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('ρ = 0.92');
    expect(text).not.toContain('UNVERIFIED terrain — illustrative');
  });
});

describe('observatory Wave-3 contracts (pure helpers, no DOM/GL)', () => {
  it('fail-closed message names the 2D fallback', () => {
    expect(WEBGL_UNAVAILABLE_MESSAGE).toBe('3D unavailable — 2D workbench below');
  });

  it('roundHoverReading quantizes to display precision', () => {
    // 4dp on R, 5dp on energy — the exact precision the HUD renders.
    expect(roundHoverReading(0.74146, -1.137001)).toEqual({ r: 0.7415, energy: -1.137 });
    expect(roundHoverReading(1.59567, -7.8820123)).toEqual({ r: 1.5957, energy: -7.88201 });
  });

  it('hoverReadingChanged fires only on real rounded-chip change', () => {
    expect(hoverReadingChanged(null, { r: 1, energy: 2 })).toBe(true);
    // Sub-display-precision jitter collapses after rounding.
    const a = roundHoverReading(0.74141, -1.137);
    const b = roundHoverReading(0.74142, -1.137);
    expect(hoverReadingChanged(a, b)).toBe(false);
    expect(hoverReadingChanged(a, roundHoverReading(0.75, -1.137))).toBe(true);
    expect(hoverReadingChanged(a, roundHoverReading(0.7414, -1.2))).toBe(true);
  });

  it('initialPixelRatioCap honors the coarse-pointer 1.5 cap', () => {
    expect(initialPixelRatioCap(3, true)).toBe(1.5);
    expect(initialPixelRatioCap(1.25, true)).toBe(1.25);
    expect(initialPixelRatioCap(3, false)).toBe(2);
    expect(initialPixelRatioCap(1, false)).toBe(1);
    expect(initialPixelRatioCap(NaN, false)).toBe(1);
  });

  it('touchProbeHint swaps to tap/drag copy on coarse pointers', () => {
    expect(touchProbeHint(true)).toContain('Tap to pin');
    expect(touchProbeHint(true)).toContain('drag to orbit');
    expect(touchProbeHint(false)).toContain('Click terrain to pin');
  });
});
