import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { DiscoveryProtocolPanel } from './DiscoveryProtocolPanel';

const PLAN = {
  ok: true,
  goal: 'spin chain',
  planned_by: 'test plan',
  steps: [{ kind: 'spin_chain', spec: { kind: 'spin_chain' } }],
};

const RUN = {
  ok: true,
  protocol_id: 'disc-1234567890',
  goal: 'spin chain',
  planned_by: 'test plan',
  steps_requested: 1,
  steps_completed: 1,
  steps_failed: 0,
  measured_results: 1,
  claimable_results: 0,
  stopped_on_failure: false,
  results: [{ job_id: 'job-12345678', kind: 'spin_chain', status: 'JOB_STATUS_COMPLETED' }],
};

type Call = { method: string; path: string; body?: Record<string, unknown> };

// No @/lib/api mock: every backend answer is served at the fetch boundary so
// the real request/contract path is exercised.
function stubDiscoveryFetch(routes: Record<string, unknown>) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = init?.method ?? 'GET';
    calls.push({ method, path, body: init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined });
    if (!(path in routes)) throw new Error(`unexpected fetch ${method} ${url}`);
    return { ok: true, status: 200, json: async () => routes[path] };
  });
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe('DiscoveryProtocolPanel in a real browser', () => {
  it('plans then runs a measured protocol', async () => {
    const calls = stubDiscoveryFetch({
      '/api/events': { events: [] },
      '/api/discovery/plan': PLAN,
      '/api/discovery/run': RUN,
    });

    const screen = await render(<DiscoveryProtocolPanel sessionId="s" />);
    await screen.getByRole('button', { name: /Plan/ }).click();
    await expect.poll(() => document.body.textContent).toContain('planned by test plan');
    expect(calls.map((call) => `${call.method} ${call.path}`)).toContain('POST /api/discovery/plan');
    expect(calls.find((call) => call.path === '/api/discovery/plan')?.body?.goal)
      .toBe('transverse-field Ising spin chain');

    await screen.getByRole('button', { name: /Run protocol/ }).click();
    await expect.poll(() => document.body.textContent)
      .toContain('protocol disc-1234567 · 1 completed · 0 failed · 1 measured · 0 claimable');
    expect(calls.map((call) => `${call.method} ${call.path}`)).toContain('POST /api/discovery/run');
    const run = calls.find((call) => call.path === '/api/discovery/run');
    expect(run?.body?.session_id).toBe('s');
    expect(run?.body?.goal).toBe('transverse-field Ising spin chain');
    expect(document.body.textContent).toContain('1. spin_chain · JOB_STATUS_COMPLETED · job job-1234');
  });
});

describe('DiscoveryProtocolPanel history in a real browser', () => {
  it('loads completed protocol summaries from the session ledger tail', async () => {
    stubDiscoveryFetch({
      '/api/events': {
        events: [{
          id: 7,
          timestamp: '2026-11-09T22:00:00Z',
          session_id: 's',
          event_type: 'discovery_protocol_completed',
          payload: {
            protocol_id: 'disc-history',
            goal: 'Ising quench',
            completed: 2,
            failed: 0,
            measured: 2,
            claimable: 0,
            steps: [{ kind: 'spin_chain', status: 'JOB_STATUS_COMPLETED' }],
          },
        }],
      },
    });

    await render(<DiscoveryProtocolPanel sessionId="s" />);

    await expect.poll(() => document.body.textContent).toContain('Protocol history');
    const text = document.body.textContent ?? '';
    expect(text).toContain('disc-history');
    expect(text).toContain('spin_chain=JOB_STATUS_COMPLETED');
  });
});
