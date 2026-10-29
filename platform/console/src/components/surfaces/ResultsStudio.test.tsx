import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ResultsStudio } from './ResultsStudio';
import type { CampaignState } from '@/types';

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    sendChatMessage: vi.fn().mockResolvedValue({ user: {}, assistant: {} }),
    fetchJobs: vi.fn().mockResolvedValue([]),
  };
});

function campaign(status: string) {
  return {
    target_system: 'H2',
    notebook: {
      title: 'H2 notebook',
      quantum_energy: -1.1368,
      classical_energy: -1.13727,
      delta_mha: 0.47,
      provenance_hash: 'sha256:abc123',
      sections: ['1. objective'],
    },
    epistemics: {
      status,
      alpha_corroborated: 0.5,
      beta_contested: 0.1,
      gamma_unverified: 0.4,
      p_correlated: 25,
      statement: 'test',
      sources: [],
    },
    quantum_factors: {},
    trajectory: [],
  } as unknown as CampaignState;
}

describe('ResultsStudio verification gating (J1)', () => {
  it('withholds convergence/accuracy/proof claims on a PROJECTED campaign with numbers', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<ResultsStudio campaign={campaign('PROJECTED')} sessionId={null} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).not.toContain('converged to an energy');
    expect(text).not.toContain('machine-checked');
    expect(text).not.toContain('0 sorryAx');
    expect(text).not.toContain('Zero sorryAx (ρ=1.000)');
    expect(text).not.toContain('satisfy strict chemical accuracy');
    expect(text).not.toContain('Verified Research Finding');
    expect(text).not.toContain('Chemical Accuracy Met');
    expect(text).toContain('PROJECTED DRAFT');
    expect(text).toContain('Projected Research Draft');
    expect(text).toContain('Variational Exploration Active');
    // Lean tab content renders on selection.
    const leanTab = tree!.root
      .findAllByType('button')
      .find((b) => JSON.stringify(b.props.children).includes('Lean 4 Proof AST'));
    act(() => {
      (leanTab!.props as { onClick: () => void }).onClick();
    });
    const leanText = JSON.stringify(tree!.toJSON());
    expect(leanText).toContain('No verified Lean 4 declaration exists');
    expect(leanText).not.toContain('sorryAx_free_check');
  });

  it('states evaluated claims on a COLLAPSED_VERIFIED campaign', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId={null} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('converged to an energy');
    expect(text).toContain('COLLAPSED_VERIFIED');
    expect(text).not.toContain('PROJECTED DRAFT');
  });

  it('renders UNVERIFIED draft with no notebook telemetry', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <ResultsStudio campaign={{ target_system: 'H2' } as unknown as CampaignState} sessionId={null} />
      );
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('UNVERIFIED DRAFT');
  });

  it('marks the optimistic annotation failed (never silently open) when sendChatMessage returns null', async () => {
    const api = await import('@/lib/api');
    vi.mocked(api.sendChatMessage).mockResolvedValueOnce(null);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId="sess-test-456" />
      );
    });

    const input = tree!.root.findByType('input');
    await act(async () => {
      input.props.onChange({ target: { value: 'Check signs on overlap integrals' } });
    });

    const form = tree!.root.findByType('form');
    await act(async () => {
      await form.props.onSubmit({ preventDefault: () => {} });
    });

    const text = JSON.stringify(tree!.toJSON());
    // The comment stays visible but is explicitly failed — never 'open'.
    expect(text).toContain('Check signs on overlap integrals');
    expect(text).toContain('failed');
    expect(text).toContain('returned null');
  });

  it('submits human annotation and dispatches re-verification prompt to active session', async () => {
    const { sendChatMessage } = await import('@/lib/api');
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId="sess-test-456" />
      );
    });

    const input = tree!.root.findByType('input');
    await act(async () => {
      input.props.onChange({ target: { value: 'Check signs on overlap integrals' } });
    });

    const form = tree!.root.findByType('form');
    await act(async () => {
      await form.props.onSubmit({ preventDefault: () => {} });
    });

    expect(sendChatMessage).toHaveBeenCalledWith(
      'sess-test-456',
      expect.stringContaining('[Human Review on §Abstract]: Check signs on overlap integrals')
    );
    expect(JSON.stringify(tree!.toJSON())).toContain('Check signs on overlap integrals');
  });
});
