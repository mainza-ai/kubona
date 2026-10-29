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

  it('labels radar liveness honestly with backoff (H5)', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('Adaptive radar');
    expect(text).toContain('backs off');
    expect(text).not.toContain('1000ms Live Telemetry Radar');
  });

  it('frames tiers as a declared catalog, not live data (H10)', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('declared catalog');
    expect(text).toContain('declared — not live canary data');
  });

  it('offers an explicit manual refresh in addition to auto-poll', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    const callsBefore = vi.mocked(api.fetchFleet).mock.calls.length;
    const refreshBtn = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Refresh now'));
    expect(refreshBtn).toBeDefined();
    await act(async () => {
      refreshBtn!.props.onClick();
    });
    expect(vi.mocked(api.fetchFleet).mock.calls.length).toBeGreaterThan(callsBefore);
  });

  it('chips UNPROVENANCED cards on the grid and cost when telemetry is absent (H6)', async () => {
    vi.mocked(api.fetchFleet).mockResolvedValue({
      backends: [
        { name: 'mystery_sim', type: 'sim', qubits: 32, t1_us: 'inf', status: 'ONLINE', queue_depth: 0, cost_per_shot: 0, jurisdiction: 'LOCAL', itar_ear: 'EXEMPT' },
      ],
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    // Card grid chip renders without expanding.
    expect(textOf(root!.root)).toContain('UNPROVENANCED');
    expect(textOf(root!.root)).not.toContain('CATALOG');
    // Cost line chip renders after expanding the card.
    const toggle = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Deep Physics'));
    expect(toggle).toBeDefined();
    await act(async () => {
      toggle!.props.onClick();
    });
    expect(textOf(root!.root)).toContain('Cost per Shot');
    expect(textOf(root!.root)).toContain('(UNPROVENANCED)');
  });

  it('tags declared-catalog cards CATALOG and leaves measured ones untagged', async () => {
    vi.mocked(api.fetchFleet).mockResolvedValue({
      backends: [
        { name: 'local_simulator', type: 'sim', qubits: 32, t1_us: 'inf', status: 'ONLINE', credential_state: 'ready', telemetry: 'measured', queue_depth: 0, cost_per_shot: 0, jurisdiction: 'LOCAL', itar_ear: 'EXEMPT' },
        { name: 'ibm_heron', type: 'qpu', qubits: 156, t1_us: '180', status: 'READY', credential_state: 'credentialless', telemetry: 'declared', catalog_verified: null, queue_depth: 14, cost_per_shot: 0.0016, jurisdiction: 'US', itar_ear: 'EAR99' },
      ],
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<FleetRadar />);
    });
    const text = textOf(root!.root);
    // Exactly one CATALOG tag: queue/coherence on the Heron card are
    // vendor catalog, not live measurement. Absent telemetry (old
    // daemons) renders no tag rather than a false claim either way.
    expect(text.match(/CATALOG/g)).toHaveLength(1);
  });
});
