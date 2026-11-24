import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { QecDiscoveryStudio } from './QecDiscoveryStudio';
import type { Job } from '@/types';

function makeJob(id: string, jobType: string, result: Record<string, unknown>): Job {
  return {
    id,
    session_id: 'sess-qec',
    status: 'completed',
    job_type: jobType,
    params_json: {},
    result_json: result,
    created_at: '2026-11-24T00:00:00Z',
    completed_at: '2026-11-24T00:00:05Z',
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('QecDiscoveryStudio in a real browser', () => {
  it('renders KPI strip, SVG Frontier Plot, responsive table, and full-group MILP certificate inspector', async () => {
    const jobs: Job[] = [
      makeJob('job-bb72', 'qec_code_eval', {
        code_name: 'bravyi2024_bb_72_12_6',
        n: 72,
        k: 12,
        d: 6,
        d_kind: 'proved',
        fom: 6.0,
        decomposable: false,
        catalog_known: true,
        catalog_match: 'bravyi2024_bb_72_12_6',
        certificate: {
          proof_scope: 'full_logical_group',
          logical_group_size: 4095,
          witness_x_support: [0, 3, 7, 11, 14, 19],
          witness_z_support: [1, 4, 8, 12, 15, 20],
          witness_sha256: 'deadbeef1234567890abcdef',
        },
      }),
      makeJob('job-heron-d3', 'qec_memory_sim', {
        code_name: 'surface_d3',
        ler: 0.0166,
        ci_low: 0.0134,
        ci_high: 0.0205,
        d_circ: 3,
        decoder: 'pymatching',
        decoder_latency_us: 14.2,
        noise: {
          name: 'heron_cal_ibm_kingston_cal_8f65e57d6f49',
          calibration_snapshot_id: 'cal_8f65e57d6f49',
          p_2q: 0.0018203,
          p_meas: 0.011475,
          is_held_out: true,
        },
      }),
    ];

    const screen = await render(
      <QecDiscoveryStudio sessionId="sess-qec" jobs={jobs} onRefresh={async () => {}} />,
    );

    const text = screen.container.textContent ?? '';
    expect(text).toContain('Full-Group Proved Codes');
    expect(text).toContain('6.00');
    expect(text).toContain('0.0166');
    expect(text).toContain('cal_8f65e57d6f49');
    expect(text).toContain('PROVED (full group)');
    expect(text).toContain('Full logical group certified');
    expect(text).toContain('deadbeef1234');

    // Switch SVG Frontier Plot to Noise Space tab and verify accessible aria-selected transition
    const noiseTab = screen.getByRole('tab', { name: /Noise Space/ });
    await noiseTab.click();
    expect(noiseTab.element().getAttribute('aria-selected')).toBe('true');
    expect(screen.container.textContent ?? '').toContain('95% Wilson CI');
  });

  it('dispatches a real Heron-r2 ledger-calibrated preset via /api/experiments', async () => {
    const calls: Array<{ method: string; path: string; body?: Record<string, unknown> }> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      const parsed = new URL(String(url));
      const method = (init?.method ?? 'GET').toUpperCase();
      calls.push({
        method,
        path: parsed.pathname,
        body: init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined,
      });
      if (method === 'POST' && parsed.pathname === '/api/experiments') {
        return { ok: true, status: 200, json: async () => ({ job: makeJob('job-live-heron', 'qec_memory_sim', { ler: 0.015 }) }) };
      }
      if (method === 'GET' && parsed.pathname.startsWith('/api/jobs/')) {
        return { ok: true, status: 200, json: async () => ({ job: makeJob('job-live-heron', 'qec_memory_sim', { ler: 0.015 }) }) };
      }
      return { ok: false, status: 404, json: async () => ({ error: 'not found' }) };
    });

    const onRefresh = vi.fn(async () => {});
    const screen = await render(
      <QecDiscoveryStudio sessionId="sess-qec" jobs={[]} onRefresh={onRefresh} />,
    );

    await screen.getByRole('button', { name: /Heron-r2 Surface d=3/ }).click();
    await expect.poll(() => onRefresh.mock.calls.length, { timeout: 10000 }).toBe(1);
    const post = calls.find((c) => c.method === 'POST' && c.path === '/api/experiments');
    expect(post).toBeTruthy();
    const spec = (post?.body as { spec?: Record<string, unknown> })?.spec;
    expect(spec?.kind).toBe('qec_memory_sim');
    expect(spec?.noise_model).toBe('heron_calibration');
    expect(spec?.calibration_snapshot_id).toBe('cal_8f65e57d6f49');
  });
});
