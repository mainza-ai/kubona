import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { EpistemicManifold } from './EpistemicManifold';
import type { CampaignState } from '@/types';

// Migrated from react-test-renderer (EpistemicManifold.test.tsx). The old
// suite asserted against JSON.stringify(tree.toJSON()), which could not tell
// visible chrome from serialized props; these assertions read the rendered
// container's text, i.e. what the operator actually sees.
afterEach(() => {
  vi.unstubAllGlobals();
});

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

describe('EpistemicManifold verification chrome in a real browser', () => {
  it('withholds verified chrome from high-amplitude projections', async () => {
    const screen = await render(<EpistemicManifold campaign={campaignWith('PROJECTED', 0.95)} />);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('No verified claim');
    expect(text).not.toContain('FCI Ground State Energy Verified');
  });

  it('verifies only on explicit collapse', async () => {
    const screen = await render(<EpistemicManifold campaign={campaignWith('COLLAPSED_VERIFIED', 0.95)} />);
    expect(screen.container.textContent ?? '').toContain('FCI Ground State Energy Verified');
  });

  it('reports no anchor measured when anchor_computed is false', async () => {
    const screen = await render(<EpistemicManifold campaign={campaignWith('PROJECTED', 0.5, false)} />);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('No anchor measured for this session');
    expect(text).not.toContain('Corroborated within chemical accuracy');
  });

  it('gates the Ring 1 badge on live verified state (H3)', async () => {
    const projected = await render(<EpistemicManifold campaign={campaignWith('PROJECTED', 0.95)} />);
    const projectedText = projected.container.textContent ?? '';
    expect(projectedText).toContain('Gate unattested — no live claim');
    expect(projectedText).not.toContain('Ring 1 Epistemic Gate Active');

    const collapsed = await render(<EpistemicManifold campaign={campaignWith('COLLAPSED_VERIFIED', 0.95)} />);
    const collapsedText = collapsed.container.textContent ?? '';
    expect(collapsedText).toContain('Ring 1 Epistemic Gate Active');
    expect(collapsedText).not.toContain('Gate unattested');
  });

  it('labels the headline percent with its α-mass basis (H4)', async () => {
    const screen = await render(<EpistemicManifold campaign={campaignWith('COLLAPSED_VERIFIED', 0.95)} />);
    // Coherent choice: keep the % but name its basis (α mass), while the
    // deep-math panel shows α² probability mass — both powers named.
    expect(screen.container.textContent ?? '').toContain('α mass');
  });

  it('shows a loading line, never a fallback claim, while the fetch is pending', async () => {
    // Never-settling fetch: first paint must be a skeleton, not fallback data.
    vi.stubGlobal('fetch', () => new Promise(() => {}));
    const screen = await render(<EpistemicManifold sessionId="sess-loading-test" />);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Loading epistemics');
    expect(text).not.toContain('H₂ ground state equilibrium energy');
  });

  it('clears the loading line into an honest empty state after a settled empty fetch', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, status: 200, json: async () => ({}) }));
    const screen = await render(<EpistemicManifold sessionId="sess-settled-test" />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('No evaluated epistemic claim for this session.');
    const text = screen.container.textContent ?? '';
    expect(text).not.toContain('Loading epistemics');
    expect(text).not.toContain('illustrative');
  });
});
