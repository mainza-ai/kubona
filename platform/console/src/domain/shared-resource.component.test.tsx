import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { resetSharedResourcesForTests, useSharedResource } from './shared-resource';

function Probe({ load }: { load: () => Promise<{ n: number }> }) {
  const state = useSharedResource('probe', load, { intervalMs: 1000 });
  return <div>{state.data ? state.data.n : 'idle'}</div>;
}

beforeEach(() => resetSharedResourcesForTests());

describe('shared domain resource in a real browser', () => {
  it('loads once for two simultaneous consumers and refreshes after disposal', async () => {
    const load = vi.fn(async () => ({ n: 7 }));
    const screen = await render(<><Probe load={load} /><Probe load={load} /></>);
    await expect.poll(() => document.body.textContent).toContain('7');
    expect(load).toHaveBeenCalledTimes(1);
    await screen.unmount();
    const again = await render(<Probe load={load} />);
    await expect.poll(() => document.body.textContent).toContain('7');
    expect(load).toHaveBeenCalledTimes(2);
    await again.unmount();
  });
});
