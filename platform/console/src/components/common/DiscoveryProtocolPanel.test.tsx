import { beforeEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { DiscoveryProtocolPanel } from './DiscoveryProtocolPanel';
import { fetchEventsResult, planDiscovery, runDiscoveryProtocol } from '@/lib/api';

vi.mock('@/lib/api', () => ({
  planDiscovery: vi.fn(),
  runDiscoveryProtocol: vi.fn(),
  fetchEventsResult: vi.fn().mockResolvedValue({ ok: true, data: [] }),
}));

const planMock = vi.mocked(planDiscovery);
const runMock = vi.mocked(runDiscoveryProtocol);
const eventsMock = vi.mocked(fetchEventsResult);

function buttons(root: TestRenderer.ReactTestInstance) {
  return root.findAll((node) => node.type === 'button');
}

describe('DiscoveryProtocolPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    eventsMock.mockResolvedValue({ ok: true, data: [] });
  });

  it('plans then runs a measured protocol', async () => {
    planMock.mockResolvedValue({
      ok: true, goal: 'spin chain', planned_by: 'test plan',
      steps: [{ kind: 'spin_chain', spec: { kind: 'spin_chain' } }],
    });
    runMock.mockResolvedValue({
      ok: true, protocol_id: 'disc-1234567890', goal: 'spin chain', planned_by: 'test plan',
      steps_requested: 1, steps_completed: 1, steps_failed: 0,
      measured_results: 1, claimable_results: 0, stopped_on_failure: false,
      results: [{ job_id: 'job-12345678', kind: 'spin_chain', status: 'JOB_STATUS_COMPLETED' }],
    });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<DiscoveryProtocolPanel sessionId="s" />);
    });
    const [planButton, runButton] = buttons(tree.root);
    await act(async () => { await planButton.props.onClick(); });
    expect(planMock).toHaveBeenCalledWith('transverse-field Ising spin chain');
    expect(JSON.stringify(tree.toJSON())).toContain('test plan');
    await act(async () => { await runButton.props.onClick(); });
    expect(runMock).toHaveBeenCalled();
    expect(JSON.stringify(tree.toJSON())).toContain('completed');
    expect(JSON.stringify(tree.toJSON())).toContain('measured');
  });
});


describe('DiscoveryProtocolPanel history', () => {
  it('loads completed protocol summaries from the session ledger tail', async () => {
    eventsMock.mockResolvedValue({
      ok: true,
      data: [{
        id: 7, timestamp: '2026-11-09T22:00:00Z', session_id: 's', event_type: 'discovery_protocol_completed',
        payload: {
          protocol_id: 'disc-history', goal: 'Ising quench', completed: 2, failed: 0,
          measured: 2, claimable: 0, steps: [{ kind: 'spin_chain', status: 'JOB_STATUS_COMPLETED' }],
        },
      }],
    } as never);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<DiscoveryProtocolPanel sessionId="s" />); await Promise.resolve(); });
    const text = JSON.stringify(tree.toJSON());
    expect(text).toContain('Protocol history');
    expect(text).toContain('disc-history');
    expect(text).toContain('spin_chain=JOB_STATUS_COMPLETED');
  });
});
