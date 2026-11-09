import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ResultsStudio } from './ResultsStudio';
import * as api from '@/lib/api';
import type { CampaignState } from '@/types';

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    sendChatMessage: vi.fn().mockResolvedValue({ user: {}, assistant: {} }),
    fetchJobs: vi.fn().mockResolvedValue([]),
    fetchJobsResult: vi.fn().mockResolvedValue({ ok: true, data: [] }),
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

describe('ResultsStudio measured distribution', () => {
  it('charts the latest completed job counts with job provenance', async () => {
    vi.mocked(api.fetchJobsResult).mockResolvedValueOnce({
      ok: true,
      data: [{
        id: 'job-42',
        session_id: 'sess-test-456',
        status: 3,
        job_type: 'circuit_run',
        params_json: {},
        result_json: { resolved_backend: 'local_simulator', counts: { '00': 512, '11': 488 } },
        created_at: '2026-11-01T10:00:00Z',
        completed_at: '2026-11-01T10:01:00Z',
      }],
    });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId="sess-test-456" />);
      await Promise.resolve();
    });
    const charts = tree!.root.findAllByProps({ 'data-testid': 'k-chart' });
    expect(charts.length).toBe(2);
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Measured bitstring distribution');
    expect(text).toContain('job-42');
    expect(text).toContain('local_simulator');
  });
});

describe('ResultsStudio verification gating (J1)', () => {
  it('withholds a manuscript on a PROJECTED campaign even when numbers exist', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<ResultsStudio campaign={campaign('PROJECTED')} sessionId={null} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('No citable result is recorded');
    expect(text).toContain('No manuscript, citation, or proof receipt yet');
    expect(text).not.toContain('converged to an energy');
    expect(text).not.toContain('PROJECTED DRAFT');
    expect(text).not.toContain('Variational Exploration Active');
    expect(tree!.root.findAllByProps({ 'data-testid': 'k-chart' }).length).toBe(0);
  });

  it('states evaluated claims on a COLLAPSED_VERIFIED campaign', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId={null} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('measured variational quantum eigensolver result');
    expect(text).not.toContain('The optimization converged');
    expect(text).toContain('COLLAPSED_VERIFIED');
    expect(text).not.toContain('PROJECTED DRAFT');
    expect(tree!.root.findByProps({ 'data-provenance': 'measured' })).toBeTruthy();
  });

  it('renders the PES point table from structured campaign data', async () => {
    const measured = {
      ...campaign('COLLAPSED_VERIFIED'),
      measurement: {
        job_id: 'job-1', claim_id: 'claim-1', method: 'VQE', backend: 'qiskit-aer-simulator',
        energy: -1.1378, fci_exact_energy: -1.13727, uncertainty_mha: 1.384, delta_mha: 0.513,
        chemical_accuracy: true, converged: false, measured: true, below_fci: true,
      },
      pes_points: [
        { bond_distance: 0.7, energy: -1.1371, fci_exact_energy: -1.1362, delta_mha: 0.946, uncertainty_mha: 1.867, chemical_accuracy: true },
      ],
    } as unknown as CampaignState;
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ResultsStudio campaign={measured} sessionId={null} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('PES points');
    expect(text).toContain('0.7000');
    expect(text).toContain('0.946');
  });

  it('renders the typed measured-experiment table for spin and excited records', async () => {
    const measured = {
      ...campaign('COLLAPSED_VERIFIED'),
      experiments: [
        { job_id: 'job-spin-1', kind: 'spin_chain', model: 'tfim', n_sites: 4, energy: -3.19, energy_exact: -3.21, gap: 0.8, uncertainty: 0.004, claimable: false, measured: true, hardware: false },
        { job_id: 'job-exc-1', kind: 'excited_state', molecule: 'H2', ground_energy: -1.1346, excited_energy: -0.5312, gap: 0.6034, uncertainty: 0.005, claimable: false, measured: true, hardware: false },
      ],
    } as unknown as CampaignState;
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ResultsStudio campaign={measured} sessionId={null} />); });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Measured experiments');
    expect(text).toContain('spin_chain');
    expect(text).toContain('excited_state');
    expect(text).toContain('not claimable');
  });

  it('offers a measured CSV export only when a result exists', async () => {
    const createObjectURL = vi.fn(() => 'blob:measured');
    const revokeObjectURL = vi.fn();
    const originalCreate = URL.createObjectURL;
    const originalRevoke = URL.revokeObjectURL;
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = revokeObjectURL;
    try {
      const measured = {
        ...campaign('COLLAPSED_VERIFIED'),
        measurement: {
          job_id: 'job-1', claim_id: 'claim-1', method: 'VQE', backend: 'qiskit-aer-simulator',
          energy: -1.1378, fci_exact_energy: -1.13727, uncertainty_mha: 1.384, delta_mha: 0.513,
          chemical_accuracy: true, converged: false, measured: true, below_fci: true,
        },
      } as unknown as CampaignState;
      let tree: TestRenderer.ReactTestRenderer;
      await act(async () => { tree = TestRenderer.create(<ResultsStudio campaign={measured} sessionId={null} />); });
      const csv = tree!.root.findByProps({ 'data-testid': 'result-csv' });
      expect(csv).toBeTruthy();
      vi.stubGlobal('document', {
        body: { appendChild: vi.fn() },
        createElement: () => ({ href: '', download: '', click: vi.fn(), remove: vi.fn() }),
      });
      await act(async () => { csv!.props.onClick(); });
      expect(createObjectURL).toHaveBeenCalled();
      vi.unstubAllGlobals();
    } finally {
      URL.createObjectURL = originalCreate;
      URL.revokeObjectURL = originalRevoke;
    }
  });

  it('renders an empty citable state with no notebook telemetry', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <ResultsStudio campaign={{ target_system: 'H2' } as unknown as CampaignState} sessionId={null} />
      );
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('No citable result is recorded');
    expect(text).not.toContain('UNVERIFIED DRAFT');
    expect(text).not.toContain('placeholders');
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

    const input = tree!.root.findAllByType('input').find((node) => String(node.props['aria-label'] || '').startsWith('Annotate'));
    if (!input) throw new Error('annotation input missing');
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

    const input = tree!.root.findAllByType('input').find((node) => String(node.props['aria-label'] || '').startsWith('Annotate'));
    if (!input) throw new Error('annotation input missing');
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
