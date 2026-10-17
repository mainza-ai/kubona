import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { FleetRadar } from './FleetRadar';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  fetchFleet: vi.fn(),
  fetchRadar: vi.fn(),
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('FleetRadar credential dimension', () => {
  beforeEach(() => {
    vi.mocked(api.fetchRadar).mockResolvedValue({ radar_endpoints: [] });
    vi.mocked(api.fetchFleet).mockResolvedValue({
      backends: [
        { name: 'local_simulator', type: 'sim', qubits: 32, t1_us: 'inf', status: 'ONLINE', credential_state: 'ready', queue_depth: 0, cost_per_shot: 0, jurisdiction: 'LOCAL', itar_ear: 'EXEMPT' },
        { name: 'ibm_heron', type: 'qpu', qubits: 156, t1_us: '180', status: 'READY', credential_state: 'credentialless', queue_depth: 14, cost_per_shot: 0.0016, jurisdiction: 'US', itar_ear: 'EAR99' },
      ],
    });
  });

  it('badges credentialless backends without merging them into down', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    const text = textOf(root!.root);
    // The credentialless device keeps its own status AND gains the badge.
    expect(text).toContain('READY');
    expect(text).toContain('NO CREDENTIALS');
    // The ready simulator gets no badge.
    expect(text.match(/NO CREDENTIALS/g)).toHaveLength(1);
  });

  it('shows no badge when the daemon predates credential_state', async () => {
    vi.mocked(api.fetchFleet).mockResolvedValue({
      backends: [
        { name: 'ibm_heron', type: 'qpu', qubits: 156, t1_us: '180', status: 'READY', queue_depth: 14, cost_per_shot: 0.0016, jurisdiction: 'US', itar_ear: 'EAR99' },
      ],
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    expect(textOf(root!.root)).not.toContain('NO CREDENTIALS');
  });
});
