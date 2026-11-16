import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LoopPartsLane } from './LoopPartsLane';

type Call = { method: string; path: string; body?: Record<string, unknown> };

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

function refused(status: number, message: string) {
  return {
    ok: false,
    status,
    json: async () => ({ message }),
    text: async () => message,
  };
}

/** Record every request the lane makes, so "no request yet" is provable. */
function installFetch(handler: (method: string, path: string) => unknown): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;
    calls.push({ method, path, body });
    return handler(method, path);
  });
  return calls;
}

function parts(loopId: string, list: unknown[]) {
  return { ok: true, loop_id: loopId, parts: list, count: list.length };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('LoopPartsLane in a real browser (C1)', () => {
  it('renders helper columns with the active part and the job need', async () => {
    installFetch((method, path) => {
      if (method === 'GET' && path === '/api/loops/l1/parts') {
        return ok(parts('l1', [
          { part_id: 'p1', loop_id: 'l1', helper: 'lead', title: 'Optimizer step 1', state: 'done', began_ms: 1, now: '‖∇‖ = 0.05' },
          { part_id: 'p2', loop_id: 'l1', helper: 'quantum_job', title: 'Dispatched circuit_run', state: 'failed', began_ms: 2, job_status: 'failed', need: { kind: 'backend_offline', reason: 'bridge unreachable' } },
        ]));
      }
      if (method === 'GET' && path === '/api/loops/l1/steers') return ok({ steers: [] });
      return refused(404, `unexpected ${method} ${path}`);
    });

    const screen = await render(<LoopPartsLane loopId="l1" live />);
    await expect.element(screen.getByTestId('run-parts')).toBeVisible();

    const lane = screen.getByTestId('run-parts').element();
    const text = lane.textContent ?? '';
    expect(text).toContain('lead');
    expect(text).toContain('quantum job');
    expect(text).toContain('Optimizer step 1');
    expect(text).toContain('bridge unreachable');
    // Neither recorded part is running or waiting, so no marker is placed.
    expect(lane.querySelectorAll('[data-active="true"]')).toHaveLength(0);
  });

  it('loads a settled run only on request', async () => {
    const calls = installFetch((method, path) => {
      if (method === 'GET' && path === '/api/loops/l3/parts') return ok(parts('l3', []));
      return refused(404, `unexpected ${method} ${path}`);
    });

    const screen = await render(<LoopPartsLane loopId="l3" live={false} />);
    await expect.element(screen.getByTestId('run-parts-open')).toBeVisible();

    // A settled run is inert until the reviewer asks: give the mount effects a
    // tick, then prove no parts request was made at all.
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(calls.filter((call) => call.path.endsWith('/parts'))).toHaveLength(0);

    await screen.getByTestId('run-parts-open').click();
    await expect.poll(() => calls.filter((call) => call.path.endsWith('/parts')).length).toBe(1);
    expect(calls.find((call) => call.path.endsWith('/parts'))).toEqual({ method: 'GET', path: '/api/loops/l3/parts', body: undefined });
    await expect.element(screen.getByTestId('run-parts-empty')).toHaveTextContent('No parts recorded yet.');
  });

  it('steers a live run with an explicit context disclosure (C10)', async () => {
    const calls = installFetch((method, path) => {
      if (method === 'GET' && path === '/api/loops/l1/parts') {
        return ok(parts('l1', [
          { part_id: 'p1', loop_id: 'l1', helper: 'lead', title: 'Optimizer step 1', state: 'running', began_ms: 1 },
        ]));
      }
      if (method === 'GET' && path === '/api/loops/l1/steers') return ok({ steers: [] });
      if (method === 'POST' && path === '/api/loops/l1/steer') {
        return ok({
          ok: true,
          steer: { id: 'steer-12345678', loop_id: 'l1', text: 'shorter ansatz', status: 'pending', created_at: '' },
        });
      }
      return refused(404, `unexpected ${method} ${path}`);
    });

    const screen = await render(<LoopPartsLane loopId="l1" live />);
    await expect.element(screen.getByTestId('run-steer')).toBeVisible();

    const box = screen.getByTestId('run-steer').element().textContent ?? '';
    expect(box).toContain('Applied at the next step boundary');
    expect(box).toContain('active: Optimizer step 1');

    await screen.getByLabelText('Steer this run').fill('shorter ansatz');
    await screen.getByTestId('run-steer-submit').click();

    await expect.poll(() => calls.filter((call) => call.method === 'POST').length).toBe(1);
    expect(calls.find((call) => call.method === 'POST')).toEqual({
      method: 'POST',
      path: '/api/loops/l1/steer',
      body: { text: 'shorter ansatz' },
    });
    await expect.element(screen.getByTestId('run-steer-receipt')).toMatchTextContent(/next step boundary/);
  });

  it('never renders an unreadable projection as empty', async () => {
    installFetch((method, path) => {
      if (method === 'GET' && path === '/api/loops/l2/parts') return refused(401, 'login required');
      return refused(404, `unexpected ${method} ${path}`);
    });

    const screen = await render(<LoopPartsLane loopId="l2" live />);
    await expect.element(screen.getByTestId('run-parts-refused')).toMatchTextContent(/unavailable/);
    expect(screen.container.querySelector('[data-testid="run-parts"]')).toBeNull();
    expect(screen.container.querySelector('[data-testid="run-parts-empty"]')).toBeNull();
  });
});
