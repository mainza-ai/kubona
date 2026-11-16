import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { SystemReadinessPanel } from './SystemReadinessPanel';

const caps = {
  catalog_digest: 'abc123456789', harnesses: ['vqe'], tools: [],
  flags: { reviewer_configured: true, bridge_connected: true },
  budget: { max_tool_rounds: 4, max_observation_chars: 800 },
};
const fidelity = {
  device_name: 'ibm_heron', predicted_fidelity: 0.97, measured_clifford: 0, probe_run: false,
  decision: 'ADMIT', reason: 'above floor', recommended_action: 'proceed',
};

afterEach(() => vi.unstubAllGlobals());

describe('SystemReadinessPanel in a real browser', () => {
  it('shows capabilities and the fidelity floor result', async () => {
    vi.stubGlobal('fetch', async (url: string) => ({
      ok: true, status: 200,
      json: async () => String(url).includes('/api/fidelity') ? fidelity : caps,
    }));
    const screen = await render(<SystemReadinessPanel />);
    await expect.poll(() => document.body.textContent).toContain('abc123456789');
    await screen.getByRole('button', { name: 'Assess' }).click();
    await expect.poll(() => document.body.textContent).toContain('ADMIT');
    expect(document.body.textContent).toContain('0.97');
  });
});
