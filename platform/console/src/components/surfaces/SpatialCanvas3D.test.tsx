import { beforeEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { SpatialCanvas3D } from './SpatialCanvas3D';
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
