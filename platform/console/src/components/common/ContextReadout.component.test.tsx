import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ContextReadout } from './ContextReadout';

const fixture = {
  session_id: 's', turns: 30, turn_cap: 40, history_chars: 24000, history_chars_cap: 48000,
  would_drop: 0, pressure_pct: 75, prompt_tokens: 1000, completion_tokens: 500,
};

function stub(data: unknown) {
  vi.stubGlobal('fetch', async () => ({ ok: true, status: 200, json: async () => data }));
}

afterEach(() => vi.unstubAllGlobals());

describe('ContextReadout in a real browser', () => {
  it('renders measured pressure and tokens', async () => {
    stub(fixture);
    const screen = await render(<ContextReadout sessionId="s" refreshKey={3} />);
    const readout = screen.getByLabelText('Context pressure 75 percent');
    await expect.element(readout).toBeVisible();
    expect(readout.element().textContent).toContain('ctx 75%');
  });

  it('flags hot pressure and pending drops', async () => {
    stub({ ...fixture, pressure_pct: 92, would_drop: 4 });
    const screen = await render(<ContextReadout sessionId="s" refreshKey={1} />);
    const readout = screen.getByLabelText('Context pressure 92 percent');
    await expect.element(readout).toBeVisible();
    expect(readout.element().textContent).toContain('↓4');
  });

  it('renders nothing when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', async () => { throw new Error('down'); });
    await render(<ContextReadout sessionId="s" refreshKey={0} />);
    await expect.poll(() => document.querySelectorAll('[aria-label^="Context pressure"]').length).toBe(0);
  });
});
