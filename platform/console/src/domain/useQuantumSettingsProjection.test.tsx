import { describe, expect, it, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import * as api from '@/lib/api';
import { useQuantumSettingsProjection } from './useQuantumSettingsProjection';

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    fetchQuantumBackends: vi.fn(),
    fetchQuantumSettings: vi.fn(),
  };
});

function Probe() {
  const { data } = useQuantumSettingsProjection();
  return <span>{data?.backends.length ?? 0}</span>;
}

describe('quantum settings projection', () => {
  beforeEach(() => {
    vi.mocked(api.fetchQuantumBackends).mockResolvedValue([]);
    vi.mocked(api.fetchQuantumSettings).mockResolvedValue({ default_backend: 'local-simulator' });
  });

  it('surfaces a refused settings read instead of an empty provider list', async () => {
    vi.mocked(api.fetchQuantumSettings).mockResolvedValueOnce(null);
    function StatusProbe() {
      const { status } = useQuantumSettingsProjection();
      return <span>{status}</span>;
    }
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<StatusProbe />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('refused');
  });

  it('shares one backends/settings fetch across co-mounted consumers', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<><Probe /><Probe /></>);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(api.fetchQuantumBackends).toHaveBeenCalledTimes(1);
    expect(api.fetchQuantumSettings).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(tree!.toJSON())).toContain('0');
  });
});
