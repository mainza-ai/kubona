import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { EpistemicManifold } from './EpistemicManifold';
import * as api from '@/lib/api';
import type { CampaignState } from '@/types';

vi.mock('@/lib/api', () => ({
  fetchEpistemics: vi.fn(),
}));

function campaignWith(status: string, alpha: number, anchorComputed = true): CampaignState {
  return {
    target_system: 'H2',
    epistemics: {
      statement: 'projection statement',
      alpha_corroborated: alpha,
      beta_contested: 0.05,
      gamma_unverified: 0.1,
      p_correlated: alpha * alpha * 100,
      status,
      literature_drift_mha: 0.4,
      sources: [],
    },
    classical_anchor: {
      discrepancy_mha: 0.4,
      chemical_accuracy_limit: 1.6,
      method: anchorComputed ? 'PySCF STO-3G' : 'uncomputed',
      anchor_computed: anchorComputed,
    },
  } as unknown as CampaignState;
}

describe('EpistemicManifold verification chrome', () => {
  it('withholds verified chrome from high-amplitude projections', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<EpistemicManifold campaign={campaignWith('PROJECTED', 0.95)} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('No verified claim');
    expect(text).not.toContain('FCI Ground State Energy Verified');
  });

  it('verifies only on explicit collapse', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<EpistemicManifold campaign={campaignWith('COLLAPSED_VERIFIED', 0.95)} />);
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('FCI Ground State Energy Verified');
  });

  it('reports no anchor measured when anchor_computed is false', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<EpistemicManifold campaign={campaignWith('PROJECTED', 0.5, false)} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('No anchor measured for this session');
    expect(text).not.toContain('Corroborated within chemical accuracy');
  });

  it('gates the Ring 1 badge on live verified state (H3)', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<EpistemicManifold campaign={campaignWith('PROJECTED', 0.95)} />);
    });
    let text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Gate unattested — no live claim');
    expect(text).not.toContain('Ring 1 Epistemic Gate Active');

    act(() => {
      tree = TestRenderer.create(<EpistemicManifold campaign={campaignWith('COLLAPSED_VERIFIED', 0.95)} />);
    });
    text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Ring 1 Epistemic Gate Active');
    expect(text).not.toContain('Gate unattested');
  });

  it('labels the headline percent with its α-mass basis (H4)', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<EpistemicManifold campaign={campaignWith('COLLAPSED_VERIFIED', 0.95)} />);
    });
    // Coherent choice: keep the % but name its basis (α mass), while the
    // deep-math panel shows α² probability mass — both powers named.
    expect(JSON.stringify(tree!.toJSON())).toContain('α mass');
  });

  it('shows a loading line — not the illustrative fallback — until the first fetch settles', async () => {
    // Never-settling fetch: first paint must be a skeleton, not fallback data.
    vi.mocked(api.fetchEpistemics).mockImplementation(() => new Promise(() => {}));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<EpistemicManifold sessionId="sess-loading-test" />);
    });
    let text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Loading epistemics');
    expect(text).not.toContain('Fallback numbers below are illustrative');

    // Settled empty fetch: loading clears and the illustrative tags remain.
    vi.mocked(api.fetchEpistemics).mockResolvedValue({});
    await act(async () => {
      tree = TestRenderer.create(<EpistemicManifold sessionId="sess-settled-test" />);
    });
    text = JSON.stringify(tree!.toJSON());
    expect(text).not.toContain('Loading epistemics');
    expect(text).toContain('illustrative');
  });
});
