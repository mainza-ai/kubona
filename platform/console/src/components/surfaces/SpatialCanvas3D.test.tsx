import { beforeEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import {
  SpatialCanvas3D,
  WEBGL_UNAVAILABLE_MESSAGE,
  roundHoverReading,
  hoverReadingChanged,
  initialPixelRatioCap,
  touchProbeHint,
} from './SpatialCanvas3D';
import type { CampaignState } from '@/types';

// F-14: no hardcoded price may appear in idle chrome. Cost figures surface
// only from live dispatch responses (lensCostUsd) or the reservation ledger.
vi.mock('@/lib/api', () => ({
  fetchLatestPrediction: vi.fn().mockImplementation(async () => ({ ok: true, sealed: null, evaluation: null })),
  sealPrediction: vi.fn(),
  evaluatePrediction: vi.fn(),
  assertPrediction: vi.fn(),
  fetchCalibration: vi.fn().mockImplementation(async () => ({ ok: true, authors: {}, evaluations: 0 })),
  fetchPredictionHistory: vi.fn().mockImplementation(async () => ({ ok: true, count: 0, predictions: [] })),
  fetchLedgerExport: vi.fn().mockImplementation(async () => ({ ok: true, events: [] })),
  uploadObservatoryAsset: vi.fn(),
  requestObservatoryLens: vi.fn(),
  fetchObservatoryLensStatus: vi.fn(),
  fetchLensHistory: vi.fn().mockImplementation(async () => ({ ok: true, operations: [] })),
  appendLedgerEvent: vi.fn().mockImplementation(async () => ({ ok: true, record_hash: 'rec_test_1' })),
  lensImageProxyUrl: (url: string | null | undefined) => url ?? null,
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node !== null && typeof node === 'object') {
    const obj = node as { children?: unknown; props?: { children?: unknown } };
    if ('children' in obj && obj.children != null) {
      return textOf(obj.children);
    }
    if (obj.props?.children != null) {
      return textOf(obj.props.children);
    }
  }
  return '';
}

const campaign = {
  session_id: 'sess-cost-test',
  target_system: 'H2',
  classical_anchor: { anchor_computed: true, fci_exact_energy: -1.137 },
  epistemics: { p_correlated: 0.92 },
} as unknown as CampaignState;

describe('observatory cost honesty (F-14)', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('idle HUD shows no price — only the on-demand descriptor', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas3D campaign={campaign} />);
    });
    const text = textOf(tree!.toJSON());
    expect(text).toContain('On-demand generative lens');
    expect(text).not.toContain('0.50');
    expect(text).not.toContain('500 cr');
    expect(text).not.toContain('Reserved $');
  });

  it('result card shows the actual reserved amount from dispatch', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <SpatialCanvas3D campaign={campaign} lensImageUrl="https://cdn.example/f.png" lensCostUsd={0.5} />,
      );
    });
    const text = textOf(tree!.toJSON());
    // textOf joins sibling text nodes with spaces.
    expect(text).toContain('Reserved $');
    expect(text).toContain('0.50');
    expect(text).toContain('against session cap');
  });

  it('result card shows no reserved line when the dispatch reported none', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <SpatialCanvas3D campaign={campaign} lensImageUrl="https://cdn.example/f.png" />,
      );
    });
    expect(textOf(tree!.toJSON())).not.toContain('Reserved $');
  });
});

describe('observatory anchor honesty (H1)', () => {
  const anchorless = {
    session_id: 'sess-anchorless',
    target_system: 'H2',
    // p_correlated present but no computed anchor: must never look computed.
    epistemics: { p_correlated: 0.92 },
  } as unknown as CampaignState;

  it('anchorless terrain shows UNVERIFIED illustrative probe + uncomputed confidence', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas3D campaign={anchorless} />);
    });
    const text = textOf(tree!.toJSON());
    expect(text).toContain('UNVERIFIED terrain — illustrative');
    expect(text).toContain('ρ uncomputed');
    expect(text).not.toContain('ρ = 0.92');
  });

  it('anchored campaign shows the computed rho and no UNVERIFIED probe tag', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas3D campaign={campaign} />);
    });
    const text = textOf(tree!.toJSON());
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

  it('hoverReadingChanged fires only on real rounded change', () => {
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
