import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AutoresearchPanel } from './AutoresearchPanel';

afterEach(() => { vi.unstubAllGlobals(); });

function loop() {
  return {
    id: 'loop-1',
    session_id: 's1',
    harness_id: 'h1',
    status: 'running',
    current_iteration: 0,
    max_iterations: 10,
    champion_metric: null,
    champion_depth: null,
    champion_2q_gates: null,
    stagnation_reason: null,
    created_at: '',
    updated_at: '',
  };
}

function stub(
  handler: (url: string, init?: RequestInit) => unknown,
  opts: { agentic?: Record<string, unknown>; put?: Record<string, unknown>; onPut?: (body: string) => void } = {},
) {
  // The panel reloads the policy after a toggle, so the stub keeps the state
  // it returned on GET in sync with the PUT; otherwise the reload would erase
  // the tested transition.
  let state: Record<string, unknown> = {
    session_id: 's1',
    agentic_enabled: true,
    agentic_locked: false,
    stages: { hypothesize: 'act', plan: 'act', execute: 'approve', collapse: 'act' },
    max_act_budget_usd: 1,
    ...(opts.agentic ?? {}),
  };
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    if (url.endsWith('/api/sessions/s1/agentic')) {
      if (init?.method === 'PUT') {
        opts.onPut?.(String(init.body ?? ''));
        let enabled = false;
        try {
          enabled = (JSON.parse(String(init.body ?? '{}')) as { enabled?: boolean }).enabled === true;
        } catch { /* keep false */ }
        state = { ...state, agentic_enabled: enabled, ...(opts.put ?? {}) };
        const payload = {
          agentic_enabled: enabled,
          agentic_locked: state.agentic_locked,
          paused_loops: enabled ? 0 : 1,
          policy: {
            stages: state.stages,
            max_act_budget_usd: state.max_act_budget_usd,
            agentic_enabled: enabled,
            agentic_locked: state.agentic_locked,
          },
        };
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify(payload),
          json: async () => payload,
        };
      }
      return { ok: true, status: 200, json: async () => ({ ...state }) };
    }
    return handler(url, init);
  });
}

const harnesses = { harnesses: [{ id: 'h1' }], docker_runner: null };

/** Wait for the harness list to load, otherwise Start loop stays disabled. */
async function harnessReady(screen: Awaited<ReturnType<typeof render>>) {
  await expect
    .poll(() => (screen.getByLabelText('Autoresearch harness').element() as HTMLSelectElement).value)
    .toBe('h1');
}

describe('AutoresearchPanel settlement in a real browser', () => {
  it('shows applied after a confirmed start', async () => {
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, status: 200, json: async () => harnesses };
      if (url.endsWith('/api/autoresearch/loops')) return { ok: true, status: 200, json: async () => ({ loops: [loop()] }) };
      if (url.endsWith('/api/autoresearch/start')) return { ok: true, status: 200, json: async () => loop() };
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<AutoresearchPanel sessionId="s1" />);
    await harnessReady(screen);
    await screen.getByLabelText('Max iterations').fill('10');
    await screen.getByRole('button', { name: 'Start loop' }).click();
    await expect.poll(() => screen.container.textContent).toContain('applied · start loop');
  });

  it('does not render an unconfirmed transition as success', async () => {
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, status: 200, json: async () => harnesses };
      if (url.endsWith('/api/autoresearch/loops')) return { ok: true, status: 200, json: async () => ({ loops: [] }) };
      if (url.endsWith('/api/autoresearch/start')) {
        return { ok: false, status: 500, text: async () => '{"error":"no runner"}', json: async () => ({ error: 'no runner' }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<AutoresearchPanel sessionId="s1" />);
    await harnessReady(screen);
    await screen.getByRole('button', { name: 'Start loop' }).click();
    await expect.poll(() => screen.container.textContent).toContain('unknown · start not confirmed');
    expect(screen.container.textContent).not.toContain('applied · start loop');
  });

  it('renders a typed budget need on a paused run', async () => {
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, status: 200, json: async () => harnesses };
      if (url.endsWith('/api/autoresearch/loops')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ loops: [{ ...loop(), status: 'paused', need: { kind: 'budget_exceeded', reason: 'tenant ceiling exceeded' } }] }),
        };
      }
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<AutoresearchPanel sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('tenant ceiling exceeded');
    await expect.element(screen.getByText('Review spend & mandates', { exact: false })).toBeVisible();
    expect(screen.container.querySelector('[data-need="budget_exceeded"]')).not.toBeNull();
  });
});


describe('AutoresearchPanel agentic master switch', () => {
  it('turns auto agent off and blocks Start loop', async () => {
    const puts: string[] = [];
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, status: 200, json: async () => harnesses };
      if (url.endsWith('/api/autoresearch/loops')) return { ok: true, status: 200, json: async () => ({ loops: [] }) };
      throw new Error(`unexpected ${url}`);
    }, { onPut: (body) => puts.push(body) });
    const screen = await render(<AutoresearchPanel sessionId="s1" />);
    await harnessReady(screen);
    const toggle = screen.getByRole('switch', { name: 'Auto agent mode' });
    // The shared Toggle renders a native checkbox with role=switch: the
    // checked property is the state screen readers compute.
    expect((toggle.element() as HTMLInputElement).checked).toBe(true);
    await toggle.click();
    await expect.poll(() => puts).toEqual(['{"enabled":false}']);
    await expect.poll(() => screen.container.textContent).toContain('Manual mode');
    expect((screen.getByRole('button', { name: 'Start loop' }).element() as HTMLButtonElement).disabled).toBe(true);
  });

  it('refuses to unlock a session after an operator halt', async () => {
    stub((url) => {
      if (url.endsWith('/api/autoresearch/harnesses')) return { ok: true, status: 200, json: async () => harnesses };
      if (url.endsWith('/api/autoresearch/loops')) return { ok: true, status: 200, json: async () => ({ loops: [] }) };
      throw new Error(`unexpected ${url}`);
    }, { agentic: { agentic_enabled: false, agentic_locked: true, stages: { execute: 'suggest' }, max_act_budget_usd: 1 } });
    const screen = await render(<AutoresearchPanel sessionId="s1" />);
    await harnessReady(screen);
    await expect.poll(() => screen.container.textContent).toContain('Locked by an operator halt');
    expect((screen.getByRole('switch', { name: 'Auto agent mode' }).element() as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Start loop' }).element() as HTMLButtonElement).disabled).toBe(true);
  });
});
