import { beforeEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ExperimentPanel } from './ExperimentPanel';
import { fetchJob, runExperiment } from '@/lib/api';

vi.mock('@/lib/api', () => ({
  runExperiment: vi.fn(),
  fetchJob: vi.fn(),
  jobStatusLabel: (status: unknown) => String(status).replace(/^JOB_STATUS_/, ''),
}));

const runExperimentMock = vi.mocked(runExperiment);
const fetchJobMock = vi.mocked(fetchJob);

function runButton(root: TestRenderer.ReactTestInstance) {
  return root.findAll((node) => node.type === 'button')[0];
}

describe('ExperimentPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('submits a spin-chain experiment with the selected model', async () => {
    runExperimentMock.mockResolvedValue({
      ok: true,
      job: {
        id: 'job-spin-ui', session_id: 's', status: 'JOB_STATUS_QUEUED', job_type: 'spin_chain',
        params_json: {}, created_at: '',
      },
    });
    fetchJobMock.mockResolvedValue({
      id: 'job-spin-ui', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain',
      params_json: {}, result_json: {
        energy: -3.1, energy_exact: -3.2, uncertainty: 0.01, gap: 0.7, claimable: false, model: 'tfim', n_sites: 4,
      }, created_at: '', completed_at: '',
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ExperimentPanel sessionId="s" />);
    });
    const kind = tree.root.findByProps({ 'aria-label': 'Experiment kind' });
    await act(async () => {
      kind.props.onChange({ target: { value: 'spin_chain' } });
    });
    const run = runButton(tree.root);
    await act(async () => {
      await run.props.onClick();
    });
    const spec = runExperimentMock.mock.calls[0][1] as Record<string, unknown>;
    expect(spec.kind).toBe('spin_chain');
    expect(spec.model).toBe('tfim');
    expect(spec.n_sites).toBe(4);
    expect(spec.shots).toBe(16384);
  });

  it('submits exact conjecture terms and labels the result as non-theorem', async () => {
    runExperimentMock.mockResolvedValue({
      ok: true,
      job: {
        id: 'job-conj-ui', session_id: 's', status: 'JOB_STATUS_QUEUED', job_type: 'conjecture_run',
        params_json: {}, created_at: '',
      },
    });
    fetchJobMock.mockResolvedValue({
      id: 'job-conj-ui', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'conjecture_run',
      params_json: {}, result_json: {
        candidate_count: 1, candidates: [{ law: 'a(n) = a(n-1) + a(n-2)', status: 'conjecture' }],
      }, created_at: '', completed_at: '',
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ExperimentPanel sessionId="s" />);
    });
    const kind = tree.root.findByProps({ 'aria-label': 'Experiment kind' });
    await act(async () => {
      kind.props.onChange({ target: { value: 'conjecture' } });
    });
    const run = runButton(tree.root);
    await act(async () => {
      await run.props.onClick();
    });
    const spec = runExperimentMock.mock.calls[0][1] as Record<string, unknown>;
    expect(spec.kind).toBe('conjecture');
    expect(spec.values).toEqual(['1', '1', '2', '3', '5', '8', '13', '21', '34']);
  });
});

describe('ExperimentPanel custom molecule', () => {
  it('submits an explicit PySCF geometry and active space', async () => {
    runExperimentMock.mockResolvedValue({
      ok: true,
      job: { id: 'job-custom', session_id: 's', status: 'JOB_STATUS_QUEUED', job_type: 'vqe_run', params_json: {}, created_at: '' },
    });
    fetchJobMock.mockResolvedValue({
      id: 'job-custom', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_run',
      params_json: {}, result_json: { energy: -1.1, fci_exact_energy: -1.13, claimable: false }, created_at: '', completed_at: '',
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ExperimentPanel sessionId="s" />); });
    const molecule = tree.root.findByProps({ 'aria-label': 'Experiment molecule' });
    await act(async () => { molecule.props.onChange({ target: { value: 'custom' } }); });
    const ncas = tree.root.findByProps({ 'aria-label': 'Active orbitals' });
    const nelecas = tree.root.findByProps({ 'aria-label': 'Active electrons' });
    await act(async () => {
      ncas.props.onChange({ target: { value: '2' } });
      nelecas.props.onChange({ target: { value: '2' } });
    });
    const run = runButton(tree.root);
    await act(async () => { await run.props.onClick(); });
    const spec = runExperimentMock.mock.calls[0][1] as Record<string, unknown>;
    expect(spec.molecule).toBe('custom');
    expect(spec.atom).toContain('H 0 0 0');
    expect(spec.ncas).toBe(2);
    expect(spec.nelecas).toBe(2);
    expect(spec.basis).toBe('sto-3g');
  });
});
