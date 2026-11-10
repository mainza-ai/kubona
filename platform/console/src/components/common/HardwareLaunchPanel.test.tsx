import { beforeEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { HardwareLaunchPanel } from './HardwareLaunchPanel';
import { prepareSpinChainHardware, launchSpinChainHardware, prepareVQEHardware, launchVQEHardware, preparePESHardware, launchPESHardware, fetchJob, verifySpendMandate } from '@/lib/api';
import { createSignedSpendMandateWithCustody } from '@/lib/mandates';

vi.mock('@/lib/api', () => ({
  prepareSpinChainHardware: vi.fn(),
  launchSpinChainHardware: vi.fn(),
  prepareVQEHardware: vi.fn(),
  launchVQEHardware: vi.fn(),
  preparePESHardware: vi.fn(),
  launchPESHardware: vi.fn(),
  fetchJob: vi.fn(),
  verifySpendMandate: vi.fn(),
  jobStatusLabel: (status: unknown) => String(status).replace(/^JOB_STATUS_/, ''),
}));
vi.mock('@/lib/mandates', () => ({ createSignedSpendMandateWithCustody: vi.fn() }));

const prepareMock = vi.mocked(prepareSpinChainHardware);
const launchMock = vi.mocked(launchSpinChainHardware);
const prepareVQEMock = vi.mocked(prepareVQEHardware);
const launchVQEMock = vi.mocked(launchVQEHardware);
const preparePESMock = vi.mocked(preparePESHardware);
const launchPESMock = vi.mocked(launchPESHardware);
const fetchJobMock = vi.mocked(fetchJob);
const verifyMock = vi.mocked(verifySpendMandate);
const signMock = vi.mocked(createSignedSpendMandateWithCustody);

const sourceJob = {
  id: 'job-spin-source', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain',
  params_json: {}, created_at: '',
  result_json: { kind: 'spin_chain', model: 'tfim', n_sites: 4 },
};

function button(root: TestRenderer.ReactTestInstance, needle: string) {
  return root.findAll((n) => n.type === 'button' && n.children.some((c) => typeof c === 'string' && c.includes(needle)))[0];
}

describe('HardwareLaunchPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('prepares the exact binding, signs it, and launches the governed job', async () => {
    prepareMock.mockResolvedValue({
      binding: 'binding-abc', spec: { requested_backend: 'ibm_fez', shots: 1024, max_cost_usd: 1 },
    });
    signMock.mockResolvedValue({
      tenant_id: 'lab', job_hash: 'binding-abc', max_budget_usd: 1,
      expiry_timestamp: '2030-01-01T00:00:00Z', public_key_hex: 'aa', signature_hex: 'bb',
    });
    verifyMock.mockResolvedValue({ valid: true, receipt: 'receipt-1' });
    launchMock.mockResolvedValue({
      id: 'job-hw-new', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain_hardware',
      params_json: {}, created_at: '', result_json: { hardware_job_id: 'vendor-1' },
    });
    fetchJobMock.mockResolvedValue({
      id: 'job-hw-new', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain_hardware',
      params_json: {}, created_at: '', result_json: { hardware_job_id: 'vendor-1' },
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<HardwareLaunchPanel sessionId="s" tenantId="lab" sourceJob={sourceJob as never} />);
    });
    await act(async () => { await button(tree.root, 'Prepare binding').props.onClick(); });
    expect(prepareMock).toHaveBeenCalledWith('s', 'job-spin-source', 'ibm_fez', 1024);
    await act(async () => { await button(tree.root, 'Sign').props.onClick(); });
    expect(signMock).toHaveBeenCalledWith(expect.objectContaining({ tenantId: 'lab', jobHash: 'binding-abc' }));
    expect(launchMock).toHaveBeenCalledWith('s', 'job-spin-source', 'receipt-1', 'ibm_fez', 1024, 1, expect.any(String), 'ZM');
    expect(JSON.stringify(tree.toJSON())).toContain('vendor-1');
  }, 10_000);

  it('routes a VQE source through the VQE hardware prepare and launch rails', async () => {
    const vqeSource = {
      id: 'job-vqe-source', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_run',
      params_json: {}, created_at: '', result_json: { kind: 'vqe', molecule: 'H2', theta: [0.1], ansatz_qasm: 'OPENQASM 2.0; qreg q[1];' },
    };
    prepareVQEMock.mockResolvedValue({ binding: 'binding-vqe', spec: { shots: 1024, max_cost_usd: 1 } });
    signMock.mockResolvedValue({ tenant_id: 'lab', job_hash: 'binding-vqe', max_budget_usd: 1, expiry_timestamp: '2030-01-01T00:00:00Z', public_key_hex: 'aa', signature_hex: 'bb' });
    verifyMock.mockResolvedValue({ valid: true, receipt: 'receipt-vqe' });
    launchVQEMock.mockResolvedValue({ id: 'job-vqe-hw', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'hardware_experiment', params_json: {}, created_at: '', result_json: { hardware_job_id: 'vendor-vqe' } });
    fetchJobMock.mockResolvedValue({ id: 'job-vqe-hw', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'hardware_experiment', params_json: {}, created_at: '', result_json: { hardware_job_id: 'vendor-vqe' } });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<HardwareLaunchPanel sessionId="s" tenantId="lab" sourceJob={vqeSource as never} />); });
    await act(async () => { await button(tree.root, 'Prepare binding').props.onClick(); });
    expect(prepareVQEMock).toHaveBeenCalledWith('s', 'job-vqe-source', 'ibm_fez', 1024, false, false);
    await act(async () => { await button(tree.root, 'Sign').props.onClick(); });
    expect(launchVQEMock).toHaveBeenCalledWith('s', 'job-vqe-source', 'receipt-vqe', 'ibm_fez', 1024, 1, expect.any(String), 'ZM', false, false);
    expect(JSON.stringify(tree.toJSON())).toContain('vendor-vqe');
  }, 10_000);
});


