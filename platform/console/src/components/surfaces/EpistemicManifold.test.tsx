import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { EpistemicManifold } from './EpistemicManifold';
import type { CampaignState } from '@/types';

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
});
