import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { SystemReadinessPanel } from './SystemReadinessPanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({ fetchCapabilities: vi.fn(), fetchFidelity: vi.fn() }));

describe('SystemReadinessPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('shows runtime capabilities and the fidelity floor result', async () => {
    vi.mocked(api.fetchCapabilities).mockResolvedValue({
      catalog_digest: 'abc123456789', harnesses: ['vqe'], tools: [], flags: { reviewer_configured: true, bridge_connected: true },
      budget: { max_tool_rounds: 4, max_observation_chars: 800 },
    });
    vi.mocked(api.fetchFidelity).mockResolvedValue({ admitted: true, predicted_fidelity: 0.97, fidelity_floor: 0.9, probe_run: false });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<SystemReadinessPanel />); });
    await vi.waitFor(() => expect(api.fetchCapabilities).toHaveBeenCalled());
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Assess'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.fetchFidelity).toHaveBeenCalled());
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('abc123456789');
    expect(text).toContain('admitted');
    expect(text).toContain('0.97');
  });
});