describe('HardwareLaunchPanel PES', () => {
  it('prepares and launches a hardware PES with pricing evidence', async () => {
    const source = {
      id: 'job-pes-source', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_pes',
      params_json: {}, created_at: '',
      result_json: { kind: 'pes', molecule: 'H2', points: [{ bond_distance: 0.7 }, { bond_distance: 0.8 }] },
    };
    preparePESMock.mockResolvedValue({
      binding: 'binding-pes', spec: { requested_backend: 'ibm_fez', shots: 1024, max_cost_usd: 1 },
      pricing: { available: true, plan: 'open', pricing_type: 'free', estimated_usd: 0 },
    });
    signMock.mockResolvedValue({ tenant_id: 'lab', job_hash: 'binding-pes', max_budget_usd: 1, expiry_timestamp: '2030-01-01T00:00:00Z', public_key_hex: 'aa', signature_hex: 'bb' });
    verifyMock.mockResolvedValue({ valid: true, receipt: 'receipt-pes' });
    launchPESMock.mockResolvedValue({ id: 'job-pes-hw', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'hardware_pes', params_json: {}, created_at: '', result_json: { hardware_job_id: 'vendor-pes' } });
    fetchJobMock.mockResolvedValue({ id: 'job-pes-hw', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'hardware_pes', params_json: {}, created_at: '', result_json: { hardware_job_id: 'vendor-pes' } });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<HardwareLaunchPanel sessionId="s" tenantId="lab" sourceJob={source as never} />); });
    await act(async () => { await button(tree.root, 'Prepare binding').props.onClick(); });
    expect(preparePESMock).toHaveBeenCalledWith('s', 'job-pes-source', 'ibm_fez', 1024, false);
    await act(async () => { await button(tree.root, 'Sign').props.onClick(); });
    expect(launchPESMock).toHaveBeenCalledWith('s', 'job-pes-source', 'receipt-pes', 'ibm_fez', 1024, 1, expect.any(String), 'ZM', false);
    expect(JSON.stringify(tree.toJSON())).toContain('vendor-pes');
    expect(JSON.stringify(tree.toJSON())).toContain('estimated $0');
  }, 10_000);
});
