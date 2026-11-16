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

function stub(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init));
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
