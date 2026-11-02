import { describe, expect, it, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import * as api from './api';
import { useCampaignProjection } from './useCampaignProjection';
import type { CampaignState } from '@/types';

vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>();
  return { ...actual, fetchCampaignResult: vi.fn() };
});

function campaign(system: string, updated_at: string): CampaignState {
  return { session_id: 's1', target_system: system, updated_at } as CampaignState;
}

describe('campaign projection ordering', () => {
  beforeEach(() => {
    vi.mocked(api.fetchCampaignResult).mockReset();
  });

  it('ignores a stale read that lands after a newer write', async () => {
    vi.mocked(api.fetchCampaignResult)
      .mockResolvedValueOnce({ ok: true, data: campaign('old', '2026-11-01T10:00:00Z') })
      .mockResolvedValueOnce({ ok: true, data: campaign('old', '2026-11-01T10:00:00Z') });
    let controls: ReturnType<typeof useCampaignProjection> | null = null;
    function Probe() {
      controls = useCampaignProjection('s1');
      return <span>{controls.campaign?.target_system ?? 'none'}</span>;
    }
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Probe />);
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => controls!.setCampaign(campaign('new', '2026-11-01T10:05:00Z')));
    await act(async () => { await controls!.reload(); });
    expect(JSON.stringify(tree!.toJSON())).toContain('new');
  });

  it('keeps the snapshot and exposes a refusal instead of erasing it', async () => {
    vi.mocked(api.fetchCampaignResult)
      .mockResolvedValueOnce({ ok: true, data: campaign('kept', '2026-11-01T10:00:00Z') })
      .mockResolvedValueOnce({
        ok: false,
        error: 'campaign read refused',
        failure: { path: '/api/sessions/s1/campaign', status: 403, kind: 'forbidden', message: 'campaign read refused', at: 1 },
      });
    let controls: ReturnType<typeof useCampaignProjection> | null = null;
    function Probe() {
      controls = useCampaignProjection('s1');
      return <span>{controls.failure?.kind ?? 'none'}</span>;
    }
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Probe />);
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => { await controls!.reload(); });
    expect(controls!.campaign?.target_system).toBe('kept');
    expect(JSON.stringify(tree!.toJSON())).toContain('forbidden');
  });
});
